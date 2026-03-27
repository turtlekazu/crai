package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
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
	defaultSoundFile   = "/System/Library/Sounds/Glass.aiff"
	echoWindow         = 100 * time.Millisecond
	minWorkingDuration = 5 * time.Second
)

var errCodexNotifyConflict = errors.New("codex notify is already configured by another command")

type monitor struct {
	mu                  sync.Mutex
	state               int
	lastOutput          time.Time
	lastInput           time.Time
	workingStarted      time.Time
	submitted           bool
	notificationPending bool
}

type notificationOptions struct {
	noBanner  bool
	noSound   bool
	soundFile string
}

type codexNotifyState struct {
	ConfigPath       string
	ConfigExists     bool
	CurrentCommand   []string
	CurrentRaw       string
	Managed          bool
	Installed        bool
	Drifted          bool
	ExecutablePath   string
	ExpectedCommand  []string
	ExecutableStable bool
}

type claudeHookState struct {
	ConfigPath       string
	ConfigExists     bool
	Installed        bool
	Drifted          bool
	ManagedCount     int
	ExpectedCommand  string
	CurrentCommands  []string
	ExecutablePath   string
	ExecutableStable bool
}

type geminiHookState struct {
	ConfigPath       string
	ConfigExists     bool
	Installed        bool
	Drifted          bool
	ManagedCount     int
	ExpectedCommand  string
	CurrentCommands  []string
	ExecutablePath   string
	ExecutableStable bool
}

func main() {
	if len(os.Args) < 2 {
		printTopLevelUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "install":
		exitIfErr(runInstall(os.Args[2:]))
	case "status":
		exitIfErr(runStatus(os.Args[2:]))
	case "uninstall":
		exitIfErr(runUninstall(os.Args[2:]))
	case "notify":
		exitIfErr(runNotify(os.Args[2:]))
	case "help", "--help", "-h":
		printTopLevelUsage(os.Stdout)
	default:
		exitIfErr(runLegacyProxy(os.Args[1:]))
	}
}

func printTopLevelUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  crai install claude")
	fmt.Fprintln(w, "  crai install codex")
	fmt.Fprintln(w, "  crai install gemini")
	fmt.Fprintln(w, "  crai status claude")
	fmt.Fprintln(w, "  crai status codex")
	fmt.Fprintln(w, "  crai status gemini")
	fmt.Fprintln(w, "  crai uninstall claude")
	fmt.Fprintln(w, "  crai uninstall codex")
	fmt.Fprintln(w, "  crai uninstall gemini")
	fmt.Fprintln(w, "  crai notify --source claude")
	fmt.Fprintln(w, "  crai notify --source codex")
	fmt.Fprintln(w, "  crai notify --source gemini")
	fmt.Fprintln(w, "  crai [legacy-options] <command> [args...]")
}

func runInstall(args []string) error {
	if len(args) == 0 {
		return errors.New("install requires a target")
	}
	if len(args) > 1 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args[1:], " "))
	}

	switch args[0] {
	case "codex":
		state, err := inspectCodexNotify()
		if err != nil {
			return err
		}
		if !state.ExecutableStable {
			return fmt.Errorf("refusing to install from unstable executable path: %s\nbuild or install crai first, then rerun", state.ExecutablePath)
		}
		if len(state.CurrentCommand) > 0 && !state.Managed {
			return fmt.Errorf("%w: %s\nrun `crai status codex` to inspect it, then remove or replace that notify command manually", errCodexNotifyConflict, state.CurrentRaw)
		}
		if state.Installed {
			fmt.Printf("codex notify is already installed\n")
			fmt.Printf("config: %s\n", state.ConfigPath)
			return nil
		}

		updated, err := upsertCodexNotify(state.ConfigPath, state.ExpectedCommand)
		if err != nil {
			return err
		}

		if state.Drifted {
			fmt.Printf("repaired codex notify in %s\n", updated)
			return nil
		}

		fmt.Printf("installed codex notify in %s\n", updated)
		return nil
	case "claude":
		state, err := inspectClaudeStopHook()
		if err != nil {
			return err
		}
		if !state.ExecutableStable {
			return fmt.Errorf("refusing to install from unstable executable path: %s\nbuild or install crai first, then rerun", state.ExecutablePath)
		}
		if state.Installed {
			fmt.Printf("claude stop hook is already installed\n")
			fmt.Printf("config: %s\n", state.ConfigPath)
			return nil
		}
		updated, err := upsertClaudeStopHook(state.ConfigPath, state.ExpectedCommand)
		if err != nil {
			return err
		}
		if state.Drifted {
			fmt.Printf("repaired claude stop hook in %s\n", updated)
			return nil
		}
		fmt.Printf("installed claude hook in %s\n", updated)
		return nil
	case "gemini":
		state, err := inspectGeminiAfterAgentHook()
		if err != nil {
			return err
		}
		if !state.ExecutableStable {
			return fmt.Errorf("refusing to install from unstable executable path: %s\nbuild or install crai first, then rerun", state.ExecutablePath)
		}
		if state.Installed {
			fmt.Printf("gemini AfterAgent hook is already installed\n")
			fmt.Printf("config: %s\n", state.ConfigPath)
			return nil
		}
		updated, err := upsertGeminiAfterAgentHook(state.ConfigPath, state.ExpectedCommand)
		if err != nil {
			return err
		}
		if state.Drifted {
			fmt.Printf("repaired gemini AfterAgent hook in %s\n", updated)
			return nil
		}
		fmt.Printf("installed gemini hook in %s\n", updated)
		return nil
	default:
		return fmt.Errorf("unsupported install target: %s", args[0])
	}
}

