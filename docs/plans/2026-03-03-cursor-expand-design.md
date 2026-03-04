# Cursor Navigation & Inline Expand for TUI Dashboard

## Problem

The dashboard shows grouped sync events (e.g. "cmd/ 2 files") but provides no
way to inspect which files are inside a group. Users need to see individual
file paths without leaving the dashboard.

## Design

### 1. Cursor navigation

Add a `cursor int` field to `DashboardModel`. Up/down arrows move the cursor
through the filtered event list. The focused row gets a subtle highlight — a
`>` marker and brighter text. The viewport auto-scrolls to keep the cursor
visible.

### 2. Individual files in SyncEvent

Add `Files []string` to the `SyncEvent` struct. When `groupFilesByTopLevel()`
in `cmd/sync.go` produces a group with `count > 1`, populate `Files` with the
individual relative paths from that group.

### 3. Inline expand/collapse

Add `expanded map[int]bool` to `DashboardModel` (keyed by event index in the
unfiltered `events` slice). Press Enter on a focused event to toggle. When
expanded, child files render below the parent, indented:

```
  14:32:05  ✓ cmd/                              2 files  1.2KB  0.3s
               └ cmd/sync.go
               └ cmd/root.go
  14:32:01  ✓ main.go                                    0.5KB  0.1s
```

Single-file events (empty `Files`) ignore the expand action.

### 4. Column alignment

The current layout uses a fixed 30-char name column. With the terminal width
available, use more space:

- Timestamp: fixed 8 chars
- Status icon: 1 char + spacing
- Name column: dynamic, scales with terminal width (min 30, up to width - 40)
- Size + duration: right-aligned in remaining space
- Expanded child lines: indented under the name column, same alignment

Child file names use the full name column width minus 2 chars for the `└ `
prefix.

### 5. Key bindings

| Key        | Action                                    |
|------------|-------------------------------------------|
| j / ↓      | Move cursor down                          |
| k / ↑      | Move cursor up                            |
| Enter / →  | Toggle expand on focused event            |
| Left / Esc | Collapse focused event (if expanded)      |

The help line updates to show the new bindings.
