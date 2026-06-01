// Package repl runs the interactive DevEdu Code session.
package repl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// Chatter is anything that can answer a prompt (the API client).
type Chatter interface {
	Chat(ctx context.Context, prompt string) (string, error)
}

// Run starts an interactive loop. Because the DevEdu API is currently single-turn,
// we keep a running transcript and resend it each turn so the model has context.
func Run(ctx context.Context, c Chatter, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var transcript strings.Builder

	fmt.Fprintln(out, "DevEdu Code — your terminal coding assistant.")
	fmt.Fprintln(out, "Type a request and press Enter. /exit to quit, /reset to clear context.")

	for {
		fmt.Fprint(out, "\n\033[1;35m›\033[0m ")
		if !scanner.Scan() {
			break // EOF (Ctrl-D)
		}
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case line == "/exit" || line == "/quit":
			return nil
		case line == "/reset":
			transcript.Reset()
			fmt.Fprintln(out, "context cleared.")
			continue
		}

		transcript.WriteString("User: " + line + "\n")

		fmt.Fprint(out, "\033[2mthinking…\033[0m")
		reply, err := c.Chat(ctx, transcript.String())
		fmt.Fprint(out, "\r\033[K") // clear the "thinking…" line
		if err != nil {
			fmt.Fprintf(out, "\033[31merror:\033[0m %v\n", err)
			continue
		}

		transcript.WriteString("Assistant: " + reply + "\n")
		fmt.Fprintln(out, reply)
	}
	return scanner.Err()
}
