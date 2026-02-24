package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
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

// stripANSI removes ANSI/VT escape sequences from b, returning plain text.
func stripANSI(b []byte) string {
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		if b[i] != 0x1B {
			out = append(out, b[i])
			i++
			continue
		}
		i++ // skip ESC
		if i >= len(b) {
			break
		}
		switch b[i] {
		case '[': // CSI — skip until final byte (0x40–0x7E)
			i++
			for i < len(b) && !(b[i] >= 0x40 && b[i] <= 0x7E) {
				i++
			}
			i++
		case ']': // OSC — skip until BEL or ST
			i++
			for i < len(b) {
				if b[i] == 0x07 {
					i++
					break
				}
				if b[i] == 0x1B && i+1 < len(b) && b[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default: // other two-byte ESC sequences
			i++
		}
	}
	return string(out)
}

func main() {
	noBanner := flag.Bool("no-banner", false, "suppress Notification Center banner")
	noSound := flag.Bool("no-sound", false, "suppress sound")
	soundFile := flag.String("sound", "/System/Library/Sounds/Glass.aiff", "path to sound file played on completion")
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

	cmd := exec.Command(args[0], args[1:]...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crai: failed to start PTY: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	// Handle terminal resize signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	sigCh <- syscall.SIGWINCH // set initial size

	// Switch stdin to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "crai: failed to set raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Silence detection state
	var mu sync.Mutex
	state := stateIdle
	lastOutput := time.Now()
	var lastInput time.Time      // zero value; notificationPending=false blocks startup notifications
	var workingStarted time.Time // when the current AI working session began
	submitted := false           // true once the user has pressed Enter at least once
	notificationPending := false // true from Enter until the resulting notification fires
	var lastAILine string        // last non-empty text line from AI output

	// Watcher goroutine: fires notification after silenceThreshold of inactivity
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			if state == stateWorking &&
				notificationPending &&
				time.Since(lastOutput) >= silenceThreshold &&
				time.Since(lastInput) >= silenceThreshold &&
				time.Since(workingStarted) >= minWorkingDuration {
				state = stateNotified
				notificationPending = false

				msg := "AI finished"
				if lastAILine != "" {
					line := lastAILine
					runes := []rune(line)
					if len(runes) > 50 {
						line = string(runes[:50]) + "..."
					}
					line = strings.ReplaceAll(line, `"`, `'`)
					msg = "AI finished: " + line
				}

				if !*noSound {
					exec.Command("afplay", *soundFile).Start()
				}
				if !*noBanner {
					exec.Command("osascript", "-e", `display notification "`+msg+`" with title "crai"`).Start()
				}
				os.Stdout.Write([]byte("\a"))
			}
			mu.Unlock()
		}
	}()

	// PTY -> stdout
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])

				mu.Lock()
				// Only treat as AI output after the user has submitted at least once,
				// and the output arrives well after the last keystroke (not an echo).
				if submitted && time.Since(lastInput) >= echoWindow {
					lastOutput = time.Now()
					if state != stateWorking {
						workingStarted = time.Now()
						lastAILine = "" // reset for new working session
					}
					state = stateWorking

					// Track the last non-empty text line for the notification banner.
					// \r within a segment overwrites earlier content on the same line.
					text := stripANSI(buf[:n])
					for _, segment := range strings.Split(text, "\n") {
						parts := strings.Split(segment, "\r")
						line := strings.TrimSpace(parts[len(parts)-1])
						if line != "" {
							lastAILine = line
						}
					}
				}
				mu.Unlock()
			}
			if err != nil {
				if err != io.EOF {
					_ = err
				}
				break
			}
		}
	}()

	// stdin -> PTY (manual loop to track last keystroke time)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				ptmx.Write(buf[:n])
				mu.Lock()
				lastInput = time.Now()
				for i := 0; i < n; i++ {
					if buf[i] == '\r' { // Enter in raw mode
						if !submitted {
							submitted = true
						}
						notificationPending = true
						break
					}
				}
				mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}()

	cmd.Wait()
}
