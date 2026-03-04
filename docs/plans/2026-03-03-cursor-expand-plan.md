# Cursor Navigation & Inline Expand Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add cursor-based navigation to the TUI dashboard event list with inline expand/collapse to reveal individual files inside directory groups.

**Architecture:** Add a `Files []string` field to `SyncEvent` so grouped events carry their children. Add `cursor` and `expanded` state to `DashboardModel`. Render focused rows with a highlight marker and expanded children indented below. Use dynamic column widths based on terminal width.

**Tech Stack:** Go, Bubbletea, Lipgloss (all already in use)

---

### Task 1: Add `Files` field to SyncEvent

**Files:**
- Modify: `internal/tui/dashboard.go:26-32`

**Step 1: Add the field**

In the `SyncEvent` struct, add a `Files` field after `Status`:

```go
type SyncEvent struct {
	File     string
	Size     string
	Duration time.Duration
	Status   string // "synced", "syncing", "error"
	Time     time.Time
	Files    []string // individual file paths for directory groups
}
```

**Step 2: Build and verify**

Run: `go build ./...`
Expected: clean build, no errors (field is unused so far, which is fine)

**Step 3: Commit**

```
feat: add Files field to SyncEvent for directory group children
```

---

### Task 2: Populate `Files` when building grouped events

**Files:**
- Modify: `cmd/sync.go:402-451` (groupFilesByTopLevel and its caller)

**Step 1: Add `files` field to `groupedEvent`**

```go
type groupedEvent struct {
	name  string   // "cmd/" or "main.go"
	count int      // number of files (1 for root files)
	bytes int64    // total bytes
	files []string // individual file paths within the group
}
```

**Step 2: Collect file names in `groupFilesByTopLevel`**

In the directory branch of the loop, append `f.Name` to the group's `files` slice. In the output loop, copy files for multi-file groups:

```go
func groupFilesByTopLevel(files []syncer.FileEntry) []groupedEvent {
	dirMap := make(map[string]*groupedEvent)
	dirFirstFile := make(map[string]string)
	var rootFiles []groupedEvent
	var dirOrder []string

	for _, f := range files {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) == 1 {
			rootFiles = append(rootFiles, groupedEvent{
				name:  f.Name,
				count: 1,
				bytes: f.Bytes,
			})
		} else {
			dir := parts[0] + "/"
			if g, ok := dirMap[dir]; ok {
				g.count++
				g.bytes += f.Bytes
				g.files = append(g.files, f.Name)
			} else {
				dirMap[dir] = &groupedEvent{
					name:  dir,
					count: 1,
					bytes: f.Bytes,
					files: []string{f.Name},
				}
				dirFirstFile[dir] = f.Name
				dirOrder = append(dirOrder, dir)
			}
		}
	}

	var out []groupedEvent
	for _, dir := range dirOrder {
		g := *dirMap[dir]
		if g.count == 1 {
			g.name = dirFirstFile[dir]
			g.files = nil // no need to expand single files
		}
		out = append(out, g)
	}
	out = append(out, rootFiles...)
	return out
}
```

**Step 3: Pass files into `SyncEvent` in the handler**

In `runTUI` handler (around line 237), set the `Files` field:

```go
for _, g := range groups {
	file := g.name
	bytes := g.bytes
	if totalGroupBytes == 0 && result.BytesTotal > 0 && totalGroupFiles > 0 {
		bytes = result.BytesTotal * int64(g.count) / int64(totalGroupFiles)
	}
	size := formatSize(bytes)
	if g.count > 1 {
		size = fmt.Sprintf("%d files  %s", g.count, formatSize(bytes))
	}
	syncCh <- tui.SyncEvent{
		File:     file,
		Size:     size,
		Duration: result.Duration,
		Status:   "synced",
		Time:     now,
		Files:    g.files,
	}
}
```

**Step 4: Build and run tests**

Run: `go build ./... && go test ./...`
Expected: clean build, all tests pass

**Step 5: Commit**

```
feat: populate SyncEvent.Files with individual paths for directory groups
```

---

### Task 3: Add cursor and expanded state to DashboardModel

**Files:**
- Modify: `internal/tui/dashboard.go:34-46`

**Step 1: Add cursor and expanded fields**

