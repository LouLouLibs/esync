# Edit Config from TUI

## Summary

Add an `e` key to the TUI dashboard that opens `.esync.toml` in `$EDITOR`. On save, the config is reloaded and the watcher/syncer are rebuilt with the new values. If no config file exists, a template is created; if the user exits without saving, nothing is persisted.

Also renames the project-level config from `esync.toml` to `.esync.toml` (dotfile).

## Config File Rename

- `FindConfigFile()` in `internal/config/config.go` searches `./.esync.toml` instead of `./esync.toml`
- System-level paths (`~/.config/esync/config.toml`, `/etc/esync/config.toml`) remain unchanged
- The `e` key always targets `./.esync.toml` in cwd

## Key Flow

### 1. Keypress

Dashboard handles `e` keypress and sends `EditConfigMsg{}` up to `AppModel`.

### 2. AppModel receives EditConfigMsg

- Target path: `./.esync.toml` (cwd)
- **File exists:** record SHA-256 checksum, open in `$EDITOR` via `tea.ExecProcess`
- **File does not exist:** write template to a temp file, record its checksum, open temp file in `$EDITOR`

### 3. Editor exits (editorConfigFinishedMsg)

- **New file flow (was temp):** compare checksums. If unchanged, delete temp file and done. If changed, copy temp contents to `./.esync.toml`, delete temp file.
- **Existing file flow:** compare checksums. If unchanged, done.
- **Config changed:** parse with `config.Load()`.
  - **Parse fails:** push error to TUI status line (e.g., "config error: missing sync.remote"), keep old config running.
  - **Parse succeeds:** send new `*config.Config` on `configReloadCh` channel.

### 4. cmd/sync.go handles reload

Listens on `app.ConfigReloadChan()`:

1. Stop existing watcher
2. Rebuild syncer with new config
3. Create new watcher with new config values (local path, debounce, ignore patterns, includes)
4. Re-wire sync handler to use new syncer and push to existing TUI channels
5. Push status event: "config reloaded"

## Template Content

Used when no `.esync.toml` exists. Required fields (`local`, `remote`) are uncommented; everything else is commented with defaults shown:

```toml
# esync configuration
# Docs: https://github.com/LouLouLibs/esync

[sync]
local = "."
remote = "user@host:/path/to/dest"
# interval = 0  # seconds, 0 = watch mode only

# [sync.ssh]
# key = "~/.ssh/id_ed25519"
# port = 22

[settings]
# watcher_debounce = 300   # ms
# initial_sync = true
# include = ["src/", "cmd/"]
# ignore = [".git", "*.tmp"]

# [settings.rsync]
# archive = true
# compress = true
# delete = false
# copy_links = false
# extra_args = ["--exclude=.DS_Store"]

# [settings.log]
# file = "esync.log"
# format = "text"
```

## Help Bar

Updated dashboard help line:

```
q quit  p pause  r resync  ↑↓ navigate  enter expand  v view  e config  l logs  / filter
```

`e config` inserted between `v view` and `l logs`, using existing `helpKey()`/`helpDesc()` styling.

## New Types and Channels

| Item | Location | Purpose |
|------|----------|---------|
| `EditConfigMsg` | `internal/tui/app.go` | Dashboard → AppModel signal |
| `editorConfigFinishedMsg` | `internal/tui/app.go` | Editor exit result |
| `ConfigReloadMsg` | `internal/tui/app.go` | Carries new `*config.Config` |
| `configReloadCh` | `AppModel` field | `chan *config.Config`, capacity 1 |
| `ConfigReloadChan()` | `AppModel` method | Exposes channel to `cmd/sync.go` |

## Error Handling

- Bad TOML or missing required fields: status line error, old config retained
- `$EDITOR` not set: fall back to `vi` (consistent with git behavior; current `v` key falls back to `less` but for editing `vi` is more appropriate)
- Editor returns non-zero exit: treat as "no change", discard

## Files Modified

- `internal/config/config.go` — rename `esync.toml` → `.esync.toml` in `FindConfigFile()`
- `internal/tui/app.go` — new message types, `configReloadCh`, editor launch/return handling
- `internal/tui/dashboard.go` — `e` key binding, help line update
- `cmd/sync.go` — listen on `configReloadCh`, tear down and rebuild watcher/syncer
