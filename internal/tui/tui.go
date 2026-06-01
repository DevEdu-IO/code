// Package tui is the full-screen interactive interface for DevEdu Code, built
// with Bubble Tea. It drives the agent loop: send a message, run the agent's file
// tools locally (read/list inline, writes confirmed), feed results back, repeat.
package tui

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"

	cl "github.com/tghastings/devedu-code/internal/client"
	"github.com/tghastings/devedu-code/internal/tools"
)

// Run launches the interactive TUI and blocks until the user quits.
func Run(c *cl.Client, host string) error {
	p := tea.NewProgram(newModel(c, host), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ---- styling -------------------------------------------------------------

var (
	pink   = lipgloss.Color("#f706b0")
	violet = lipgloss.Color("#9b7bff")
	muted  = lipgloss.Color("#8a86a0")
	body   = lipgloss.Color("#e9e6f5")
	green  = lipgloss.Color("#3ecf8e")
	red    = lipgloss.Color("#ff6b6b")

	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(pink).Padding(0, 1)
	hostStyle   = lipgloss.NewStyle().Foreground(muted)
	youLabel    = lipgloss.NewStyle().Bold(true).Foreground(pink)
	botLabel    = lipgloss.NewStyle().Bold(true).Foreground(violet)
	bodyStyle   = lipgloss.NewStyle().Foreground(body)
	toolStyle   = lipgloss.NewStyle().Foreground(muted)
	toolOK      = lipgloss.NewStyle().Foreground(green)
	errStyle    = lipgloss.NewStyle().Foreground(red)
	helpStyle   = lipgloss.NewStyle().Foreground(muted)
	confirmBox  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(violet).Padding(0, 1)
	inputStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(violet).Padding(0, 1)
	welcomeText = lipgloss.NewStyle().Foreground(muted).Italic(true)
)

// ---- model ---------------------------------------------------------------

type role int

const (
	roleYou role = iota
	roleBot
	roleTool
	roleErr
)

type message struct {
	role role
	text string
}

type agentRespMsg struct {
	resp *cl.AgentResponse
	err  error
}

type chatResultMsg struct {
	reply string
	err   error
}

type model struct {
	client *cl.Client
	host   string

	msgs    []message
	vp      viewport.Model
	ta      textarea.Model
	sp      spinner.Model
	running bool
	ready   bool
	width   int
	height  int

	md      *glamour.TermRenderer
	mdWidth int

	// agent loop state
	useAgent     bool
	session      string
	invocationID string
	pending      []cl.ToolCall
	results      []cl.ToolResult
	confirm      *cl.ToolCall
}

func newModel(c *cl.Client, host string) model {
	ta := textarea.New()
	ta.Placeholder = "Ask DevEdu Code about your code, or to make a change…"
	ta.Prompt = "❯ "
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.CharLimit = 0
	ta.Focus()
	ta.KeyMap.InsertNewline.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(pink)

	return model{client: c, host: host, ta: ta, sp: sp, useAgent: true}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.sp.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.renderTranscript()
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		// A pending write confirmation captures the next keypress.
		if m.confirm != nil {
			if msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
			approved := false
			switch msg.String() {
			case "y", "Y":
				approved = true
			}
			cmd := m.resolveConfirm(approved)
			return m, cmd
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.running {
				return m, nil
			}
			input := strings.TrimSpace(m.ta.Value())
			if input == "" {
				return m, nil
			}
			if input == "/exit" || input == "/quit" {
				return m, tea.Quit
			}
			m.ta.Reset()
			m.msgs = append(m.msgs, message{role: roleYou, text: input})
			m.running = true
			m.renderTranscript()
			return m, tea.Batch(m.startCmd(input), m.sp.Tick)
		}

	case agentRespMsg:
		if errors.Is(msg.err, cl.ErrNoAgent) {
			// Instance has no agent; fall back to plain chat for this and future turns.
			m.useAgent = false
			cmd := m.chatCmd()
			return m, tea.Batch(cmd, m.sp.Tick)
		}
		if msg.err != nil {
			m.running = false
			m.msgs = append(m.msgs, message{role: roleErr, text: msg.err.Error()})
			m.renderTranscript()
			return m, nil
		}
		resp := msg.resp
		m.session = resp.SessionID
		if strings.TrimSpace(resp.Text) != "" {
			m.msgs = append(m.msgs, message{role: roleBot, text: resp.Text})
		}
		if resp.Done {
			m.running = false
			m.renderTranscript()
			return m, nil
		}
		m.invocationID = resp.InvocationID
		m.pending = resp.ToolCalls
		m.results = nil
		cmd := m.processTools()
		return m, cmd

	case chatResultMsg:
		m.running = false
		if msg.err != nil {
			m.msgs = append(m.msgs, message{role: roleErr, text: msg.err.Error()})
		} else if strings.TrimSpace(msg.reply) != "" {
			m.msgs = append(m.msgs, message{role: roleBot, text: msg.reply})
		}
		m.renderTranscript()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		if m.running {
			return m, cmd
		}
		return m, nil
	}

	var tcmd, vcmd tea.Cmd
	m.ta, tcmd = m.ta.Update(msg)
	m.vp, vcmd = m.vp.Update(msg)
	return m, tea.Batch(tcmd, vcmd)
}

func (m model) View() string {
	if !m.ready {
		return "starting DevEdu Code…"
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center, headerStyle.Render("DevEdu Code"), " ", hostStyle.Render(m.host))

	status := " "
	switch {
	case m.confirm != nil:
		status = confirmBox.Render(" "+tools.Summary(m.confirm.Function, m.confirm.Params)+" — apply? ") + helpStyle.Render("  [y] yes   [any other key] no")
	case m.running:
		status = m.sp.View() + helpStyle.Render(" working…")
	}

	help := helpStyle.Render("enter: send  ·  ↑/↓ pgup/pgdn: scroll  ·  /exit or ctrl+c: quit")
	input := inputStyle.Width(m.width - 2).Render(m.ta.View())
	return lipgloss.JoinVertical(lipgloss.Left, header, m.vp.View(), status, input, help)
}

// ---- commands ------------------------------------------------------------

func (m model) startCmd(input string) tea.Cmd {
	if m.useAgent {
		return m.agentMsgCmd(input)
	}
	return m.chatCmd()
}

func (m model) agentMsgCmd(input string) tea.Cmd {
	c, session := m.client, m.session
	return func() tea.Msg {
		resp, err := c.AgentMessage(context.Background(), session, input)
		return agentRespMsg{resp: resp, err: err}
	}
}

func (m model) agentResultsCmd() tea.Cmd {
	c, session, inv, results := m.client, m.session, m.invocationID, m.results
	return func() tea.Msg {
		resp, err := c.AgentToolResults(context.Background(), session, inv, results)
		return agentRespMsg{resp: resp, err: err}
	}
}

func (m model) chatCmd() tea.Cmd {
	c, convo := m.client, m.transcriptForModel()
	return func() tea.Msg {
		reply, err := c.Chat(context.Background(), convo)
		return chatResultMsg{reply: reply, err: err}
	}
}

// ---- tool execution ------------------------------------------------------

// processTools runs read/list tools inline and pauses on the first write for
// confirmation. When the batch is exhausted it resumes the agent with the results.
func (m *model) processTools() tea.Cmd {
	for len(m.pending) > 0 {
		tc := m.pending[0]
		if tools.NeedsConfirm(tc.Function) {
			m.confirm = &tc
			m.renderTranscript()
			return nil // wait for the user's y/n
		}
		m.pending = m.pending[1:]
		m.runTool(tc)
	}
	m.renderTranscript()
	return m.agentResultsCmd()
}

// resolveConfirm handles the user's y/n on a pending write, then continues.
func (m *model) resolveConfirm(approved bool) tea.Cmd {
	tc := *m.confirm
	m.confirm = nil
	if len(m.pending) > 0 {
		m.pending = m.pending[1:]
	}
	if approved {
		m.runTool(tc)
	} else {
		m.msgs = append(m.msgs, message{role: roleTool, text: "✗ declined " + tools.Summary(tc.Function, tc.Params)})
		m.results = append(m.results, cl.ToolResult{ActionGroup: tc.ActionGroup, Function: tc.Function, Output: "The user declined to run this tool."})
	}
	return m.processTools()
}

func (m *model) runTool(tc cl.ToolCall) {
	out, err := tools.Run(tc.Function, tc.Params)
	if err != nil {
		m.msgs = append(m.msgs, message{role: roleErr, text: "✗ " + tools.Summary(tc.Function, tc.Params) + " — " + err.Error()})
		out = "error: " + err.Error()
	} else {
		m.msgs = append(m.msgs, message{role: roleTool, text: "✓ " + tools.Summary(tc.Function, tc.Params)})
	}
	m.results = append(m.results, cl.ToolResult{ActionGroup: tc.ActionGroup, Function: tc.Function, Output: out})
}

// ---- rendering -----------------------------------------------------------

func (m model) transcriptForModel() string {
	var b strings.Builder
	for _, msg := range m.msgs {
		switch msg.role {
		case roleYou:
			b.WriteString("User: " + msg.text + "\n")
		case roleBot:
			b.WriteString("Assistant: " + msg.text + "\n")
		}
	}
	return b.String()
}

func (m *model) layout() {
	const header, status, help = 1, 1, 1
	inputH := m.ta.Height() + 2
	vpH := m.height - header - status - inputH - help
	if vpH < 3 {
		vpH = 3
	}
	m.ta.SetWidth(m.width - 4)
	if m.vp.Width == 0 && m.vp.Height == 0 {
		m.vp = viewport.New(m.width, vpH)
	} else {
		m.vp.Width = m.width
		m.vp.Height = vpH
	}
}

func (m *model) renderTranscript() {
	wrap := lipgloss.NewStyle().Width(max(20, m.vp.Width-1))
	if len(m.msgs) == 0 {
		m.vp.SetContent(wrap.Render(welcomeText.Render(
			"Hi! I'm DevEdu Code. I can read, list, and (with your OK) write files in this directory to help you build. Ask me anything about your code. /exit to quit.")))
		return
	}
	var b strings.Builder
	for i, msg := range m.msgs {
		if i > 0 {
			b.WriteString("\n")
		}
		switch msg.role {
		case roleYou:
			b.WriteString(youLabel.Render("you") + "\n" + wrap.Render(bodyStyle.Render(msg.text)) + "\n")
		case roleBot:
			b.WriteString(botLabel.Render("DevEdu") + "\n" + m.renderMarkdown(msg.text) + "\n")
		case roleTool:
			label := toolStyle
			if strings.HasPrefix(msg.text, "✓") {
				label = toolOK
			}
			b.WriteString(label.Render("  "+msg.text) + "\n")
		case roleErr:
			b.WriteString(wrap.Render(errStyle.Render(msg.text)) + "\n")
		}
	}
	m.vp.SetContent(b.String())
	m.vp.GotoBottom()
}

// markdownStyle is glamour's dark theme with inline code toned down: the default
// renders `code` as a grey, space-padded chip, which turns a comma-separated list
// of filenames into scattered blocks. We drop the background and padding so inline
// code is just softly colored text that flows with the sentence.
func markdownStyle() ansi.StyleConfig {
	s := styles.DarkStyleConfig
	lavender := "#b9adff"
	s.Code = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{Color: &lavender},
	}
	// No left indent on the document so replies sit flush with the "DevEdu" label.
	s.Document.Margin = uintPtr(0)
	return s
}

func uintPtr(u uint) *uint { return &u }

func (m *model) renderMarkdown(text string) string {
	width := max(20, m.vp.Width-1)
	if m.md == nil || m.mdWidth != width {
		// Use a fixed dark style, NOT WithAutoStyle: auto-detection queries the
		// terminal background (OSC 11) and, since this runs inside the Bubble Tea
		// loop, the terminal's reply leaks into the textarea as junk input.
		r, err := glamour.NewTermRenderer(glamour.WithStyles(markdownStyle()), glamour.WithWordWrap(width))
		if err != nil {
			return lipgloss.NewStyle().Width(width).Render(bodyStyle.Render(text))
		}
		m.md, m.mdWidth = r, width
	}
	out, err := m.md.Render(text)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(bodyStyle.Render(text))
	}
	return strings.Trim(out, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
