# TUI Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the dashboard event list to show grouped top-level results with timestamps, fill the terminal, scroll, and wire up the `r` resync key.

**Architecture:** Replace per-file "syncing"/"synced" events with a status message for the header and grouped directory-level events. Add scroll offset to the dashboard. Add a resync channel so the TUI can trigger a full sync. Accumulate stats in the handler.

**Tech Stack:** Go, Bubbletea, Lipgloss

---

### Task 1: Add new message types to app.go

**Files:**
- Modify: `internal/tui/app.go`

**Step 1: Add SyncStatusMsg and ResyncRequestMsg types and resync channel**

In `internal/tui/app.go`, add after the `view` constants (line 16):

```go
// SyncStatusMsg updates the header status without adding an event.
type SyncStatusMsg string

// ResyncRequestMsg signals that the user pressed 'r' for a full resync.
type ResyncRequestMsg struct{}
```

Add `resyncCh` field to `AppModel` struct (after `logEntries`):

```go
resyncCh   chan struct{}
```

Initialize it in `NewApp`:

```go
resyncCh:   make(chan struct{}, 1),
```

Add accessor:

```go
// ResyncChan returns a channel that receives when the user requests a full resync.
func (m *AppModel) ResyncChan() <-chan struct{} {
	return m.resyncCh
}
```

**Step 2: Handle new messages in AppModel.Update**

In the `Update` method's switch, add cases before the `SyncEventMsg` case:

```go
case SyncStatusMsg:
	m.dashboard.status = string(msg)
	return m, nil

case ResyncRequestMsg:
	select {
	case m.resyncCh <- struct{}{}:
	default:
	}
	return m, nil
```

**Step 3: Build and verify compilation**

Run: `go build ./...`
Expected: success

**Step 4: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat(tui): add SyncStatusMsg, ResyncRequestMsg, resync channel"
```

---

### Task 2: Update dashboard — timestamps, scrolling, fill terminal

**Files:**
- Modify: `internal/tui/dashboard.go`

**Step 1: Add scroll offset field**

Add `offset int` to `DashboardModel` struct (after `filtering`).

**Step 2: Add j/k/up/down scroll keys and r resync key to updateNormal**

Replace `updateNormal`:

```go
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
```

**Step 3: Add eventViewHeight helper**

```go
// eventViewHeight returns the number of event rows that fit in the terminal.
// Layout: header (3 lines) + "Recent" header (1) + stats section (3) + help (1) = 8 fixed.
func (m DashboardModel) eventViewHeight() int {
	return max(1, m.height-8)
}
```

**Step 4: Rewrite View to fill terminal with timestamps**

Replace the `View` method:

```go
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
	stats := fmt.Sprintf("  %d synced │ %s total │ %d errors",
		m.totalSynced, m.totalBytes, m.totalErrors)
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
```

**Step 5: Update renderEvent to include timestamp**

Replace `renderEvent`:

```go
func (m DashboardModel) renderEvent(evt SyncEvent) string {
	ts := dimStyle.Render(evt.Time.Format("15:04:05"))
	switch evt.Status {
	case "synced":
		name := padRight(evt.File, 30)
		detail := ""
		if evt.Size != "" {
			detail = dimStyle.Render(fmt.Sprintf("%8s  %s", evt.Size, evt.Duration.Truncate(100*time.Millisecond)))
		}
		return ts + "  " + statusSynced.Render("✓") + " " + name + detail
	case "error":
		name := padRight(evt.File, 30)
		return ts + "  " + statusError.Render("✗") + " " + name + statusError.Render("error")
	default:
		return ts + "  " + evt.File
	}
}
```

**Step 6: Remove "syncing" case from renderEvent**

The "syncing" case is no longer needed — it was removed in step 5 above.

**Step 7: Build and verify**

Run: `go build ./...`
Expected: success

**Step 8: Commit**

```bash
git add internal/tui/dashboard.go
git commit -m "feat(tui): timestamps, scrolling, fill terminal, resync key"
```

---

### Task 3: Top-level grouping and stats accumulation in handler

**Files:**
- Modify: `cmd/sync.go`

**Step 1: Add groupFiles helper**

Add after `formatSize` at the bottom of `cmd/sync.go`:

```go
// groupedEvent represents a top-level directory or root file for the TUI.
type groupedEvent struct {
	name  string // "cmd/" or "main.go"
	count int    // number of files (1 for root files)
	bytes int64  // total bytes
}

