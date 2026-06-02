// Command devedu is the DevEdu Code CLI: a terminal coding assistant that
// authenticates with your DevEdu API key and talks to an AI model (via DevEdu's
// Amazon Bedrock proxy) — like Claude Code, powered by your DevEdu account.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tghastings/devedu-code/internal/agent"
	"github.com/tghastings/devedu-code/internal/client"
	"github.com/tghastings/devedu-code/internal/config"
	"github.com/tghastings/devedu-code/internal/repl"
	"github.com/tghastings/devedu-code/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=...".
// It stays "dev" for local builds.
var version = "dev"

func main() {
	apiKey := flag.String("api-key", "", "DevEdu API key (or set DEVEDU_API_KEY)")
	baseURL := flag.String("url", "", "DevEdu base URL (or set DEVEDU_API_URL)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("devedu", version)
		return
	}

	cfg := config.Load(*apiKey, *baseURL)
	stdin := bufio.NewReader(os.Stdin)

	// First run (or no key configured anywhere): set up interactively and save.
	if cfg.APIKey == "" {
		if !stdinIsTerminal() {
			fmt.Fprintln(os.Stderr, "devedu: no API key found. Set DEVEDU_API_KEY, pass --api-key, or run interactively to set one up.")
			os.Exit(1)
		}
		var err error
		cfg, err = firstRunSetup(cfg, stdin, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "devedu:", err)
			os.Exit(1)
		}
	}

	c := client.New(cfg.BaseURL, cfg.APIKey)
	ctx := context.Background()

	// One-shot mode: any trailing args are sent as a single prompt. Try the
	// agentic loop (tool use); fall back to plain chat if no agent is configured.
	if args := flag.Args(); len(args) > 0 {
		msg := strings.Join(args, " ")
		reply, err := agent.Run(ctx, c, msg, os.Stderr, confirmTool)
		if errors.Is(err, client.ErrNoAgent) {
			reply, err = c.Chat(ctx, msg)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "devedu:", err)
			os.Exit(1)
		}
		fmt.Println(reply)
		return
	}

	// Humans get the full-screen TUI; pipes/CI fall back to a line-based loop.
	if stdinIsTerminal() {
		if err := tui.Run(c, hostLabel(cfg.BaseURL)); err != nil {
			fmt.Fprintln(os.Stderr, "devedu:", err)
			os.Exit(1)
		}
		return
	}
	if err := repl.Run(ctx, c, stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "devedu:", err)
		os.Exit(1)
	}
}

// hostLabel strips the scheme for a tidy header (https://app.devedu.io → app.devedu.io).
func hostLabel(url string) string {
	for _, p := range []string{"https://", "http://"} {
		url = strings.TrimPrefix(url, p)
	}
	return strings.TrimRight(url, "/")
}

// firstRunSetup walks a new user through entering their DevEdu URL and API key,
// then stores them locally so they never have to do it again.
func firstRunSetup(cfg config.Config, in *bufio.Reader, out io.Writer) (config.Config, error) {
	fmt.Fprintln(out, "Welcome to DevEdu Code 👋  Let's get you set up (you only do this once).")
	fmt.Fprintln(out, "Find your API key in DevEdu — students and teachers both have one:")
	fmt.Fprintln(out, "  sign in → account menu (your email) → API key.")
	fmt.Fprintln(out)

	fmt.Fprintf(out, "DevEdu URL [%s]: ", cfg.BaseURL)
	if url := readLine(in); url != "" {
		cfg.BaseURL = url
	}

	for cfg.APIKey == "" {
		fmt.Fprint(out, "Paste your API key: ")
		cfg.APIKey = readLine(in)
		if cfg.APIKey == "" {
			fmt.Fprintln(out, "  (an API key is required — copy it from the API key page)")
		}
	}

	path, err := cfg.Save()
	if err != nil {
		return cfg, fmt.Errorf("could not save your settings: %w", err)
	}
	fmt.Fprintf(out, "\nSaved to %s — you're all set.\n\n", path)
	return cfg, nil
}

func readLine(in *bufio.Reader) string {
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}

// confirmTool prompts before a one-shot agent action with side effects (writing
// a file or running a shell command). Never auto-approves in a non-interactive
// context.
func confirmTool(tc client.ToolCall) bool {
	if !stdinIsTerminal() {
		return false
	}
	switch tc.Function {
	case "run_command":
		fmt.Fprintf(os.Stderr, "  $  run: %s\n     run this command? [y/N] ", tc.Params["command"])
	default:
		fmt.Fprintf(os.Stderr, "  ✏  write %s (%d bytes)? [y/N] ", tc.Params["path"], len(tc.Params["content"]))
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// stdinIsTerminal reports whether we can prompt the user (stdin is a TTY).
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func usage() {
	fmt.Fprint(os.Stderr, `DevEdu Code — terminal coding assistant

Usage:
  devedu                 start an interactive session
  devedu "your prompt"   one-shot: print the answer and exit

On first run, devedu asks for your API key and saves it to your config dir.
Override anytime with flags or environment:

  --api-key   DevEdu API key   (env DEVEDU_API_KEY)
  --url       DevEdu base URL  (env DEVEDU_API_URL)   default: `+config.DefaultBaseURL+`

Get your API key at <your-devedu>/account/api_key
`)
}
