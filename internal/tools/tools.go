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
		return s[:maxRead] + "\n...[truncated]"
	}
	return s
}

const maxRead = 200 * 1024 // cap reads so a huge file can't blow up the prompt

// safePath resolves p under the working directory and rejects anything that
// escapes it — via .., absolute paths, OR symlinks. The symlink-resolved target
// (or, for a file that doesn't exist yet, its nearest real ancestor) must stay
// within the symlink-resolved working root; otherwise a symlink inside the
// sandbox could be followed out of it.
func safePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("missing path")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		root = filepath.Clean(cwd)
	}

	abs := filepath.Clean(filepath.Join(root, p))
	real, err := resolveReal(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the working directory", p)
	}
	return real, nil
}

// resolveReal returns the fully symlink-resolved absolute path for p. If p does
// not exist yet (e.g. a new file being written), it resolves the nearest
// existing ancestor and re-appends the remaining components, so the location is
// validated against the real path of its parent rather than a lexical guess.
func resolveReal(p string) (string, error) {
	cur, rest := p, ""
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			if rest == "" {
				return real, nil
			}
			return filepath.Join(real, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve path %q", p)
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
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
		return string(data[:maxRead]) + "\n...[truncated]", nil
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

// ConfirmText is the FULL, untruncated description shown when asking the user to
// approve a side-effecting tool. Unlike Summary (a short transcript label), it
// never hides any of the command — informed consent requires seeing exactly what
// will run.
func ConfirmText(name string, params map[string]string) string {
	switch name {
	case "write_file":
		return fmt.Sprintf("write %s (%d bytes)", params["path"], len(params["content"]))
	case "run_command":
		return "run: " + params["command"]
	default:
		return Summary(name, params)
	}
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
			c = c[:57] + "..."
		}
		return "run `" + c + "`"
	default:
		return name
	}
}