func runStatus(args []string) error {
	if len(args) == 0 {
		return errors.New("status requires a target")
	}
	if len(args) > 1 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args[1:], " "))
	}

	switch args[0] {
	case "codex":
		state, err := inspectCodexNotify()
		if err != nil {
			return err
		}

		fmt.Printf("target: codex\n")
		fmt.Printf("integration: notify command\n")
		fmt.Printf("config: %s\n", state.ConfigPath)
		if !state.ConfigExists {
			fmt.Printf("status: not installed\n")
			fmt.Printf("next: run `crai install codex`\n")
			return nil
		}
		switch {
		case state.Installed:
			fmt.Printf("status: installed\n")
		case state.Drifted:
			fmt.Printf("status: drifted\n")
		case len(state.CurrentCommand) == 0:
			fmt.Printf("status: not installed\n")
		default:
			fmt.Printf("status: conflict\n")
		}
		if len(state.CurrentCommand) > 0 {
			fmt.Printf("notify: %s\n", formatCommand(state.CurrentCommand))
		}
		if state.Drifted {
			fmt.Printf("expected: %s\n", formatCommand(state.ExpectedCommand))
			fmt.Printf("next: run `crai install codex` to repair it\n")
		}
		if !state.Drifted && !state.Installed && len(state.CurrentCommand) > 0 {
			fmt.Printf("next: remove the existing notify command manually, then run `crai install codex`\n")
		}
		return nil
	case "claude":
		state, err := inspectClaudeStopHook()
		if err != nil {
			return err
		}
		fmt.Printf("target: claude\n")
		fmt.Printf("integration: Stop hook\n")
		fmt.Printf("config: %s\n", state.ConfigPath)
		if !state.ConfigExists {
			fmt.Printf("status: not installed\n")
			fmt.Printf("next: run `crai install claude`\n")
			return nil
		}
		switch {
		case state.Installed:
			fmt.Printf("status: installed\n")
		case state.Drifted:
			fmt.Printf("status: drifted\n")
		default:
			fmt.Printf("status: not installed\n")
		}
		for _, command := range state.CurrentCommands {
			fmt.Printf("hook: %s\n", strconv.Quote(command))
		}
		if state.Drifted {
			fmt.Printf("expected: %s\n", strconv.Quote(state.ExpectedCommand))
			fmt.Printf("next: run `crai install claude` to repair it\n")
		}
		if !state.Installed && !state.Drifted {
			fmt.Printf("next: run `crai install claude`\n")
		}
		return nil
	case "gemini":
		state, err := inspectGeminiAfterAgentHook()
		if err != nil {
			return err
		}
		fmt.Printf("target: gemini\n")
		fmt.Printf("integration: AfterAgent hook\n")
		fmt.Printf("config: %s\n", state.ConfigPath)
		if !state.ConfigExists {
			fmt.Printf("status: not installed\n")
			fmt.Printf("next: run `crai install gemini`\n")
			return nil
		}
		switch {
		case state.Installed:
			fmt.Printf("status: installed\n")
		case state.Drifted:
			fmt.Printf("status: drifted\n")
		default:
			fmt.Printf("status: not installed\n")
		}
		for _, command := range state.CurrentCommands {
			fmt.Printf("hook: %s\n", strconv.Quote(command))
		}
		if state.Drifted {
			fmt.Printf("expected: %s\n", strconv.Quote(state.ExpectedCommand))
			fmt.Printf("next: run `crai install gemini` to repair it\n")
		}
		if !state.Installed && !state.Drifted {
			fmt.Printf("next: run `crai install gemini`\n")
		}
		return nil
	default:
		return fmt.Errorf("unsupported status target: %s", args[0])
	}
}

