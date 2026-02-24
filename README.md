| English | [日本語](README-ja.md) |
|:---:|:---:|

# crai (catcher in the rAI)

---

```
  ██████╗ ██████╗  █████╗ ██╗
 ██╔════╝ ██╔══██╗██╔══██╗██║
 ██║      ██████╔╝███████║██║
 ██║      ██╔══██╗██╔══██║██║
 ╚██████╗ ██║  ██║██║  ██║██║
  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝
  catcher in the rAI
```

A CLI tool that detects when your AI agent finishes a long response and notifies you — with sound and a Notification Center banner.

Like the Catcher in the Rye — though perhaps it's *you* who's being caught, before your mind falls too deep into another context.

> *"I thought what I'd do was, I'd pretend I was one of those deaf-mutes... until the AI finishes its thought."*

---

## How It Works

```
 ┌──────────┐   raw stdin   ┌─────────────┐   PTY   ┌───────────┐
 │  You     │ ────────────► │    crai     │ ──────► │  claude   │
 │  (human) │ ◄──────────── │  (watcher)  │ ◄────── │  (AI CLI) │
 └──────────┘   raw stdout  └──────┬──────┘         └───────────┘
                                   │
                   silence ≥ 1500ms after AI output
                                   │
                                   ▼
                    🔔 Play Sound
                    🪟 Notification Center Banner
                    🔕 Terminal Bell (\a)
```

1. Spawns your command inside a **pseudo-terminal (PTY)**
2. Bridges your raw stdin/stdout through it with zero transformation
3. Monitors the output stream for **silence** — if no new output arrives for 1500ms, the AI is considered done
4. On completion: fires three notifications in parallel — a system sound, a Notification Center banner, and a terminal bell

### Smart filtering

- **1:1 prompt gating** — each Enter press arms exactly one notification; AI output with no corresponding prompt (startup banners, unsolicited output) is ignored
- **Echo suppression** — output arriving within 100ms of a keystroke is treated as PTY echo, not AI output, and ignored
- **Quick-response suppression** — if the AI responds in under 5 seconds, no notification fires (you're probably still watching)
- **Typing suppression** — no notification while you're actively composing your next message

---

## Install

```sh
brew install your-name/tap/crai
```

Or build from source:

```sh
git clone https://github.com/your-name/crai
cd crai
go build -o crai .
sudo mv crai /usr/local/bin/
```

---

## Usage

```sh
# Wrap claude directly
crai claude

# Pass arguments through transparently
crai claude --dangerously-skip-permissions
```

Everything is passed through as-is. Colors, spinners, keybindings — all intact. `crai` is invisible until it isn't.

---

## Alias

Add this to your shell config (`~/.zshrc` or `~/.bashrc`):

```sh
alias claude="crai claude "
```

> **Why the trailing space?**
> In bash and zsh, a trailing space in an alias value causes the shell to also expand the next word as an alias. This means any arguments you pass after `claude` are also subject to alias expansion — preserving the full alias magic chain.

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

- macOS (uses `afplay` for audio and `osascript` for Notification Center banners)
- Any command-line AI tool (or other long-running interactive CLI)

---

## License

[MIT](LICENSE.md).

---

*"Don't ever tell anybody anything. If you do, you start missing everybody."*
— J.D. Salinger, *The Catcher in the Rye*
