package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
)

var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Glamour's default dark style renders H2-H6 as a literal "## "/"### " prefix.
// markdownStyle() must strip those so headings are interpreted, not shown raw.
// Assert on the visible text (Glamour interleaves ANSI codes in the raw string).
func TestMarkdownHeadingsHaveNoHashPrefix(t *testing.T) {
	r, err := glamour.NewTermRenderer(glamour.WithStyles(markdownStyle()), glamour.WithWordWrap(80))
	if err != nil {
		t.Fatalf("renderer: %v", err)
	}
	out, err := r.Render("# Level 1\n\n## Level 2 Header\n\n### Level 3 Header\n\nbody text\n")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	visible := ansiCodes.ReplaceAllString(out, "")

	for _, hashes := range []string{"# ", "## ", "### "} {
		if strings.Contains(visible, hashes) {
			t.Errorf("heading hash prefix %q not stripped:\n%s", hashes, visible)
		}
	}
	for _, want := range []string{"Level 1", "Level 2 Header", "Level 3 Header", "body text"} {
		if !strings.Contains(visible, want) {
			t.Errorf("expected %q in rendered output:\n%s", want, visible)
		}
	}
}