func runUninstall(args []string) error {
	if len(args) == 0 {
		return errors.New("uninstall requires a target")
	}
	if len(args) > 1 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args[1:], " "))
	}

	switch args[0] {
	case "codex":
		state, err := inspectCodexNotify()
		if err != nil {
			return err
		}
		if len(state.CurrentCommand) == 0 {
			fmt.Printf("codex notify is not installed\n")
			return nil
		}
		if !state.Managed {
			return fmt.Errorf("%w: %s", errCodexNotifyConflict, state.CurrentRaw)
		}

		updated, removed, err := removeCodexNotify(state.ConfigPath)
		if err != nil {
			return err
		}
		if !removed {
			fmt.Printf("codex notify is not installed\n")
			return nil
		}
		fmt.Printf("removed codex notify from %s\n", updated)
		return nil
	case "claude":
		state, err := inspectClaudeStopHook()
		if err != nil {
			return err
		}
		if state.ManagedCount == 0 {
			fmt.Printf("claude hook is not installed\n")
			return nil
		}
		updated, removed, err := removeClaudeStopHook(state.ConfigPath)
		if err != nil {
			return err
		}
		if !removed {
			fmt.Printf("claude hook is not installed\n")
			return nil
		}
		fmt.Printf("removed claude hook from %s\n", updated)
		return nil
	case "gemini":
		state, err := inspectGeminiAfterAgentHook()
		if err != nil {
			return err
		}
		if state.ManagedCount == 0 {
			fmt.Printf("gemini hook is not installed\n")
			return nil
		}
		updated, removed, err := removeGeminiAfterAgentHook(state.ConfigPath)
		if err != nil {
			return err
		}
		if !removed {
			fmt.Printf("gemini hook is not installed\n")
			return nil
		}
		fmt.Printf("removed gemini hook from %s\n", updated)
		return nil
	default:
		return fmt.Errorf("unsupported uninstall target: %s", args[0])
	}
}

func runNotify(args []string) error {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	source := fs.String("source", "", "notification source")
	noBanner := fs.Bool("no-banner", false, "suppress Notification Center banner")
	noSound := fs.Bool("no-sound", false, "suppress sound")
	soundFile := fs.String("sound", defaultSoundFile, "path to sound file played on completion")
	if err := fs.Parse(args); err != nil {
		return err
	}

	agentName := "AI"
	switch *source {
	case "", "codex":
		agentName = "Codex"
	case "claude":
		agentName = "Claude Code"
	case "gemini":
		agentName = "Gemini"
	default:
		agentName = humanizeName(*source)
	}

	// Codex passes a JSON payload to notify commands. We don't need it yet,
	// but we still drain stdin so future callers don't block on a full pipe.
	_, _ = io.Copy(io.Discard, os.Stdin)

	emitNotification(agentName, notificationOptions{
		noBanner:  *noBanner,
		noSound:   *noSound,
		soundFile: *soundFile,
	})
	return nil
}

