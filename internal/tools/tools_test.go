package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := Run("write_file", map[string]string{"path": "a.txt", "content": "hello"})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "wrote 5 bytes") {
		t.Errorf("write summary = %q", out)
	}
	got, err := Run("read_file", map[string]string{"path": "a.txt"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "hello" {
		t.Errorf("read = %q, want hello", got)
	}
}

func TestWriteCreatesParentDirs(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := Run("write_file", map[string]string{"path": "pkg/sub/f.go", "content": "x"}); err != nil {
		t.Fatalf("write nested: %v", err)
	}
	if _, err := os.Stat("pkg/sub/f.go"); err != nil {
		t.Errorf("nested file not created: %v", err)
	}
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("p"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	out, err := Run("list_dir", map[string]string{"path": "."})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "x.go") || !strings.Contains(out, "sub/") {
		t.Errorf("list = %q", out)
	}
}

func TestSandboxRejectsEscape(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, p := range []string{"../secret", "../../etc/passwd"} {
		if _, err := Run("read_file", map[string]string{"path": p}); err == nil {
			t.Errorf("expected %q to be rejected", p)
		}
	}
}

func TestUnknownTool(t *testing.T) {
	if _, err := Run("rm_rf", nil); err == nil {
		t.Error("expected unknown-tool error")
	}
}

func TestNeedsConfirm(t *testing.T) {
	for _, n := range []string{"write_file", "run_command"} {
		if !NeedsConfirm(n) {
			t.Errorf("%s should need confirmation", n)
		}
	}
	for _, n := range []string{"read_file", "list_dir"} {
		if NeedsConfirm(n) {
			t.Errorf("%s should not need confirmation", n)
		}
	}
}

func TestRunCommand(t *testing.T) {
	out, err := Run("run_command", map[string]string{"command": "echo hi"})
	if err != nil {
		t.Fatalf("run echo: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("output = %q, want it to contain hi", out)
	}
}

func TestRunCommandCapturesNonZeroExit(t *testing.T) {
	out, err := Run("run_command", map[string]string{"command": "echo oops; exit 3"})
	if err != nil {
		t.Fatalf("non-zero exit should not be a tool error: %v", err)
	}
	if !strings.Contains(out, "oops") || !strings.Contains(out, "exited 3") {
		t.Errorf("output = %q, want output plus exit code", out)
	}
}

func TestRunCommandEmpty(t *testing.T) {
	if _, err := Run("run_command", map[string]string{"command": "  "}); err == nil {
		t.Error("expected error for empty command")
	}
}
