package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetRootAssignmentPreservesSectionNewline(t *testing.T) {
	content := strings.Join([]string{
		`notify = ["/tmp/old-crai", "notify", "--source", "codex"]`,
		`[projects."/Users/turtlekazu/StudioProjects/Messay2"]`,
		`trust_level = "trusted"`,
		"",
	}, "\n")

	updated, err := setRootAssignment(
		content,
		"notify",
		`notify = ["/opt/homebrew/bin/crai", "notify", "--source", "codex"]`,
	)
	if err != nil {
		t.Fatalf("setRootAssignment returned error: %v", err)
	}

	want := strings.Join([]string{
		`notify = ["/opt/homebrew/bin/crai", "notify", "--source", "codex"]`,
		`[projects."/Users/turtlekazu/StudioProjects/Messay2"]`,
		`trust_level = "trusted"`,
		"",
	}, "\n")
	if updated != want {
		t.Fatalf("updated content mismatch:\nwant:\n%s\ngot:\n%s", want, updated)
	}
}

func TestUpsertCodexNotifyKeepsFollowingProjectSectionValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := strings.Join([]string{
		`notify = ["/tmp/go-build/crai", "notify", "--source", "codex"]`,
		`[projects."/Users/turtlekazu/StudioProjects/Messay2"]`,
		`trust_level = "trusted"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := upsertCodexNotify(path, []string{"/opt/homebrew/bin/crai", "notify", "--source", "codex"})
	if err != nil {
		t.Fatalf("upsertCodexNotify returned error: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	got := string(updated)
	if strings.Contains(got, `"codex"][projects.`) {
		t.Fatalf("project section was concatenated onto notify assignment:\n%s", got)
	}
	if !strings.Contains(got, "\n[projects.") {
		t.Fatalf("project section missing newline separator:\n%s", got)
	}
}

func TestUpsertClaudeStopHookWritesValidJSONFromCompactInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	initial := `{"permissions":{"allow":["Bash(echo ok)"]}}`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := upsertClaudeStopHook(path, "/opt/homebrew/bin/crai notify --source claude")
	if err != nil {
		t.Fatalf("upsertClaudeStopHook returned error: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(updated, &root); err != nil {
		t.Fatalf("updated Claude config is invalid JSON: %v\n%s", err, string(updated))
	}
}

func TestUpsertGeminiAfterAgentHookWritesValidJSONFromCompactInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	initial := `{"theme":"dark"}`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := upsertGeminiAfterAgentHook(path, "/opt/homebrew/bin/crai notify --source gemini")
	if err != nil {
		t.Fatalf("upsertGeminiAfterAgentHook returned error: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(updated, &root); err != nil {
		t.Fatalf("updated Gemini config is invalid JSON: %v\n%s", err, string(updated))
	}
}