func runLegacyProxy(args []string) error {
	fs := flag.NewFlagSet("legacy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	noBanner := fs.Bool("no-banner", false, "suppress Notification Center banner")
	noSound := fs.Bool("no-sound", false, "suppress sound")
	soundFile := fs.String("sound", defaultSoundFile, "path to sound file played on completion")
	silenceMs := fs.Int("silence", 1500, "silence threshold in milliseconds before notification fires")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("legacy mode requires a command")
	}

	agentName := agentDisplayName(rest[0])
	cmd := exec.Command(rest[0], rest[1:]...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}
	defer ptmx.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)
	go func() {
		for range sigCh {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	sigCh <- syscall.SIGWINCH

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	m := &monitor{lastOutput: time.Now()}
	opts := notificationOptions{
		noBanner:  *noBanner,
		noSound:   *noSound,
		soundFile: *soundFile,
	}

	go m.watchAndNotify(agentName, opts, time.Duration(*silenceMs)*time.Millisecond)
	go m.copyPTYToStdout(ptmx)
	go m.copyStdinToPTY(ptmx)

	return cmd.Wait()
}

func inspectCodexNotify() (codexNotifyState, error) {
	configPath, err := codexConfigPath()
	if err != nil {
		return codexNotifyState{}, err
	}

	exePath, stable, err := executableInstallPath()
	if err != nil {
		return codexNotifyState{}, err
	}

	state := codexNotifyState{
		ConfigPath:       configPath,
		ExecutablePath:   exePath,
		ExpectedCommand:  []string{exePath, "notify", "--source", "codex"},
		ExecutableStable: stable,
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return codexNotifyState{}, err
	}

	state.ConfigExists = true
	value, raw, found, err := readRootAssignment(string(content), "notify")
	if err != nil {
		return codexNotifyState{}, err
	}
	if !found {
		return state, nil
	}

	state.CurrentRaw = strings.TrimSpace(raw)
	cmd, ok := parseStringArray(value)
	if !ok {
		return state, nil
	}

	state.CurrentCommand = cmd
	state.Managed = isManagedCodexCommand(cmd)
	state.Installed = equalStrings(cmd, state.ExpectedCommand)
	state.Drifted = state.Managed && !state.Installed
	return state, nil
}

func codexConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "config.toml"), nil
}

func claudeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func geminiConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "settings.json"), nil
}

func executableInstallPath() (string, bool, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	exePath = filepath.Clean(exePath)

	tempDir := filepath.Clean(os.TempDir()) + string(os.PathSeparator)
	stable := !strings.HasPrefix(exePath, tempDir) && !strings.Contains(exePath, string(os.PathSeparator)+"go-build")
	return exePath, stable, nil
}

func inspectClaudeStopHook() (claudeHookState, error) {
	configPath, err := claudeConfigPath()
	if err != nil {
		return claudeHookState{}, err
	}
	exePath, stable, err := executableInstallPath()
	if err != nil {
		return claudeHookState{}, err
	}

	state := claudeHookState{
		ConfigPath:       configPath,
		ExecutablePath:   exePath,
		ExpectedCommand:  shellQuote(exePath) + " notify --source claude",
		ExecutableStable: stable,
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return claudeHookState{}, err
	}
	state.ConfigExists = true

	var root map[string]any
	if len(bytes.TrimSpace(content)) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(content, &root); err != nil {
		return claudeHookState{}, err
	}

	commands := collectClaudeManagedCommands(root)
	state.CurrentCommands = commands
	state.ManagedCount = len(commands)
	if len(commands) == 1 && commands[0] == state.ExpectedCommand {
		state.Installed = true
		return state, nil
	}
	if len(commands) > 0 {
		state.Drifted = true
	}

	return state, nil
}

func inspectGeminiAfterAgentHook() (geminiHookState, error) {
	configPath, err := geminiConfigPath()
	if err != nil {
		return geminiHookState{}, err
	}
	exePath, stable, err := executableInstallPath()
	if err != nil {
		return geminiHookState{}, err
	}

	state := geminiHookState{
		ConfigPath:       configPath,
		ExecutablePath:   exePath,
		ExpectedCommand:  shellQuote(exePath) + " notify --source gemini",
		ExecutableStable: stable,
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return geminiHookState{}, err
	}
	state.ConfigExists = true
	if len(bytes.TrimSpace(content)) == 0 {
		return state, nil
	}

	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		return geminiHookState{}, err
	}

	commands := collectGeminiManagedCommands(root)
	state.CurrentCommands = commands
	state.ManagedCount = len(commands)
	if len(commands) == 1 && commands[0] == state.ExpectedCommand {
		state.Installed = true
		return state, nil
	}
	if len(commands) > 0 {
		state.Drifted = true
	}
	return state, nil
}

