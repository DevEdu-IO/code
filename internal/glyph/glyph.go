// Package glyph provides the small UI symbols used across the CLI, with ASCII
// fallbacks on Windows — whose default console fonts often can't render the
// Unicode versions (❯, ✓, ✗, ✏, …), showing boxes or blanks instead.
package glyph

import "runtime"

var (
	Prompt    = "❯" // input prompt
	Check     = "✓" // tool succeeded
	Cross     = "✗" // tool failed / declined
	Pencil    = "✏" // write confirmation
	Ellipsis  = "…"
	Bullet    = "·"
	ArrowUp   = "↑"
	ArrowDown = "↓"
	Dash      = "—"
)

func init() {
	if runtime.GOOS != "windows" {
		return
	}
	Prompt = ">"
	Check = "+"
	Cross = "x"
	Pencil = "*"
	Ellipsis = "..."
	Bullet = "-"
	ArrowUp = "^"
	ArrowDown = "v"
	Dash = "-"
}
