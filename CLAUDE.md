# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```sh
go build -o crai .
./crai claude
```

Install to PATH:
```sh
go build -o crai . && sudo mv crai /usr/local/bin/
```

There are no tests in this project.

## Architecture

This is a single-file Go program (`main.go`) — a **PTY proxy** that wraps any interactive CLI and fires notifications when the wrapped process goes silent.

### Core design

- `pty.Start(cmd)` spawns the target command in a pseudo-terminal
- Three goroutines run concurrently:
  1. **PTY → stdout**: copies output to the terminal, updates `lastOutput` and `state`
  2. **stdin → PTY**: copies keystrokes to the process, updates `lastInput` and `submitted`
  3. **Watcher**: polls every 100ms; fires notifications when silence conditions are met
- All shared state (`state`, `lastOutput`, `lastInput`, `workingStarted`, `submitted`) is protected by a single `sync.Mutex`

### State machine

```
stateIdle → stateWorking → stateNotified → stateIdle
```

- `stateIdle`: initial state, waiting for AI to start producing output
- `stateWorking`: AI is actively streaming; reset whenever new output arrives
- `stateNotified`: notification has fired; next output burst resets back to `stateIdle`

### Notification conditions (all must be true)

- `state == stateWorking`
- `time.Since(lastOutput) >= silenceThreshold` (1500ms)
- `time.Since(lastInput) >= silenceThreshold` (1500ms — avoids notifying while user is typing)
- `time.Since(workingStarted) >= minWorkingDuration` (5s — skips quick responses)
- `submitted == true` (suppresses startup banner output)

### Notification method

Three simultaneous notifications:
1. `afplay /System/Library/Sounds/Glass.aiff` — system sound
2. `osascript display notification` — macOS Notification Center banner (icon is the Script app icon; this is a known limitation)
3. `\a` written to stdout — terminal bell

### Dependencies

- `github.com/creack/pty` — PTY creation and size inheritance
- `golang.org/x/term` — raw terminal mode
- macOS system tools: `afplay`, `osascript`