func upsertClaudeStopHook(path string, expectedCommand string) (string, error) {
	root, err := loadJSONConfig(path)
	if err != nil {
		return "", err
	}

	hooks := getOrCreateMap(root, "hooks")
	stopEntries := getOrCreateArray(hooks, "Stop")

	var cleaned []any
	for _, entry := range stopEntries {
		group, ok := entry.(map[string]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		var nextHooks []any
		for _, hook := range hookList {
			hookMap, ok := hook.(map[string]any)
			if !ok {
				nextHooks = append(nextHooks, hook)
				continue
			}
			if isClaudeManagedHookMap(hookMap) {
				continue
			}
			nextHooks = append(nextHooks, hook)
		}
		group["hooks"] = nextHooks
		cleaned = append(cleaned, group)
	}

	cleaned = append(cleaned, map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": expectedCommand,
			},
		},
	})

	hooks["Stop"] = cleaned
	if err := writeJSONConfig(path, root, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func upsertGeminiAfterAgentHook(path string, expectedCommand string) (string, error) {
	root, err := loadJSONConfig(path)
	if err != nil {
		return "", err
	}

	hooks := getOrCreateMap(root, "hooks")
	afterAgentEntries := getOrCreateArray(hooks, "AfterAgent")

	var cleaned []any
	for _, entry := range afterAgentEntries {
		group, ok := entry.(map[string]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		var nextHooks []any
		for _, hook := range hookList {
			hookMap, ok := hook.(map[string]any)
			if !ok {
				nextHooks = append(nextHooks, hook)
				continue
			}
			if isGeminiManagedHookMap(hookMap) {
				continue
			}
			nextHooks = append(nextHooks, hook)
		}
		group["hooks"] = nextHooks
		cleaned = append(cleaned, group)
	}

	cleaned = append(cleaned, map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": expectedCommand,
			},
		},
	})

	hooks["AfterAgent"] = cleaned
	if err := writeJSONConfig(path, root, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func removeClaudeStopHook(path string) (string, bool, error) {
	root, err := loadJSONConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, false, nil
		}
		return "", false, err
	}

	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return path, false, nil
	}
	stopEntries, ok := hooks["Stop"].([]any)
	if !ok {
		return path, false, nil
	}

	var (
		removed bool
		cleaned []any
	)
	for _, entry := range stopEntries {
		group, ok := entry.(map[string]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		var nextHooks []any
		for _, hook := range hookList {
			hookMap, ok := hook.(map[string]any)
			if ok && isClaudeManagedHookMap(hookMap) {
				removed = true
				continue
			}
			nextHooks = append(nextHooks, hook)
		}
		if len(nextHooks) == 0 {
			continue
		}
		group["hooks"] = nextHooks
		cleaned = append(cleaned, group)
	}

	if !removed {
		return path, false, nil
	}

	if len(cleaned) == 0 {
		delete(hooks, "Stop")
	} else {
		hooks["Stop"] = cleaned
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	}

	if err := writeJSONConfig(path, root, 0o600); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func removeGeminiAfterAgentHook(path string) (string, bool, error) {
	root, err := loadJSONConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, false, nil
		}
		return "", false, err
	}

	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return path, false, nil
	}
	afterAgentEntries, ok := hooks["AfterAgent"].([]any)
	if !ok {
		return path, false, nil
	}

	var (
		removed bool
		cleaned []any
	)
	for _, entry := range afterAgentEntries {
		group, ok := entry.(map[string]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			cleaned = append(cleaned, entry)
			continue
		}

		var nextHooks []any
		for _, hook := range hookList {
			hookMap, ok := hook.(map[string]any)
			if ok && isGeminiManagedHookMap(hookMap) {
				removed = true
				continue
			}
			nextHooks = append(nextHooks, hook)
		}
		if len(nextHooks) == 0 {
			continue
		}
		group["hooks"] = nextHooks
		cleaned = append(cleaned, group)
	}

	if !removed {
		return path, false, nil
	}

	if len(cleaned) == 0 {
		delete(hooks, "AfterAgent")
	} else {
		hooks["AfterAgent"] = cleaned
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	}

	if err := writeJSONConfig(path, root, 0o600); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func upsertCodexNotify(path string, command []string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	line := "notify = " + formatTomlStringArray(command)
	updated, err := setRootAssignment(string(content), "notify", line)
	if err != nil {
		return "", err
	}
	if err := writeConfigAtomically(path, []byte(updated), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func loadJSONConfig(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return map[string]any{}, nil
	}

	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, err
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, nil
}

func writeJSONConfig(path string, root map[string]any, defaultMode os.FileMode) error {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeConfigAtomically(path, data, defaultMode)
}

func removeCodexNotify(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, false, nil
		}
		return "", false, err
	}

	updated, removed, err := deleteRootAssignment(string(content), "notify")
	if err != nil {
		return "", false, err
	}
	if !removed {
		return path, false, nil
	}
	if err := writeConfigAtomically(path, []byte(updated), 0o600); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func writeConfigAtomically(path string, data []byte, defaultMode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	mode := defaultMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "crai-config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readRootAssignment(content string, key string) (string, string, bool, error) {
	start, end, raw, found, err := locateRootAssignment(content, key)
	if err != nil || !found {
		return "", "", found, err
	}
	_ = start
	_ = end

	line := strings.TrimSpace(raw)
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", "", false, fmt.Errorf("invalid assignment for %s", key)
	}
	return strings.TrimSpace(line[eq+1:]), raw, true, nil
}

