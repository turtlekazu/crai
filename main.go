package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
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
	silenceThreshold   = 1500 * time.Millisecond
	// PTY echo arrives within a few ms of input; LLM first-token latency is 100ms+.
	// Treat output as AI output only when it arrives this long after the last keystroke.
	echoWindow         = 100 * time.Millisecond
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
	lastInput := time.Now() // initialized to now so startup output is not treated as AI output
	var workingStarted time.Time // when the current AI working session began
	submitted := false // true once the user has pressed Enter at least once

	// Watcher goroutine: fires notification after silenceThreshold of inactivity
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			if state == stateWorking &&
				time.Since(lastOutput) >= silenceThreshold &&
				time.Since(lastInput) >= silenceThreshold &&
				time.Since(workingStarted) >= minWorkingDuration {
				state = stateNotified
				exec.Command("afplay", "/System/Library/Sounds/Glass.aiff").Start()
			}
			mu.Unlock()
		}
	}()

	// PTY → stdout
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

	// stdin → PTY (manual loop to track last keystroke time)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				ptmx.Write(buf[:n])
				mu.Lock()
				lastInput = time.Now()
				if !submitted {
					for i := 0; i < n; i++ {
						if buf[i] == '\r' { // Enter in raw mode
							submitted = true
							break
						}
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
