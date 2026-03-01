# TUI Improvements Design

## Problems

1. **"Syncing" events pollute the event list.** The handler sends `{File: ".", Status: "syncing"}` before every rsync run. These pile up as permanent `⟳ .  syncing...` rows and never clear.

2. **Per-file events don't scale.** A sync transferring 1000 files would produce 1000 event rows, overwhelming the list and slowing TUI updates.

3. **Event list doesn't fill the terminal.** The visible row count uses a hardcoded `height-10` offset. Tall terminals waste space; short terminals clip.

4. **No scrolling or timestamps.** Events are a flat, non-navigable list with no time information.

5. **`r` (full resync) is a dead key.** Shown in the help bar but has no handler.

6. **Stats bar shows 0.** The `totalSynced` / `totalBytes` / `totalErrors` counters are never updated because nothing sends `SyncStatsMsg`.

## Design

### Syncing indicator: transient header status

Remove "syncing" events from the event list. Add a new message type `SyncStatusMsg string` that updates only the header status line. The handler sends `SyncStatusMsg("syncing")` before rsync runs and `SyncStatusMsg("watching")` after. No syncing rows appear in the event list.

### Top-level grouping of file events

After rsync completes, the handler in `cmd/sync.go` groups `result.Files` by top-level path component:

- Files in subdirectories are grouped by their first path segment. `cmd/sync.go` + `cmd/init.go` + `cmd/root.go` become one event: `✓ cmd/  3 files  12.3KB`.
- Files at the root level get individual events: `✓ main.go  2.1KB`.

Grouping happens in the handler after rsync returns, so it adds no overhead to the transfer. The TUI receives at most `N_top_level_dirs + N_root_files` events per sync.

### Event list fills terminal, scrollable with timestamps

**Layout**: compute available event rows as `height - 6`:
- Header: 3 lines (title, paths, status + blank)
- Stats + help: 3 lines

Pad with empty lines when fewer events exist so the section always fills.

**Timestamps**: each event row includes `HH:MM:SS` from `evt.Time`:
```
  15:04:05  ✓ cmd/                   3 files  12.3KB  120ms
  15:04:05  ✓ main.go                          2.1KB  120ms
  15:03:58  ✓ internal/              5 files  45.2KB  200ms
```

**Scrolling**: add `offset int` to `DashboardModel`. `j`/`k` or `↑`/`↓` move the viewport. The event list is a window into `filteredEvents()[offset:offset+viewHeight]`.

### `r` triggers full resync

Add a `resyncCh chan struct{}` to `AppModel`, exposed via `ResyncChan()`. When the user presses `r`, the dashboard emits a `ResyncRequestMsg`. AppModel catches it and sends on the channel. The handler in `cmd/sync.go` listens on `resyncCh` in a goroutine and calls `s.Run()` when signalled, feeding results back through the existing event channel.

### Stats bar accumulates

The handler updates running totals (`totalSynced`, `totalBytes`, `totalErrors`) after each sync and sends a `SyncStatsMsg`. The dashboard renders these in the stats section.

## Event row format

```
  HH:MM:SS  icon  name(padded)  detail      size     duration
  15:04:05  ✓     cmd/          3 files    12.3KB     120ms
  15:04:05  ✓     main.go                   2.1KB     120ms
  15:04:05  ✗     internal/     error       ─          ─
```

## Files to change

- `internal/tui/dashboard.go` — timestamps, scrolling, fill terminal, remove syncing events
- `internal/tui/app.go` — new message types (`SyncStatusMsg`, `ResyncRequestMsg`), resync channel
- `cmd/sync.go` — top-level grouping, stats accumulation, resync listener, remove syncing event send