func setRootAssignment(content string, key string, assignment string) (string, error) {
	start, end, _, found, err := locateRootAssignment(content, key)
	if err != nil {
		return "", err
	}
	if found {
		return content[:start] + assignment + content[end:], nil
	}

	insertAt := firstRootSectionIndex(content)
	prefix := assignment + "\n"
	if insertAt == 0 {
		return prefix + content, nil
	}
	return content[:insertAt] + prefix + content[insertAt:], nil
}

func deleteRootAssignment(content string, key string) (string, bool, error) {
	start, end, _, found, err := locateRootAssignment(content, key)
	if err != nil || !found {
		return content, found, err
	}
	return content[:start] + content[end:], true, nil
}

func locateRootAssignment(content string, key string) (int, int, string, bool, error) {
	lines := strings.SplitAfter(content, "\n")
	offset := 0
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if isSectionHeader(trimmed) {
			return 0, 0, "", false, nil
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			offset += len(line)
			continue
		}
		if !strings.HasPrefix(trimmed, key) {
			offset += len(line)
			continue
		}

		afterKey := strings.TrimPrefix(trimmed, key)
		afterKey = strings.TrimSpace(afterKey)
		if !strings.HasPrefix(afterKey, "=") {
			offset += len(line)
			continue
		}

		start := offset
		end := offset + len(line)
		raw := line
		brackets := bracketDelta(line)
		for brackets > 0 && i+1 < len(lines) {
			i++
			line = lines[i]
			raw += line
			end += len(line)
			brackets += bracketDelta(line)
		}
		if brackets != 0 {
			return 0, 0, "", false, fmt.Errorf("unterminated array for %s", key)
		}
		return start, end, raw, true, nil
	}
	return 0, 0, "", false, nil
}

func firstRootSectionIndex(content string) int {
	lines := strings.SplitAfter(content, "\n")
	offset := 0
	for _, line := range lines {
		if isSectionHeader(strings.TrimSpace(line)) {
			return offset
		}
		offset += len(line)
	}
	return 0
}

func isSectionHeader(line string) bool {
	return strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")
}

func bracketDelta(s string) int {
	var (
		delta     int
		inString  bool
		quote     rune
		escaped   bool
		inComment bool
	)

	for _, r := range s {
		switch {
		case inComment:
			continue
		case inString:
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if r == quote {
				inString = false
			}
		default:
			switch r {
			case '#':
				inComment = true
			case '"', '\'':
				inString = true
				quote = r
			case '[':
				delta++
			case ']':
				delta--
			}
		}
	}
	return delta
}

func parseStringArray(value string) ([]string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, false
	}
	value = strings.TrimSpace(value[1 : len(value)-1])
	if value == "" {
		return []string{}, true
	}

	var (
		items    []string
		current  bytes.Buffer
		inString bool
		escaped  bool
	)

	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inString {
			current.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
			current.WriteByte(ch)
		case ',':
			item := strings.TrimSpace(current.String())
			if item == "" {
				return nil, false
			}
			unquoted, err := strconv.Unquote(item)
			if err != nil {
				return nil, false
			}
			items = append(items, unquoted)
			current.Reset()
		case ' ', '\t', '\r', '\n':
			current.WriteByte(ch)
		default:
			current.WriteByte(ch)
		}
	}

	item := strings.TrimSpace(current.String())
	if item == "" {
		return nil, false
	}
	unquoted, err := strconv.Unquote(item)
	if err != nil {
		return nil, false
	}
	items = append(items, unquoted)
	return items, true
}

func formatTomlStringArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func formatCommand(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func getOrCreateMap(root map[string]any, key string) map[string]any {
	if existing, ok := root[key].(map[string]any); ok {
		return existing
	}
	created := map[string]any{}
	root[key] = created
	return created
}

func getOrCreateArray(root map[string]any, key string) []any {
	if existing, ok := root[key].([]any); ok {
		return existing
	}
	created := []any{}
	root[key] = created
	return created
}

func collectClaudeManagedCommands(root map[string]any) []string {
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	stopEntries, ok := hooks["Stop"].([]any)
	if !ok {
		return nil
	}

	var commands []string
	for _, entry := range stopEntries {
		group, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hookList {
			hookMap, ok := hook.(map[string]any)
			if !ok || !isClaudeManagedHookMap(hookMap) {
				continue
			}
			command, _ := hookMap["command"].(string)
			commands = append(commands, command)
		}
	}
	return commands
}

func collectGeminiManagedCommands(root map[string]any) []string {
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	entries, ok := hooks["AfterAgent"].([]any)
	if !ok {
		return nil
	}

	var commands []string
	for _, entry := range entries {
		group, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hookList {
			hookMap, ok := hook.(map[string]any)
			if !ok || !isGeminiManagedHookMap(hookMap) {
				continue
			}
			command, _ := hookMap["command"].(string)
			commands = append(commands, command)
		}
	}
	return commands
}

func isClaudeManagedHookMap(hook map[string]any) bool {
	hookType, _ := hook["type"].(string)
	command, _ := hook["command"].(string)
	return hookType == "command" && isClaudeManagedCommand(command)
}

func isClaudeManagedCommand(command string) bool {
	return strings.Contains(command, " notify --source claude") &&
		strings.Contains(command, "crai")
}

func isGeminiManagedHookMap(hook map[string]any) bool {
	hookType, _ := hook["type"].(string)
	command, _ := hook["command"].(string)
	return hookType == "command" && isGeminiManagedCommand(command)
}

func isGeminiManagedCommand(command string) bool {
	return strings.Contains(command, " notify --source gemini") &&
		strings.Contains(command, "crai")
}

func isManagedCodexCommand(cmd []string) bool {
	if len(cmd) != 4 {
		return false
	}
	return strings.HasPrefix(filepath.Base(cmd[0]), "crai") &&
		cmd[1] == "notify" &&
		cmd[2] == "--source" &&
		cmd[3] == "codex"
}

func emitNotification(agentName string, opts notificationOptions) {
	msg := agentName + " finished"

	if !opts.noSound {
		_ = exec.Command("afplay", opts.soundFile).Start()
	}
	if !opts.noBanner {
		if _, err := exec.LookPath("terminal-notifier"); err == nil {
			_ = exec.Command(
				"terminal-notifier",
				"-title", "crai",
				"-message", msg,
				"-ignoreDnD",
			).Start()
		} else {
			_ = exec.Command("osascript", "-e", `display notification "`+msg+`" with title "crai"`).Start()
		}
	}

	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		_, _ = tty.Write([]byte("\a"))
		_ = tty.Close()
	}
}

func (m *monitor) onPTYOutput() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.submitted && time.Since(m.lastInput) >= echoWindow {
		m.lastOutput = time.Now()
		if m.state != stateWorking {
			m.workingStarted = time.Now()
		}
		m.state = stateWorking
	}
}

func (m *monitor) onStdinInput(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastInput = time.Now()
	for _, b := range data {
		if b == '\r' {
			m.submitted = true
			m.notificationPending = true
			break
		}
	}
}

func (m *monitor) watchAndNotify(agentName string, opts notificationOptions, silenceThreshold time.Duration) {
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
			emitNotification(agentName, opts)
		}
	}
}

func (m *monitor) copyPTYToStdout(ptmx *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			_, _ = os.Stdout.Write(buf[:n])
			m.onPTYOutput()
		}
		if err != nil {
			if err != io.EOF {
				_ = err
			}
			return
		}
	}
}

func (m *monitor) copyStdinToPTY(ptmx *os.File) {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			_, _ = ptmx.Write(buf[:n])
			m.onStdinInput(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func agentDisplayName(cmd string) string {
	base := filepath.Base(cmd)
	switch base {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "gemini":
		return "Gemini"
	}
	return humanizeName(base)
}

func humanizeName(name string) string {
	if name == "" {
		return "AI"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func exitIfErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "crai: %v\n", err)
	os.Exit(1)
}
