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

// tickMsg is sent on every one-second tick for periodic refresh.
type tickMsg time.Time

// SyncEventMsg carries a single sync event into the TUI.
type SyncEventMsg SyncEvent

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SyncEvent represents a single file sync operation.
type SyncEvent struct {
	File     string
	Size     string
	Duration time.Duration
	Status   string // "synced", "syncing", "error"
	Time     time.Time
}

// DashboardModel is the main TUI view showing sync status and recent events.
type DashboardModel struct {
	local, remote string
	status        string // "watching", "syncing", "paused", "error"
	lastSync      time.Time
	events        []SyncEvent
	totalSynced   int
	totalErrors   int
	width, height int
	filter        string
	filtering     bool
	offset        int
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewDashboard returns a DashboardModel configured with the given local and
// remote paths.
func NewDashboard(local, remote string) DashboardModel {
	return DashboardModel{
		local:  local,
		remote: remote,
		status: "watching",
	}
}

// ---------------------------------------------------------------------------
// tea.Model interface
// ---------------------------------------------------------------------------

// Init starts the periodic tick timer.
func (m DashboardModel) Init() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages for the dashboard view.
func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}
		return m.updateNormal(msg)

	case tickMsg:
		// Re-arm the ticker.
		return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})

	case SyncEventMsg:
		evt := SyncEvent(msg)

		// Status-only messages update the header, not the event list
		if strings.HasPrefix(evt.Status, "status:") {
			m.status = strings.TrimPrefix(evt.Status, "status:")
			return m, nil
		}

		// Prepend event; cap at 500.
		m.events = append([]SyncEvent{evt}, m.events...)
		if len(m.events) > 500 {
			m.events = m.events[:500]
		}
		if evt.Status == "synced" {
			m.lastSync = evt.Time
			m.totalSynced++
		} else if evt.Status == "error" {
			m.totalErrors++
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
func (m DashboardModel) updateNormal(msg tea.KeyMsg) (DashboardModel, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "p":
		if m.status == "paused" {
			m.status = "watching"
		} else {
			m.status = "paused"
		}
	case "r":
		return m, func() tea.Msg { return ResyncRequestMsg{} }
	case "j", "down":
		filtered := m.filteredEvents()
		maxOffset := max(0, len(filtered)-m.eventViewHeight())
		if m.offset < maxOffset {
			m.offset++
		}
	case "k", "up":
		if m.offset > 0 {
			m.offset--
		}
	case "/":
		m.filtering = true
		m.filter = ""
		m.offset = 0
	}
	return m, nil
}

// updateFiltering handles keys when in filtering mode.
func (m DashboardModel) updateFiltering(msg tea.KeyMsg) (DashboardModel, tea.Cmd) {
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
	return m, nil
}

// eventViewHeight returns the number of event rows that fit in the terminal.
// Layout: header (3 lines) + "Recent" header (1) + stats section (3) + help (1) = 8 fixed.
func (m DashboardModel) eventViewHeight() int {
	return max(1, m.height-8)
}

// View renders the dashboard.
func (m DashboardModel) View() string {
	var b strings.Builder

	// --- Header (3 lines) ---
	header := titleStyle.Render(" esync ") + dimStyle.Render(strings.Repeat("─", max(0, m.width-8)))
	b.WriteString(header + "\n")
	b.WriteString(fmt.Sprintf("  %s → %s\n", m.local, m.remote))

	statusIcon, statusText := m.statusDisplay()
	agoText := ""
	if !m.lastSync.IsZero() {
		ago := time.Since(m.lastSync).Truncate(time.Second)
		agoText = fmt.Sprintf(" (synced %s ago)", ago)
	}
	b.WriteString(fmt.Sprintf("  %s %s%s\n", statusIcon, statusText, dimStyle.Render(agoText)))

	// --- Recent events ---
	b.WriteString("  " + titleStyle.Render("Recent") + " " + dimStyle.Render(strings.Repeat("─", max(0, m.width-11))) + "\n")

	filtered := m.filteredEvents()
	vh := m.eventViewHeight()
	start := m.offset
	end := min(start+vh, len(filtered))

	for i := start; i < end; i++ {
		b.WriteString("  " + m.renderEvent(filtered[i]) + "\n")
	}
	// Pad empty rows
	for i := end - start; i < vh; i++ {
		b.WriteString("\n")
	}

	// --- Stats (2 lines) ---
	b.WriteString("  " + titleStyle.Render("Stats") + " " + dimStyle.Render(strings.Repeat("─", max(0, m.width-10))) + "\n")
	stats := fmt.Sprintf("  %d events │ %d errors", m.totalSynced, m.totalErrors)
	b.WriteString(stats + "\n")

	// --- Help (1 line) ---
	if m.filtering {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  filter: %s█  (enter apply  esc clear)", m.filter)))
	} else {
		help := "  q quit  p pause  r resync  ↑↓ scroll  l logs  / filter"
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

// statusDisplay returns the icon and styled text for the current status.
func (m DashboardModel) statusDisplay() (string, string) {
	switch {
	case m.status == "watching":
		return statusSynced.Render("●"), statusSynced.Render("Watching")
	case strings.HasPrefix(m.status, "syncing"):
		label := "Syncing"
		if pct := strings.TrimPrefix(m.status, "syncing "); pct != m.status {
			label = "Syncing " + pct
		}
		return statusSyncing.Render("⟳"), statusSyncing.Render(label)
	case m.status == "paused":
		return dimStyle.Render("⏸"), dimStyle.Render("Paused")
	case m.status == "error":
		return statusError.Render("✗"), statusError.Render("Error")
	default:
		return "?", m.status
	}
}

// renderEvent formats a single sync event line.
func (m DashboardModel) renderEvent(evt SyncEvent) string {
	ts := dimStyle.Render(evt.Time.Format("15:04:05"))
	switch evt.Status {
	case "synced":
		name := padRight(abbreviatePath(evt.File, 30), 30)
		detail := ""
		if evt.Size != "" {
			detail = dimStyle.Render(fmt.Sprintf("%18s  %s", evt.Size, evt.Duration.Truncate(100*time.Millisecond)))
		}
		return ts + "  " + statusSynced.Render("✓") + " " + name + detail
	case "error":
		name := padRight(abbreviatePath(evt.File, 30), 30)
		return ts + "  " + statusError.Render("✗") + " " + name + statusError.Render("error")
	default:
		return ts + "  " + evt.File
	}
}

// filteredEvents returns events matching the current filter (case-insensitive).
func (m DashboardModel) filteredEvents() []SyncEvent {
	if m.filter == "" {
		return m.events
	}
	lf := strings.ToLower(m.filter)
	var out []SyncEvent
	for _, evt := range m.events {
		if strings.Contains(strings.ToLower(evt.File), lf) {
			out = append(out, evt)
		}
	}
	return out
}

// abbreviatePath shortens a file path to fit within maxLen by replacing
// leading directory segments with their first letter.
// "internal/syncer/syncer.go" → "i/s/syncer.go"
func abbreviatePath(p string, maxLen int) string {
	if len(p) <= maxLen {
		return p
	}
	parts := strings.Split(p, "/")
	if len(parts) <= 1 {
		return p
	}
	// Shorten directory segments from the left, keep the filename intact.
	for i := 0; i < len(parts)-1; i++ {
		if len(parts[i]) > 1 {
			parts[i] = parts[i][:1]
		}
		if len(strings.Join(parts, "/")) <= maxLen {
			break
		}
	}
	return strings.Join(parts, "/")
}

// padRight pads s with spaces to width n, truncating if necessary.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