```go
type DashboardModel struct {
	local, remote string
	status        string
	lastSync      time.Time
	events        []SyncEvent
	totalSynced   int
	totalErrors   int
	width, height int
	filter        string
	filtering     bool
	offset        int
	cursor        int          // index into filtered events
	expanded      map[int]bool // keyed by index in unfiltered events slice
}
```

**Step 2: Initialize expanded map in NewDashboard**

```go
func NewDashboard(local, remote string) DashboardModel {
	return DashboardModel{
		local:    local,
		remote:   remote,
		status:   "watching",
		expanded: make(map[int]bool),
	}
}
```

**Step 3: Build**

Run: `go build ./...`
Expected: clean build

**Step 4: Commit**

```
feat: add cursor and expanded state to DashboardModel
```

---

### Task 4: Cursor navigation and expand/collapse key handling

**Files:**
- Modify: `internal/tui/dashboard.go` — `updateNormal` method (lines 121-149)

**Step 1: Replace scroll-only navigation with cursor-based navigation**

Replace the `updateNormal` method:

```go
func (m DashboardModel) updateNormal(msg tea.KeyMsg) (DashboardModel, tea.Cmd) {
	filtered := m.filteredEvents()
	maxCursor := max(0, len(filtered)-1)

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
		if m.cursor < maxCursor {
			m.cursor++
		}
		m.ensureCursorVisible()
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		m.ensureCursorVisible()
	case "enter", "right":
		if m.cursor < len(filtered) {
			evt := filtered[m.cursor]
			if len(evt.Files) > 0 {
				idx := m.unfilteredIndex(m.cursor)
				if idx >= 0 {
					m.expanded[idx] = !m.expanded[idx]
				}
			}
		}
	case "left", "esc":
		if m.cursor < len(filtered) {
			idx := m.unfilteredIndex(m.cursor)
			if idx >= 0 {
				delete(m.expanded, idx)
			}
		}
	case "/":
		m.filtering = true
		m.filter = ""
		m.cursor = 0
		m.offset = 0
	}
	return m, nil
}
```

**Step 2: Add `unfilteredIndex` helper**

This maps a filtered-list index back to the index in `m.events`:

```go
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
```

**Step 3: Add `ensureCursorVisible` helper**

This adjusts `offset` so the cursor row (plus any expanded children above it) stays in view:

```go
// ensureCursorVisible adjusts offset so the cursor row is within the viewport.
func (m *DashboardModel) ensureCursorVisible() {
	vh := m.eventViewHeight()
	// Count visible lines up to and including cursor
	visibleLine := 0
	filtered := m.filteredEvents()
	for i := 0; i <= m.cursor && i < len(filtered); i++ {
		if i >= m.offset {
			visibleLine++
		}
		idx := m.unfilteredIndex(i)
		if idx >= 0 && m.expanded[idx] {
			if i >= m.offset {
				visibleLine += len(filtered[i].Files)
			}
		}
	}
	// Scroll down if cursor is below viewport
	for visibleLine > vh && m.offset < m.cursor {
		// Subtract lines for the row we're scrolling past
		old := m.offset
		m.offset++
		visibleLine--
		oldIdx := m.unfilteredIndex(old)
		if oldIdx >= 0 && m.expanded[oldIdx] {
			visibleLine -= len(filtered[old].Files)
		}
	}
	// Scroll up if cursor is above viewport
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
}
```

**Step 4: Clamp cursor when new events arrive**

In the `SyncEventMsg` handler in `Update` (around line 88-109), after prepending the event, shift expanded indices and keep cursor valid:

```go
case SyncEventMsg:
	evt := SyncEvent(msg)

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
```

**Step 5: Build**

Run: `go build ./...`
Expected: clean build

**Step 6: Commit**

```
feat: cursor navigation with expand/collapse for dashboard events
```

---

### Task 5: Render focused row and expanded children with aligned columns

**Files:**
- Modify: `internal/tui/dashboard.go` — `View`, `renderEvent`, `eventViewHeight` methods
- Modify: `internal/tui/styles.go` — add focused style

**Step 1: Add focused style to styles.go**

```go
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	statusSynced  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusSyncing = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	statusError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	focusedStyle  = lipgloss.NewStyle().Bold(true)
)
```

**Step 2: Update `renderEvent` to accept focus flag and use dynamic name width**

Replace the `renderEvent` method:

```go
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
			detail = dimStyle.Render(fmt.Sprintf("  %s  %s", evt.Size, evt.Duration.Truncate(100*time.Millisecond)))
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
```

