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

// SyncStatsMsg carries aggregate sync statistics.
type SyncStatsMsg struct {
	TotalSynced int
	TotalBytes  string
	TotalErrors int
}

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
	totalBytes    string
	totalErrors   int
	width, height int
	filter        string
	filtering     bool
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewDashboard returns a DashboardModel configured with the given local and
// remote paths.
func NewDashboard(local, remote string) DashboardModel {
	return DashboardModel{
		local:      local,
		remote:     remote,
		status:     "watching",
		totalBytes: "0B",
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
		// Prepend; cap at 100.
		m.events = append([]SyncEvent{evt}, m.events...)
		if len(m.events) > 100 {
			m.events = m.events[:100]
		}
		if evt.Status == "synced" {
			m.lastSync = evt.Time
		}
		return m, nil

	case SyncStatsMsg:
		m.totalSynced = msg.TotalSynced
		m.totalBytes = msg.TotalBytes
		m.totalErrors = msg.TotalErrors
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
	case "/":
		m.filtering = true
		m.filter = ""
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

// View renders the dashboard.
func (m DashboardModel) View() string {
	var b strings.Builder

	// --- Header ---
	header := titleStyle.Render(" esync ") + dimStyle.Render(strings.Repeat("─", max(0, m.width-8)))
	b.WriteString(header + "\n")
	b.WriteString(fmt.Sprintf("  %s → %s\n", m.local, m.remote))

	// Status line
	statusIcon, statusText := m.statusDisplay()
	agoText := ""
	if !m.lastSync.IsZero() {
		ago := time.Since(m.lastSync).Truncate(time.Second)
		agoText = fmt.Sprintf(" (synced %s ago)", ago)
	}
	b.WriteString(fmt.Sprintf("  %s %s%s\n", statusIcon, statusText, dimStyle.Render(agoText)))
	b.WriteString("\n")

	// --- Recent events ---
	b.WriteString("  " + titleStyle.Render("Recent") + " " + dimStyle.Render(strings.Repeat("─", max(0, m.width-11))) + "\n")

	filtered := m.filteredEvents()
	visible := min(len(filtered), max(0, m.height-10))
	for i := 0; i < visible; i++ {
		evt := filtered[i]
		b.WriteString("  " + m.renderEvent(evt) + "\n")
	}
	b.WriteString("\n")

	// --- Stats ---
	b.WriteString("  " + titleStyle.Render("Stats") + " " + dimStyle.Render(strings.Repeat("─", max(0, m.width-10))) + "\n")
	stats := fmt.Sprintf("  %d synced │ %s total │ %d errors",
		m.totalSynced, m.totalBytes, m.totalErrors)
	b.WriteString(stats + "\n")
	b.WriteString("\n")

	// --- Help / filter ---
	if m.filtering {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  filter: %s█  (enter apply  esc clear)", m.filter)))
	} else {
		help := "  q quit  p pause  r full resync  l logs  d dry-run  / filter"
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
	switch m.status {
	case "watching":
		return statusSynced.Render("●"), statusSynced.Render("Watching")
	case "syncing":
		return statusSyncing.Render("⟳"), statusSyncing.Render("Syncing")
	case "paused":
		return dimStyle.Render("⏸"), dimStyle.Render("Paused")
	case "error":
		return statusError.Render("✗"), statusError.Render("Error")
	default:
		return "?", m.status
	}
}

// renderEvent formats a single sync event line.
func (m DashboardModel) renderEvent(evt SyncEvent) string {
	switch evt.Status {
	case "synced":
		name := padRight(evt.File, 30)
		return statusSynced.Render("✓") + " " + name + dimStyle.Render(fmt.Sprintf("%8s  %5s", evt.Size, evt.Duration.Truncate(100*time.Millisecond)))
	case "syncing":
		name := padRight(evt.File, 30)
		return statusSyncing.Render("⟳") + " " + name + statusSyncing.Render("syncing...")
	case "error":
		name := padRight(evt.File, 30)
		return statusError.Render("✗") + " " + name + statusError.Render("error")
	default:
		return evt.File
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

// padRight pads s with spaces to width n, truncating if necessary.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
