package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

const (
	stateIdle     = iota // initial / after notification, waiting for next output
	stateWorking         // AI is streaming output
	stateNotified        // notified; waiting for next output burst to reset
)

const (
	silenceThreshold = 1500 * time.Millisecond
	// PTY echo arrives within a few ms of input; LLM first-token latency is 100ms+.
	// Treat output as AI output only when it arrives this long after the last keystroke.
	echoWindow = 100 * time.Millisecond
	// Don't notify if the AI finished in less than this duration (quick responses
	// don't need an audible alert because the developer is likely still watching).
	minWorkingDuration = 5 * time.Second
)

type monitor struct {
	mu                  sync.Mutex
	state               int
	lastOutput          time.Time
	lastInput           time.Time
	workingStarted      time.Time
	submitted           bool
	notificationPending bool
}

// onPTYOutput updates state when AI output arrives (called with lock held externally).
func (m *monitor) onPTYOutput() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Only treat as AI output after the user has submitted at least once,
	// and the output arrives well after the last keystroke (not an echo).
	if m.submitted && time.Since(m.lastInput) >= echoWindow {
		m.lastOutput = time.Now()
		if m.state != stateWorking {
			m.workingStarted = time.Now()
		}
		m.state = stateWorking
	}
}

// onStdinInput updates state when the user types.
func (m *monitor) onStdinInput(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastInput = time.Now()
	for _, b := range data {
		if b == '\r' { // Enter in raw mode
			m.submitted = true
			m.notificationPending = true
			break
		}
	}
}

// notify fires the three simultaneous notifications (sound, banner, bell).
// Must be called without the monitor lock held.
func notify(agentName string, noSound bool, soundFile string, noBanner bool) {
	msg := agentName + " finished"
	if !noSound {
		exec.Command("afplay", soundFile).Start()
	}
	if !noBanner {
		if _, err := exec.LookPath("terminal-notifier"); err == nil {
			exec.Command("terminal-notifier",
				"-title", "crai",
				"-message", msg,
				"-ignoreDnD",
			).Start()
		} else {
			exec.Command("osascript", "-e", `display notification "`+msg+`" with title "crai"`).Start()
		}
	}
	os.Stdout.Write([]byte("\a"))
}

// watchAndNotify polls every 100ms and fires a notification when silence conditions are met.
func (m *monitor) watchAndNotify(agentName string, noSound bool, soundFile string, noBanner bool, silenceThreshold time.Duration) {
	for {
		time.Sleep(100 * time.Millisecond)
		m.mu.Lock()
		shouldNotify := m.state == stateWorking &&
			m.notificationPending &&
			time.Since(m.lastOutput) >= silenceThreshold &&
			time.Since(m.lastInput) >= silenceThreshold &&
			time.Since(m.workingStarted) >= minWorkingDuration
		if shouldNotify {
			m.state = stateNotified
			m.notificationPending = false
		}
		m.mu.Unlock()

		if shouldNotify {
			notify(agentName, noSound, soundFile, noBanner)
		}
	}
}

// copyPTYToStdout streams PTY output to stdout and updates monitor state.
func (m *monitor) copyPTYToStdout(ptmx *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
			m.onPTYOutput()
		}
		if err != nil {
			if err != io.EOF {
				_ = err
			}
			break
		}
	}
}

// copyStdinToPTY streams stdin to the PTY and tracks input state.
func (m *monitor) copyStdinToPTY(ptmx *os.File) {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			ptmx.Write(buf[:n])
			m.onStdinInput(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// agentDisplayName returns a human-readable name for the wrapped command.
func agentDisplayName(cmd string) string {
	base := filepath.Base(cmd)
	switch base {
	case "claude":
		return "Claude Code"
	case "gemini":
		return "Gemini"
	}
	if len(base) > 0 {
		return strings.ToUpper(base[:1]) + base[1:]
	}
	return "AI"
}

func main() {
	noBanner := flag.Bool("no-banner", false, "suppress Notification Center banner")
	noSound := flag.Bool("no-sound", false, "suppress sound")
	soundFile := flag.String("sound", "/System/Library/Sounds/Glass.aiff", "path to sound file played on completion")
	silenceMs := flag.Int("silence", 1500, "silence threshold in milliseconds before notification fires")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: crai [options] <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "Example: crai claude --dangerously-skip-permissions\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	agentName := agentDisplayName(args[0])

	cmd := exec.Command(args[0], args[1:]...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crai: failed to start PTY: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	// Handle terminal resize signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	sigCh <- syscall.SIGWINCH // set initial size

	// Switch stdin to raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "crai: failed to set raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	m := &monitor{lastOutput: time.Now()}

	go m.watchAndNotify(agentName, *noSound, *soundFile, *noBanner, time.Duration(*silenceMs)*time.Millisecond)
	go m.copyPTYToStdout(ptmx)
	go m.copyStdinToPTY(ptmx)

	cmd.Wait()
}
