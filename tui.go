package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The UI draws on stderr so stdout stays clean for the selected command.
var (
	re          = lipgloss.NewRenderer(os.Stderr)
	nameStyle   = re.NewStyle().Bold(true)
	selStyle    = re.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	descStyle   = re.NewStyle().Foreground(lipgloss.Color("245"))
	detailStyle = re.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	countStyle  = re.NewStyle().Foreground(lipgloss.Color("240"))
	cursorMark  = re.NewStyle().Foreground(lipgloss.Color("212")).Render("▌")
	boxStyle    = re.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

// boxChrome is the width the box's border and padding take from each row.
const boxChrome = 4

type model struct {
	searcher *Searcher
	total    int
	built    time.Time
	input    textinput.Model
	results  []Result
	cursor   int
	offset   int
	width    int
	height   int
	chosen   string
	quitting bool
	peeking  bool // a man page is open; draw nothing so the picker redraws in place
}

func newModel(s *Searcher, total int, built time.Time, query string) model {
	ti := textinput.New()
	ti.Prompt = "❯ "
	ti.Placeholder = "describe what you want to do…"
	ti.SetValue(query)
	ti.Focus()
	m := model{searcher: s, total: total, built: built, input: ti, width: 80, height: 20}
	m.refresh()
	return m
}

func (m *model) refresh() {
	m.results = m.searcher.Search(m.input.Value(), 200)
	m.cursor, m.offset = 0, 0
}

func (m model) listHeight() int {
	return max(3, min(15, m.height-6))
}

func (m model) Init() tea.Cmd { return textinput.Blink }

type manDone struct{}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Some ptys report 0x0; keep the defaults rather than render nothing.
		if msg.Width > 0 && msg.Height > 0 {
			m.width, m.height = msg.Width, msg.Height
		}
		return m, nil
	case manDone:
		m.peeking = false
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if len(m.results) > 0 {
				m.chosen = m.results[m.cursor].Entry.Name
			}
			m.quitting = true
			return m, tea.Quit
		case "up", "ctrl+p", "ctrl+k":
			m.move(-1)
			return m, nil
		case "down", "ctrl+n", "ctrl+j", "tab":
			m.move(1)
			return m, nil
		case "pgup":
			m.move(-m.listHeight())
			return m, nil
		case "pgdown":
			m.move(m.listHeight())
			return m, nil
		case "ctrl+o":
			if len(m.results) > 0 {
				// Blank the picker first: bubbletea draws the next frame where
				// the command left the cursor, below any lines still on screen.
				m.peeking = true
				c := exec.Command("man", m.results[m.cursor].Entry.Name)
				return m, tea.ExecProcess(c, func(error) tea.Msg { return manDone{} })
			}
			return m, nil
		}
	}
	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.refresh()
	}
	return m, cmd
}

func (m *model) move(delta int) {
	if len(m.results) == 0 {
		return
	}
	m.cursor = max(0, min(len(m.results)-1, m.cursor+delta))
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

func (m model) View() string {
	if m.quitting || m.peeking {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.input.View())
	b.WriteString("\n")

	// The list keeps a fixed height, framed so it reads as a list even
	// before there's anything in it.
	h := m.listHeight()
	rows := make([]string, h)
	for i := m.offset; i < min(len(m.results), m.offset+h); i++ {
		rows[i-m.offset] = m.renderRow(i)
	}
	b.WriteString(boxStyle.Width(m.width - 2).Render(strings.Join(rows, "\n")))
	b.WriteString("\n")

	if len(m.results) > 0 {
		b.WriteString(truncate(detailStyle.Render(details(m.results[m.cursor].Entry)), m.width))
	}
	b.WriteString("\n")
	status := fmt.Sprintf("%d/%d", len(m.results), m.total)
	if strings.TrimSpace(m.input.Value()) == "" {
		status = fmt.Sprintf("%d commands last indexed %s", m.total, m.built.Local().Format("Jan 2, 3:04PM"))
	}
	b.WriteString(countStyle.Render(status + " · enter select · ctrl+o man page · esc quit"))
	return b.String()
}

func (m model) renderRow(i int) string {
	e := m.results[i].Entry
	mark, ns := "  ", nameStyle
	if i == m.cursor {
		mark, ns = cursorMark+" ", selStyle
	}
	name := ns.Render(e.Name)
	used := 2 + lipgloss.Width(name) + 2
	desc := e.Desc()
	if room := m.width - boxChrome - used - 1; room > 0 {
		desc = truncatePlain(desc, room)
	} else {
		desc = ""
	}
	return mark + name + "  " + descStyle.Render(desc)
}

// details describes the highlighted entry as "source: where · more",
// e.g. "system: /usr/bin/cal · man cal(1)" or "alias for: git status".
func details(e *Entry) string {
	if e.Kind == "alias" {
		return e.Extra
	}
	var parts []string
	if e.Path != "" {
		parts = append(parts, e.Path)
	}
	if e.Pkg != "" && e.Pkg != e.Name {
		parts = append(parts, "from "+e.Pkg)
	}
	if e.ManSect != "" {
		parts = append(parts, fmt.Sprintf("man %s(%s)", e.Name, e.ManSect))
	}
	if len(parts) == 0 {
		return e.Source()
	}
	return e.Source() + ": " + strings.Join(parts, " · ")
}

func truncatePlain(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func truncate(styled string, n int) string {
	return lipgloss.NewStyle().MaxWidth(n).Render(styled)
}

func runTUI(s *Searcher, total int, built time.Time, query string) (string, error) {
	p := tea.NewProgram(newModel(s, total, built, query), tea.WithOutput(os.Stderr), tea.WithInputTTY())
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	return final.(model).chosen, nil
}
