| English | [日本語](README-ja.md) |
|:---:|:---:|

# crai (catcher in the rAI)

---

```text
   ______  ____    ___     ____
  / ____/ / __ \  /   |   /  _/
 / /     / /_/ / / /| |   / /
/ /___  / _, _/ / ___ | _/ /
\____/ /_/ |_| /_/  |_|/___/
 catcher in the rAI
```

A CLI tool that notifies you with sound and banners when your AI CLI needs attention. It can install native hooks or notification settings into supported CLIs, currently Codex, Claude Code, and Gemini CLI. Even unsupported AI CLIs can still use `crai` through the PTY wrapper mode.

Like a catcher in the semiconductor fields — though standing at the cliff's edge, catching both the AI agents running wild and your own consciousness before it falls into the depths of another context.

> *"I thought what I'd do was, I'd pretend I was one of those deaf-mutes... until the AI finishes its thought."*

---

## Install

```sh
brew install turtlekazu/tap/crai
```

Or build from source:

```sh
git clone https://github.com/turtlekazu/crai
cd crai
go build -o crai .
sudo mv crai /usr/local/bin/
```

## Quick Start

```sh
# Claude Code
crai install claude
claude

# Codex
crai install codex
codex

# Gemini CLI
crai install gemini
gemini
```

## Uninstall

```sh
brew uninstall crai
brew untap turtlekazu/tap
```

## Hook Mode

```text
 +----------+                         +---------------------------+
 |  You     | -- one-time setup ----> | crai install <agent>      |
 |  (human) |                         | writes hook / notify      |
 +----------+                         +-------------+-------------+
                                                   |
                                                   v
                                  +----------------+----------------+
                                  | AI CLI config                   |
                                  | claude: ~/.claude/settings.json |
                                  | codex:  ~/.codex/config.toml    |
                                  | gemini: ~/.gemini/settings.json |
                                  +----------------+----------------+
                                                   |
                                                   v
 +----------+   normal usage   +-----------+   hook/notify   +-----------------------------+
 |  You     | ---------------> | AI CLI    | -------------> | crai notify --source <agent> |
 |  (human) |                  |           |                +---------------+---------------+
 +----------+                  +-----------+                                |
                                                                            v
                                                              * Play Sound
                                                              * Notification Center Banner
                                                              * Terminal Bell (\a)
```

Install the notification command once for each supported CLI:

```sh
crai install claude
crai install codex
crai install gemini
```

After that, just use `claude` or `codex` as normal.

- `crai install claude` adds a `Stop` command hook to `~/.claude/settings.json`
- `crai install codex` writes a `notify` command into `~/.codex/config.toml`
- `crai install gemini` adds an `AfterAgent` command hook to `~/.gemini/settings.json`

Both integrations eventually call:

```sh
crai notify --source <agent>
```

Useful commands:

```sh
crai status claude
crai status codex
crai status gemini
crai uninstall claude
crai uninstall codex
crai uninstall gemini
```

If `~/.codex/config.toml` already has a non-`crai` `notify` command, `crai install codex` refuses to overwrite it.
Running `crai install <agent>` again is safe. If a `crai`-managed hook has drifted, install repairs it in place.

## Traditional PTY Mode

The original wrapper mode is still available, but the hook-based mode is now the primary path.

```text
 +----------+   raw stdin   +-------------+   PTY   +-----------+
 |  You     | ------------> |    crai     | ------> |  claude   |
 |  (human) | <------------ |  (watcher)  | <------ |  (AI CLI) |
 +----------+   raw stdout  +------+------+         +-----------+
                                   |
                   AI output >= 1500ms silence
                                   |
                                   v
                    * Play Sound
                    * Notification Center Banner
                    * Terminal Bell (\a)
```

1. Spawns your command inside a **pseudo-terminal (PTY)**
2. Bridges raw stdin/stdout with zero transformation
3. Watches the output stream for **silence** and treats the AI as done when no new output arrives for 1500ms or more
4. On completion, fires three notifications at once: system sound, Notification Center banner, and terminal bell

### Smart filtering

- **1:1 prompt gating** — each Enter press arms exactly one notification; AI output with no corresponding prompt (startup banners, unsolicited output) is ignored
- **Echo suppression** — output arriving within 100ms of a keystroke is treated as PTY echo, not AI output, and ignored
- **Quick-response suppression** — if the AI responds in under 5 seconds, no notification fires (you're probably still watching)
- **Typing suppression** — no notification while you're actively composing your next message

---

## PTY Mode Usage

```sh
# Wrap claude directly
crai claude

# Pass arguments through transparently
crai claude --dangerously-skip-permissions
```

Everything is passed through as-is. Colors, spinners, keybindings, all of it stays intact. `crai` stays out of the way until it needs to speak up.

---

## Alias Setup (for PTY mode)

Add this to your shell config (`~/.zshrc` or `~/.bashrc`):

```sh
alias claude="crai claude "
```

> **Why the trailing space?**
> In bash and zsh, a trailing space at the end of an alias value makes the shell expand the next word as an alias too. That means arguments passed after `claude` still participate in alias chaining, so the whole setup keeps working cleanly.

Now you just use `claude` as normal. `crai` is silently watching.

---

## Options

| Flag | Description |
|------|-------------|
| `--no-banner` | suppress Notification Center banner |
| `--no-sound` | suppress sound |
| `--sound <path>` | path to sound file played on completion (default: `Glass.aiff`) |
| `--silence <ms>` | silence threshold in milliseconds before notification fires (default: `1500`) |

macOS ships with the following sounds in `/System/Library/Sounds/`:

```
Basso  Blow  Bottle  Frog  Funk  Glass  Hero
Morse  Ping  Pop     Purr  Sosumi  Submarine  Tink
```

```sh
crai --sound /System/Library/Sounds/Ping.aiff claude
```

Any `.aiff` or `.mp3` file can be specified.

---

## Requirements

- macOS (uses `afplay` for audio and `terminal-notifier` for Notification Center banners)
- Any command-line AI tool — Claude Code, Codex, Gemini CLI, and more

### Notification setup (recommended)

Install [`terminal-notifier`](https://github.com/julienXX/terminal-notifier) for reliable notifications that appear in System Settings and respect Do Not Disturb correctly:

```sh
brew install terminal-notifier
```

If `terminal-notifier` is not installed, `crai` falls back to `osascript`. However, `osascript`-based notifications appear under "Script Editor" in System Settings → Notifications, which can be difficult to find and configure.

After installing `terminal-notifier`, the first notification will register it in **System Settings → Notifications → terminal-notifier**, where you can adjust its behavior.

---

## License

[MIT](LICENSE.md).

---

*"Don't ever tell anybody anything. If you do, you start missing everybody."*
— J.D. Salinger, *The Catcher in the Rye*
