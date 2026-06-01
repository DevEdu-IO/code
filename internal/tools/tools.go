// Package tools executes the agent's file tools locally, sandboxed to the current
// working directory. read_file / list_dir are read-only; write_file mutates and
// should be confirmed by the caller before it's run.
package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// NeedsConfirm reports whether a tool has side effects and must be confirmed by
// the user before it runs (writes and shell commands).
func NeedsConfirm(name string) bool { return name == "write_file" || name == "run_command" }

// Run dispatches a tool by name. Returns the tool's textual output (which is fed
// back to the agent) or an error.
func Run(name string, params map[string]string) (string, error) {
	switch name {
	case "read_file":
		return readFile(params["path"])
	case "list_dir":
		return listDir(params["path"])
	case "write_file":
		return writeFile(params["path"], params["content"])
	case "run_command":
		return runCommand(params["command"])
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

const commandTimeout = 120 * time.Second

// runCommand runs a shell command in the working directory and returns its
// combined output. A non-zero exit is normal "data" for the agent (test
// failures, etc.), so it's returned as output with the exit code, not an error.
func runCommand(command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("missing command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	result := truncate(string(out))

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return result + fmt.Sprintf("\n[timed out after %s]", commandTimeout), nil
	case isExit(err):
		return result + fmt.Sprintf("\n[exited %d]", exitCode(err)), nil
	case err != nil:
		return "", err // couldn't start the command at all
	default:
		return result, nil
	}
}

func isExit(err error) bool { _, ok := err.(*exec.ExitError); return ok }

func exitCode(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func truncate(s string) string {
	if len(s) > maxRead {
		return s[:maxRead] + "\n…[truncated]"
	}
	return s
}

const maxRead = 200 * 1024 // cap reads so a huge file can't blow up the prompt

// safePath resolves p under the working directory and rejects anything that
// escapes it (via .. or absolute paths).
func safePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("missing path")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs := filepath.Clean(filepath.Join(cwd, p))
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the working directory", p)
	}
	return abs, nil
}

func readFile(p string) (string, error) {
	abs, err := safePath(p)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if len(data) > maxRead {
		return string(data[:maxRead]) + "\n…[truncated]", nil
	}
	return string(data), nil
}

func listDir(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		p = "."
	}
	abs, err := safePath(p)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() {
			n += "/"
		}
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(names, "\n"), nil
}

func writeFile(p, content string) (string, error) {
	abs, err := safePath(p)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), p), nil
}

// Summary is a short, human-readable label for the TUI ("read main.go").
func Summary(name string, params map[string]string) string {
	switch name {
	case "read_file":
		return "read " + params["path"]
	case "list_dir":
		p := params["path"]
		if p == "" {
			p = "."
		}
		return "list " + p
	case "write_file":
		return "write " + params["path"]
	case "run_command":
		c := params["command"]
		if len(c) > 60 {
			c = c[:57] + "…"
		}
		return "run `" + c + "`"
	default:
		return name
	}
}
