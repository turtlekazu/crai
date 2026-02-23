# crai
*(pronounced: cry)*

> *"I thought what I'd do was, I'd pretend I was one of those deaf-mutes... until the AI finishes its thought."*

---

```
  ██████╗ ██████╗  █████╗ ██╗
 ██╔════╝ ██╔══██╗██╔══██╗██║
 ██║      ██████╔╝███████║██║
 ██║      ██╔══██╗██╔══██║██║
 ╚██████╗ ██║  ██║██║  ██║██║
  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝
  Catcher in the rAI
```

A **fully transparent PTY proxy** that wraps any interactive AI CLI — like Claude Code — and cries out the moment the AI finishes its thought.

---

## The Deal

You launch your AI. It starts writing code, spinning its gears, thinking in silicon silence.

You? You go back to work — headphones on, eyes elsewhere, pretending you're deaf-mute to the machine. No tab-switching. No anxiety-polling. No `"is it done yet?"`.

The moment the AI returns to its prompt — waiting for your next command — `crai` shatters the silence. A single chime. Glass breaking.

**That's your cue. Come back.**

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

## Alias Magic

Add this to your shell config (`~/.zshrc` or `~/.bashrc`):

```sh
alias claude="crai claude "
```

> **Why the trailing space?**
> In bash and zsh, a trailing space in an alias value causes the shell to also expand the next word as an alias. This means any arguments you pass after `claude` are also subject to alias expansion — preserving the full alias magic chain.

Now you just use `claude` as normal. `crai` is silently watching.

---

## How It Works

```
 ┌──────────┐   raw stdin   ┌─────────────┐   PTY   ┌───────────┐
 │  You     │ ────────────► │    crai     │ ──────► │  claude   │
 │  (human) │ ◄──────────── │  (watcher)  │ ◄────── │  (AI CLI) │
 └──────────┘   raw stdout  └──────┬──────┘         └───────────┘
                                   │
                     detects ❯  or > in output
                                   │
                                   ▼
                          🔔 afplay Glass.aiff
```

1. Spawns your command inside a **pseudo-terminal (PTY)**
2. Bridges your raw stdin/stdout through it with zero transformation
3. Monitors the PTY output stream for the AI's input prompt (`❯ ` / `> `)
4. On detection: fires `afplay /System/Library/Sounds/Glass.aiff` asynchronously
5. Returns to silence. Waiting. Watching.

---

## Etymology / Lore

The name `crai` carries three meanings simultaneously:

### 1. **C**atcher in the **rAI**
An homage to J.D. Salinger's *The Catcher in the Rye* — the novel that Aoi, the Laughing Man of *Ghost in the Shell: S.A.C.*, carried as his manifesto. He embedded its opening quote into a corporate logo, invisible to everyone who wasn't looking. `crai` is invisible too — until it speaks.

### 2. The Laughing Man vs. The Crying One
Aoi was *the Laughing Man* — silent, masked, untraceable. This tool is his shadow: **the Crying Man**. Where he embraced silence, `crai` breaks it. A melancholic counterpart to the ghost who never spoke.

### 3. Crying Out
The tool's function, plainly stated: it **cries out** to notify the developer. When the AI finishes its thought and returns to the prompt, `crai` is the voice that says *"hey. it's done."*

---

## Requirements

- macOS (uses `afplay` for audio)
- A command-line AI tool that uses `❯ ` or `> ` as its input prompt

---

## License

MIT. Do whatever you want with it. Salinger would probably hate that.

---

*"Don't ever tell anybody anything. If you do, you start missing everybody."*
— J.D. Salinger, *The Catcher in the Rye*

*(Unless the AI finishes. Then crai tells you everything.)*
