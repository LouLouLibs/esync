package tui

import (
	"fmt"
	"path/filepath"
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
	Files      []string // individual file paths for directory groups (max 10)
	FileCount  int      // total file count in group (may exceed len(Files))
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
	cursor        int          // index into filtered events
	childCursor   int          // -1 = on parent row, >=0 = index into expanded Files
	expanded      map[int]bool // keyed by index in unfiltered events slice
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewDashboard returns a DashboardModel configured with the given local and
// remote paths.
func NewDashboard(local, remote string) DashboardModel {
	return DashboardModel{
		local:       local,
		remote:      remote,
		status:      "watching",
		childCursor: -1,
		expanded:    make(map[int]bool),
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

		// Shift expanded indices since we're prepending
		newExpanded := make(map[int]bool, len(m.expanded))
		for idx, v := range m.expanded {
			newExpanded[idx+1] = v
		}
		m.expanded = newExpanded
		m.childCursor = -1

		// Prepend event; cap at 500.
		m.events = append([]SyncEvent{evt}, m.events...)
		if len(m.events) > 500 {
			m.events = m.events[:500]
			// Clean up expanded entries beyond 500
			for idx := range m.expanded {
				if idx >= 500 {
					delete(m.expanded, idx)
				}
			}
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
	filtered := m.filteredEvents()

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
		m.moveDown(filtered)
		m.ensureCursorVisible()
	case "k", "up":
		m.moveUp(filtered)
		m.ensureCursorVisible()
	case "enter", "right":
		if m.cursor < len(filtered) {
			evt := filtered[m.cursor]
			if len(evt.Files) > 0 {
				idx := m.unfilteredIndex(m.cursor)
				if idx >= 0 {
					m.expanded[idx] = !m.expanded[idx]
					m.childCursor = -1
				}
			}
		}
	case "left":
		if m.cursor < len(filtered) {
			idx := m.unfilteredIndex(m.cursor)
			if idx >= 0 {
				delete(m.expanded, idx)
				m.childCursor = -1
			}
		}
	case "v":
		if m.cursor >= len(filtered) {
			break
		}
		evt := filtered[m.cursor]
		idx := m.unfilteredIndex(m.cursor)

		// On a child file — open it
		if m.childCursor >= 0 && m.childCursor < len(evt.Files) {
			path := filepath.Join(m.local, evt.Files[m.childCursor])
			return m, func() tea.Msg { return OpenFileMsg{Path: path} }
		}

		// On a parent with children — expand (same as enter)
		if len(evt.Files) > 0 {
			if idx >= 0 && !m.expanded[idx] {
				m.expanded[idx] = true
				return m, nil
			}
			// Already expanded but cursor on parent — do nothing
			return m, nil
		}

		// Single-file event — open it
		path := filepath.Join(m.local, evt.File)
		return m, func() tea.Msg { return OpenFileMsg{Path: path} }
	case "e":
		return m, func() tea.Msg { return EditConfigMsg{} }
	case "/":
		m.filtering = true
		m.filter = ""
		m.cursor = 0
		m.offset = 0
		m.childCursor = -1
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

// moveDown advances cursor one visual row, entering expanded children.
func (m *DashboardModel) moveDown(filtered []SyncEvent) {
	if m.cursor >= len(filtered) {
		return
	}
	idx := m.unfilteredIndex(m.cursor)
	evt := filtered[m.cursor]

	// Currently on parent of expanded event — enter children
	if m.childCursor == -1 && idx >= 0 && m.expanded[idx] && len(evt.Files) > 0 {
		m.childCursor = 0
		return
	}

	// Currently on a child — advance within children
	if m.childCursor >= 0 {
		if m.childCursor < len(evt.Files)-1 {
			m.childCursor++
			return
		}
		// Past last child — move to next event
		if m.cursor < len(filtered)-1 {
			m.cursor++
			m.childCursor = -1
		}
		return
	}

	// Normal: move to next event
	if m.cursor < len(filtered)-1 {
		m.cursor++
		m.childCursor = -1
	}
}

// moveUp moves cursor one visual row, entering expanded children from bottom.
func (m *DashboardModel) moveUp(filtered []SyncEvent) {
	// Currently on a child — move up within children
	if m.childCursor > 0 {
		m.childCursor--
		return
	}

	// On first child — move back to parent
	if m.childCursor == 0 {
		m.childCursor = -1
		return
	}

	// On a parent row — move to previous event
	if m.cursor <= 0 {
		return
	}
	m.cursor--
	m.childCursor = -1

	// If previous event is expanded, land on its last child
	prevIdx := m.unfilteredIndex(m.cursor)
	prevEvt := filtered[m.cursor]
	if prevIdx >= 0 && m.expanded[prevIdx] && len(prevEvt.Files) > 0 {
		m.childCursor = len(prevEvt.Files) - 1
	}
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
	nw := m.nameWidth()

	// Render events from offset, counting visible lines including expanded children
	linesRendered := 0
	for i := m.offset; i < len(filtered) && linesRendered < vh; i++ {
		focused := i == m.cursor
		b.WriteString(m.renderEvent(filtered[i], focused, nw) + "\n")
		linesRendered++

		// Render expanded children
		idx := m.unfilteredIndex(i)
		if idx >= 0 && m.expanded[idx] && len(filtered[i].Files) > 0 {
			focusedChild := -1
			if i == m.cursor {
				focusedChild = m.childCursor
			}
			children := m.renderChildren(filtered[i].Files, filtered[i].FileCount, nw, focusedChild)
			for _, child := range children {
				if linesRendered >= vh {
					break
				}
				b.WriteString(child + "\n")
				linesRendered++
			}
		}
	}
	// Pad empty rows
	for i := linesRendered; i < vh; i++ {
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
		help := "  " +
			helpKey("q") + helpDesc("quit") +
			helpKey("p") + helpDesc("pause") +
			helpKey("r") + helpDesc("resync") +
			helpKey("↑↓") + helpDesc("navigate") +
			helpKey("enter") + helpDesc("expand") +
			helpKey("v") + helpDesc("view") +
			helpKey("e") + helpDesc("config") +
			helpKey("l") + helpDesc("logs") +
			helpKey("/") + helpDesc("filter")
		if m.filter != "" {
			help += dimStyle.Render(fmt.Sprintf("  [filter: %s]", m.filter))
		}
		b.WriteString(help)
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
// nameWidth is the column width for the file name.
func (m DashboardModel) renderEvent(evt SyncEvent, focused bool, nameWidth int) string {
	ts := dimStyle.Render(evt.Time.Format("15:04:05"))
	marker := "  "
	if focused {
		marker = "> "
	}

	switch evt.Status {
	case "synced":
		name := padRight(abbreviatePath(evt.File, nameWidth), nameWidth)
		if focused {
			name = focusedStyle.Render(name)
		}
		detail := ""
		if evt.Size != "" {
			dur := fmt.Sprintf("%s", evt.Duration.Truncate(100*time.Millisecond))
			count := ""
			if evt.FileCount > 1 {
				count = fmt.Sprintf("%d files", evt.FileCount)
			}
			detail = dimStyle.Render(fmt.Sprintf("%8s  %7s  %5s", count, evt.Size, dur))
		}
		icon := statusSynced.Render("✓")
		return marker + ts + "  " + icon + " " + name + detail
	case "error":
		name := padRight(abbreviatePath(evt.File, nameWidth), nameWidth)
		if focused {
			name = focusedStyle.Render(name)
		}
		return marker + ts + "  " + statusError.Render("✗") + " " + name + statusError.Render("error")
	default:
		return marker + ts + "  " + evt.File
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

// nameWidth returns the dynamic width for the file name column.
// Reserves space for: marker(2) + timestamp(8) + gap(2) + icon(1) + gap(1) +
// [name] + gap(2) + size/duration(~30) = ~46 fixed chars.
func (m DashboardModel) nameWidth() int {
	w := m.width - 46
	if w < 30 {
		w = 30
	}
	if w > 60 {
		w = 60
	}
	return w
}

// unfilteredIndex returns the index in m.events corresponding to the i-th
// item in the filtered event list, or -1 if out of range.
func (m DashboardModel) unfilteredIndex(filteredIdx int) int {
	if m.filter == "" {
		return filteredIdx
	}
	lf := strings.ToLower(m.filter)
	count := 0
	for i, evt := range m.events {
		if strings.Contains(strings.ToLower(evt.File), lf) {
			if count == filteredIdx {
				return i
			}
			count++
		}
	}
	return -1
}

// ensureCursorVisible adjusts offset so the cursor row is within the viewport.
func (m *DashboardModel) ensureCursorVisible() {
	vh := m.eventViewHeight()

	// Scroll up if cursor is above viewport
	if m.cursor < m.offset {
		m.offset = m.cursor
		return
	}

	// Count visible lines from offset to cursor (inclusive),
	// including expanded children.
	filtered := m.filteredEvents()
	lines := 0
	for i := m.offset; i <= m.cursor && i < len(filtered); i++ {
		lines++ // the event row itself
		idx := m.unfilteredIndex(i)
		if idx >= 0 && m.expanded[idx] {
			if i == m.cursor && m.childCursor >= 0 {
				// Only count up to the focused child
				lines += m.childCursor + 1
			} else {
				lines += expandedLineCount(filtered[i])
			}
		}
	}

	// Scroll down if cursor line is beyond viewport
	for lines > vh && m.offset < m.cursor {
		// Subtract lines for the row we scroll past
		lines-- // the event row
		idx := m.unfilteredIndex(m.offset)
		if idx >= 0 && m.expanded[idx] {
			lines -= expandedLineCount(filtered[m.offset])
		}
		m.offset++
	}
}

// expandedLineCount returns the number of child lines rendered for an event:
// one per stored file, plus a "+N more" line if FileCount exceeds len(Files).
func expandedLineCount(evt SyncEvent) int {
	n := len(evt.Files)
	if evt.FileCount > n {
		n++ // the "+N more" line
	}
	return n
}

// renderChildren renders the expanded file list for a directory group.
// totalCount is the original number of files in the group (may exceed len(files)).
// focusedChild is the index of the focused child (-1 if none).
func (m DashboardModel) renderChildren(files []string, totalCount int, nameWidth int, focusedChild int) []string {
	// Prefix aligns under the parent name column:
	// marker(2) + timestamp(8) + gap(2) + icon(1) + gap(1) = 14 chars
	prefix := strings.Repeat(" ", 14)
	var lines []string
	for i, f := range files {
		name := abbreviatePath(f, nameWidth-2)
		if i == focusedChild {
			lines = append(lines, prefix+"> "+focusedStyle.Render(name))
		} else {
			lines = append(lines, prefix+"  "+dimStyle.Render(name))
		}
	}
	if remaining := totalCount - len(files); remaining > 0 {
		lines = append(lines, prefix+dimStyle.Render(fmt.Sprintf("  +%d more", remaining)))
	}
	return lines
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

// helpKey renders a keyboard shortcut key in normal (bright) style.
func helpKey(k string) string { return helpKeyStyle.Render(k) + " " }

// helpDesc renders a shortcut description in dim style with spacing.
func helpDesc(d string) string { return dimStyle.Render(d) + "  " }

// padRight pads s with spaces to width n, truncating if necessary.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