// groupFilesByTopLevel collapses file entries into top-level directories
// and root files. "cmd/sync.go" + "cmd/init.go" → one entry "cmd/" with count=2.
func groupFilesByTopLevel(files []syncer.FileEntry) []groupedEvent {
	dirMap := make(map[string]*groupedEvent)
	var rootFiles []groupedEvent
	var dirOrder []string

	for _, f := range files {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) == 1 {
			// Root-level file
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
			} else {
				dirMap[dir] = &groupedEvent{name: dir, count: 1, bytes: f.Bytes}
				dirOrder = append(dirOrder, dir)
			}
		}
	}

	var out []groupedEvent
	for _, dir := range dirOrder {
		out = append(out, *dirMap[dir])
	}
	out = append(out, rootFiles...)
	return out
}
```

**Step 2: Rewrite TUI handler to use grouping, stats, and status messages**

Replace the entire `handler` closure inside `runTUI`:

```go
	var totalSynced int
	var totalBytes int64
	var totalErrors int

	handler := func() {
		// Update header status
		syncCh <- tui.SyncEvent{Status: "status:syncing"}

		result, err := s.Run()
		now := time.Now()

		if err != nil {
			totalErrors++
			syncCh <- tui.SyncEvent{
				File:   "sync error",
				Status: "error",
				Time:   now,
			}
			// Reset header
			syncCh <- tui.SyncEvent{Status: "status:watching"}
			return
		}

		// Group files by top-level directory
		groups := groupFilesByTopLevel(result.Files)
		for _, g := range groups {
			file := g.name
			size := formatSize(g.bytes)
			if g.count > 1 {
				file = g.name
				size = fmt.Sprintf("%d files  %s", g.count, formatSize(g.bytes))
			}
			syncCh <- tui.SyncEvent{
				File:     file,
				Size:     size,
				Duration: result.Duration,
				Status:   "synced",
				Time:     now,
			}
		}

		// Fallback: rsync ran but no individual files parsed
		if len(groups) == 0 && result.FilesCount > 0 {
			syncCh <- tui.SyncEvent{
				File:     fmt.Sprintf("%d files", result.FilesCount),
				Size:     formatSize(result.BytesTotal),
				Duration: result.Duration,
				Status:   "synced",
				Time:     now,
			}
		}

		// Accumulate stats
		totalSynced += result.FilesCount
		totalBytes += result.BytesTotal

		// Reset header
		syncCh <- tui.SyncEvent{Status: "status:watching"}
	}
```

**Step 3: Handle status messages in dashboard Update**

In `internal/tui/dashboard.go`, update the `SyncEventMsg` case in `Update`:

```go
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
	}
	return m, nil
```

Add `"strings"` to the imports in `dashboard.go` if not already present (it is).

**Step 4: Send stats after each sync**

Still in the TUI handler in `cmd/sync.go`, after the status reset, send stats. But we're using the same `syncCh` channel which sends `SyncEvent`. We need a different approach.

Simpler: update the dashboard's stats directly from the event stream. In `dashboard.go`, update the `SyncEventMsg` handler to accumulate stats:

```go
if evt.Status == "synced" {
	m.lastSync = evt.Time
	m.totalSynced++
	// Parse size back (or just count events)
}
```

Actually, the simplest approach: count synced events and track the `lastSync` time. Remove the `SyncStatsMsg` type and the `totalBytes` / `totalErrors` fields. Replace the stats bar with just event count + last sync time. The exact byte total isn't meaningful in grouped view anyway.

Replace stats rendering in `View`:

```go
// --- Stats (2 lines) ---
b.WriteString("  " + titleStyle.Render("Stats") + " " + dimStyle.Render(strings.Repeat("─", max(0, m.width-10))) + "\n")
stats := fmt.Sprintf("  %d events │ %d errors", m.totalSynced, m.totalErrors)
b.WriteString(stats + "\n")
```

In the `SyncEventMsg` handler, increment counters:

```go
if evt.Status == "synced" {
	m.lastSync = evt.Time
	m.totalSynced++
} else if evt.Status == "error" {
	m.totalErrors++
}
```

Remove `totalBytes string` from `DashboardModel` and `SyncStatsMsg` type from `dashboard.go`. Remove the `SyncStatsMsg` case from `Update`.

**Step 5: Wire up resync channel in cmd/sync.go**

In `runTUI`, after starting the watcher and before creating the tea.Program, add a goroutine:

```go
	resyncCh := app.ResyncChan()
	go func() {
		for range resyncCh {
			handler()
		}
	}()
```

**Step 6: Build and verify**

Run: `go build ./...`
Expected: success

**Step 7: Run tests**

Run: `go test ./...`
Expected: all pass

**Step 8: Commit**

```bash
git add cmd/sync.go internal/tui/dashboard.go
git commit -m "feat(tui): top-level grouping, stats accumulation, resync wiring"
```

---

### Task 4: End-to-end verification

**Step 1: Build binary**

```bash
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o esync-darwin-arm64 .
```

**Step 2: Test with local docs sync**

```bash
rm -rf /tmp/esync-docs && mkdir -p /tmp/esync-docs
./esync-darwin-arm64 sync --daemon -v
# In another terminal: touch docs/plans/2026-03-01-go-rewrite-design.md
# Verify: "Synced 2 files" appears
```

**Step 3: Test TUI**

```bash
./esync-darwin-arm64 sync
# Verify:
# - Header shows "● Watching", switches to "⟳ Syncing" during rsync
# - Events show timestamps: "15:04:05 ✓ plans/ ..."
# - j/k scrolls the event list
# - r triggers a full resync
# - Event list fills terminal height
# - No "⟳ . syncing..." rows in the event list
```

**Step 4: Run full test suite**

Run: `go test ./...`
Expected: all pass

**Step 5: Commit design doc**

```bash
git add docs/plans/
git commit -m "docs: add TUI improvements design and plan"
```
