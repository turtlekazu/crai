package main

import (
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

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: crai <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "Example: crai claude --dangerously-skip-permissions\n")
		os.Exit(1)
	}

	cmd := exec.Command(os.Args[1], os.Args[2:]...)

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
	var displayPrompt string     // prompt text shown in notification
	notificationPending := false // true from Enter until the resulting notification fires

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
				if dp := displayPrompt; dp != "" {
					runes := []rune(dp)
					if len(runes) > 50 {
						dp = string(runes[:50]) + "..."
					}
					dp = strings.ReplaceAll(dp, `"`, `'`)
					msg = "AI finished: " + dp
				}

				exec.Command("afplay", "/System/Library/Sounds/Glass.aiff").Start()
				exec.Command("osascript", "-e", `display notification "`+msg+`" with title "crai"`).Start()
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
					}
					state = stateWorking
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

	// stdin -> PTY (manual loop to track last keystroke time and buffer prompt text)
	go func() {
		buf := make([]byte, 256)
		var promptBuf []byte // local: accumulates the current input line
		inEsc := false       // inside an escape sequence
		escStep := 0         // 1 = saw ESC, 2 = saw ESC [ or ESC O

		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				ptmx.Write(buf[:n])
				mu.Lock()
				lastInput = time.Now()

				for i := 0; i < n; i++ {
					b := buf[i]

					// Escape sequence state machine: skip arrow keys, function keys, etc.
					if inEsc {
						switch escStep {
						case 1: // after ESC
							if b == '[' || b == 'O' {
								escStep = 2
							} else {
								inEsc = false
								escStep = 0
							}
						case 2: // in CSI / SS3: wait for final byte (0x40-0x7E)
							if b >= 0x40 && b <= 0x7E {
								inEsc = false
								escStep = 0
							}
						}
						continue
					}

					switch {
					case b == 0x1B: // ESC - start of escape sequence
						inEsc = true
						escStep = 1
					case b == 0x7F: // backspace - remove last rune from buffer
						s := string(promptBuf)
						runes := []rune(s)
						if len(runes) > 0 {
							promptBuf = []byte(string(runes[:len(runes)-1]))
						}
					case b == '\r': // Enter - submit
						if !submitted {
							submitted = true
						}
						displayPrompt = string(promptBuf)
						promptBuf = promptBuf[:0]
						notificationPending = true
					case b < 0x20:
						// skip other control characters
					default:
						// printable ASCII or UTF-8 continuation / lead byte
						promptBuf = append(promptBuf, b)
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