**Step 3: Add `renderChildren` method**

```go
// renderChildren renders the expanded file list for a directory group.
func (m DashboardModel) renderChildren(files []string, nameWidth int) []string {
	var lines []string
	for _, f := range files {
		// Indent to align under the parent name column:
		// "  " (marker) + "HH:MM:SS" (8) + "  " (2) + icon (1) + " " (1) = 14 chars prefix
		prefix := strings.Repeat(" ", 14)
		name := abbreviatePath(f, nameWidth-2)
		lines = append(lines, prefix+"└ "+dimStyle.Render(name))
	}
	return lines
}
```

**Step 4: Update `nameWidth` helper**

```go
// nameWidth returns the dynamic width for the file name column based on
// terminal width. Reserves space for: marker(2) + timestamp(8) + gap(2) +
// icon(1) + gap(1) + [name] + gap(2) + size/duration(~30) = ~46 fixed.
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
```

**Step 5: Update `View` to render cursor and expanded children**

Replace the event rendering loop in `View`:

```go
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
		children := m.renderChildren(filtered[i].Files, nw)
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
```

**Step 6: Update `eventViewHeight`**

The fixed layout adds 2 chars for the marker prefix per row. The header/stats/help line count stays the same (8 lines). No change needed to the calculation — it still returns `m.height - 8`.

**Step 7: Update the help line**

Replace the help text in the non-filtering branch:

```go
help := "  q quit  p pause  r resync  ↑↓ navigate  enter expand  l logs  / filter"
```

**Step 8: Build and test manually**

Run: `go build ./... && go test ./...`
Expected: clean build, all tests pass

**Step 9: Commit**

```
feat: render focused row highlight and inline expanded children with aligned columns
```

---

### Task 6: Test the groupFilesByTopLevel change

**Files:**
- Create: `cmd/sync_test.go`

**Step 1: Write test for grouping with files populated**

```go
package cmd

import (
	"testing"

	"github.com/louloulibs/esync/internal/syncer"
)

func TestGroupFilesByTopLevel_MultiFile(t *testing.T) {
	files := []syncer.FileEntry{
		{Name: "cmd/sync.go", Bytes: 100},
		{Name: "cmd/root.go", Bytes: 200},
		{Name: "main.go", Bytes: 50},
	}

	groups := groupFilesByTopLevel(files)

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}

	// First group: cmd/ with 2 files
	g := groups[0]
	if g.name != "cmd/" {
		t.Errorf("group[0].name = %q, want %q", g.name, "cmd/")
	}
	if g.count != 2 {
		t.Errorf("group[0].count = %d, want 2", g.count)
	}
	if len(g.files) != 2 {
		t.Fatalf("group[0].files has %d entries, want 2", len(g.files))
	}
	if g.files[0] != "cmd/sync.go" || g.files[1] != "cmd/root.go" {
		t.Errorf("group[0].files = %v, want [cmd/sync.go cmd/root.go]", g.files)
	}

	// Second group: root file
	g = groups[1]
	if g.name != "main.go" {
		t.Errorf("group[1].name = %q, want %q", g.name, "main.go")
	}
	if g.files != nil {
		t.Errorf("group[1].files should be nil for root file, got %v", g.files)
	}
}

func TestGroupFilesByTopLevel_SingleFileDir(t *testing.T) {
	files := []syncer.FileEntry{
		{Name: "internal/config/config.go", Bytes: 300},
	}

	groups := groupFilesByTopLevel(files)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}

	g := groups[0]
	// Single-file dir uses full path
	if g.name != "internal/config/config.go" {
		t.Errorf("name = %q, want full path", g.name)
	}
	// No files for single-file groups
	if g.files != nil {
		t.Errorf("files should be nil for single-file dir, got %v", g.files)
	}
}
```

**Step 2: Run tests**

Run: `go test ./cmd/ -run TestGroupFilesByTopLevel -v`
Expected: both tests pass

**Step 3: Commit**

```
test: add tests for groupFilesByTopLevel with files field
```

---

### Task 7: Final build and integration check

**Step 1: Full build and test suite**

Run: `go build ./... && go test ./...`
Expected: all pass

**Step 2: Build release binary**

Run: `GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o esync-darwin-arm64 .`
Expected: binary produced

**Step 3: Commit**

```
chore: verify build after cursor navigation feature
```
