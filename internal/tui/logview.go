package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// LogEntryMsg carries a log entry into the TUI.
type LogEntryMsg LogEntry

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// LogEntry represents a single log line.
type LogEntry struct {
	Time    time.Time
	Level   string // "INF", "WRN", "ERR"
	Message string
}

// LogViewModel is a scrollable log view. It is not a standalone tea.Model;
// its Update and View methods are called by AppModel.
type LogViewModel struct {
	entries   []LogEntry
	offset    int
	width     int
	height    int
	filter    string
	filtering bool
	follow    bool // tail mode: auto-scroll to bottom on new entries
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewLogView returns an empty LogViewModel with follow mode enabled.
func NewLogView() LogViewModel {
	return LogViewModel{follow: true}
}

// ---------------------------------------------------------------------------
// Update / View (not tea.Model — managed by AppModel)
// ---------------------------------------------------------------------------

// Update handles messages for the log view.
func (m LogViewModel) Update(msg tea.Msg) (LogViewModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}
		return m.updateNormal(msg)

	case LogEntryMsg:
		entry := LogEntry(msg)
		m.entries = append(m.entries, entry)
		if len(m.entries) > 1000 {
			m.entries = m.entries[len(m.entries)-1000:]
		}
		if m.follow {
			filtered := m.filteredEntries()
			m.offset = max(0, len(filtered)-m.viewHeight())
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	return m, nil
}

// updateNormal handles keys when NOT in filtering mode.
func (m LogViewModel) updateNormal(msg tea.KeyMsg) (LogViewModel, tea.Cmd) {
	filtered := m.filteredEntries()
	maxOffset := max(0, len(filtered)-m.viewHeight())
	switch msg.String() {
	case "up", "k":
		m.follow = false
		if m.offset > 0 {
			m.offset--
		}
	case "down", "j":
		if m.offset < maxOffset {
			m.offset++
		}
	case "pgup":
		m.follow = false
		m.offset = max(0, m.offset-m.viewHeight())
	case "pgdown":
		m.offset = min(maxOffset, m.offset+m.viewHeight())
	case "g":
		m.follow = false
		m.offset = 0
	case "G":
		m.offset = maxOffset
	case "f":
		m.follow = !m.follow
		if m.follow {
			m.offset = maxOffset
		}
	case "/":
		m.filtering = true
		m.filter = ""
		m.offset = 0
	}
	return m, nil
}

// updateFiltering handles keys when in filtering mode.
func (m LogViewModel) updateFiltering(msg tea.KeyMsg) (LogViewModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyEscape:
		m.filter = ""
		m.filtering = false
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
		}
	}
	m.offset = 0
	return m, nil
}

// View renders the log view.
func (m LogViewModel) View() string {
	var b strings.Builder

	// Header
	header := titleStyle.Render(" esync ─ logs ") + dimStyle.Render(strings.Repeat("─", max(0, m.width-15)))
	b.WriteString(header + "\n")

	// Log lines
	filtered := m.filteredEntries()
	vh := m.viewHeight()
	start := m.offset
	end := min(start+vh, len(filtered))

	for i := start; i < end; i++ {
		entry := filtered[i]
		ts := entry.Time.Format("15:04:05")
		lvl := m.styledLevel(entry.Level)
		b.WriteString(fmt.Sprintf("  %s %s %s\n", dimStyle.Render(ts), lvl, entry.Message))
	}

	// Pad remaining lines
	rendered := end - start
	for i := rendered; i < vh; i++ {
		b.WriteString("\n")
	}

	// Help / filter
	if m.filtering {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  filter: %s█  (enter apply  esc clear)", m.filter)))
	} else {
		help := "  ↑↓/pgup/pgdn scroll  g top  G end  f follow  / filter  l back  q quit"
		if m.follow {
			help += "  [FOLLOW]"
		}
		if m.filter != "" {
			help += fmt.Sprintf("  [filter: %s]", m.filter)
		}
		b.WriteString(helpStyle.Render(help))
	}
	b.WriteString("\n")

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// viewHeight returns the number of log lines visible (total height minus
// header and help bar).
func (m LogViewModel) viewHeight() int {
	return max(1, m.height-3)
}

// styledLevel returns the level string with appropriate color.
func (m LogViewModel) styledLevel(level string) string {
	switch level {
	case "INF":
		return statusSynced.Render("INF")
	case "WRN":
		return statusSyncing.Render("WRN")
	case "ERR":
		return statusError.Render("ERR")
	default:
		return dimStyle.Render(level)
	}
}

// filteredEntries returns log entries matching the current filter
// (case-insensitive match on Message).
func (m LogViewModel) filteredEntries() []LogEntry {
	if m.filter == "" {
		return m.entries
	}
	lf := strings.ToLower(m.filter)
	var out []LogEntry
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.Message), lf) {
			out = append(out, e)
		}
	}
	return out
}
