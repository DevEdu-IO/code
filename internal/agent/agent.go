// Package agent runs the headless (non-TUI) agent loop: send a message, execute
// the agent's file tools locally, feed results back, repeat until a final answer.
package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	cl "github.com/tghastings/devedu-code/internal/client"
	"github.com/tghastings/devedu-code/internal/tools"
)

// ConfirmFunc is asked before a mutating tool runs; return true to allow.
type ConfirmFunc func(tc cl.ToolCall) bool

const maxSteps = 25 // safety cap on tool round-trips

// Run executes the agent loop for one user message. Tool activity is written to
// `out`; the final answer is returned.
func Run(ctx context.Context, c *cl.Client, message string, out io.Writer, confirm ConfirmFunc) (string, error) {
	resp, err := c.AgentMessage(ctx, "", message)
	if err != nil {
		return "", err
	}

	for step := 0; step < maxSteps; step++ {
		if t := strings.TrimSpace(resp.Text); t != "" && !resp.Done {
			fmt.Fprintln(out, t) // any preamble the agent emits before tool calls
		}
		if resp.Done {
			return resp.Text, nil
		}

		results := make([]cl.ToolResult, 0, len(resp.ToolCalls))
		for _, tc := range resp.ToolCalls {
			results = append(results, runOne(tc, out, confirm))
		}

		resp, err = c.AgentToolResults(ctx, resp.SessionID, resp.InvocationID, results)
		if err != nil {
			return "", err
		}
	}
	return resp.Text, fmt.Errorf("agent did not finish within %d steps", maxSteps)
}

func runOne(tc cl.ToolCall, out io.Writer, confirm ConfirmFunc) cl.ToolResult {
	res := cl.ToolResult{ActionGroup: tc.ActionGroup, Function: tc.Function}

	if tools.NeedsConfirm(tc.Function) && (confirm == nil || !confirm(tc)) {
		fmt.Fprintf(out, "  ✗ skipped %s\n", tools.Summary(tc.Function, tc.Params))
		res.Output = "The user declined to run this tool."
		return res
	}

	output, err := tools.Run(tc.Function, tc.Params)
	if err != nil {
		fmt.Fprintf(out, "  ✗ %s — %v\n", tools.Summary(tc.Function, tc.Params), err)
		res.Output = "error: " + err.Error()
		return res
	}
	fmt.Fprintf(out, "  ✓ %s\n", tools.Summary(tc.Function, tc.Params))
	res.Output = output
	return res
}
