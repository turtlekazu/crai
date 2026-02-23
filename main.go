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

const silenceThreshold = 1500 * time.Millisecond

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

	// Watcher goroutine: fires notification after silenceThreshold of inactivity
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			if state == stateWorking && time.Since(lastOutput) >= silenceThreshold {
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
				lastOutput = time.Now()
				// Reset to WORKING whenever new output arrives
				// (covers both IDLE→WORKING and NOTIFIED→WORKING)
				state = stateWorking
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

	// stdin → PTY
	go func() {
		io.Copy(ptmx, os.Stdin)
	}()

	cmd.Wait()
}
