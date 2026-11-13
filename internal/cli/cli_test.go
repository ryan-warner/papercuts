package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAvailableAgentCandidatesRequiresAgentDir(t *testing.T) {
	home := t.TempDir()
	if got := availableAgentCandidates(home); len(got) != 0 {
		t.Fatalf("expected no candidates in empty home, got %d", len(got))
	}

	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := availableAgentCandidates(home)
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}
	if got[0].name != "Codex" || got[1].name != "OpenCode" {
		t.Fatalf("unexpected candidates: %s, %s", got[0].name, got[1].name)
	}
}

func TestAppendAgentInstructionsCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	changed, err := appendAgentInstructions(path)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected file to be created")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `papercuts add "<concise observation>"`) {
		t.Fatalf("created file is missing instructions: %q", data)
	}
}

func TestAppendAgentInstructionsPreservesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("existing guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := appendAgentInstructions(path)
	if err != nil || !changed {
		t.Fatalf("expected append, changed=%v err=%v", changed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "existing guidance\n") {
		t.Fatalf("existing content was not preserved: %q", data)
	}
}

func TestAppendAgentInstructionsSkipsConfiguredFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(path, []byte("already mentions papercuts add\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := appendAgentInstructions(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected configured file to be skipped")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("configured file was modified")
	}
}
