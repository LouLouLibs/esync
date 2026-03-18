# Edit Config from TUI

## Summary

Add an `e` key to the TUI dashboard that opens `.esync.toml` in `$EDITOR`. On save, the config is reloaded and the watcher/syncer are rebuilt with the new values. If no config file exists, a template is created; if the user exits without saving, nothing is persisted.

Also renames the project-level config from `esync.toml` to `.esync.toml` (dotfile).

## Config File Rename

- `FindConfigFile()` in `internal/config/config.go` searches `./.esync.toml` instead of `./esync.toml`
- System-level paths (`~/.config/esync/config.toml`, `/etc/esync/config.toml`) remain unchanged
- `cmd/init.go` default output path changes from `./esync.toml` to `./.esync.toml`
- `esync.toml.example` renamed to `.esync.toml.example`
- `README.md` references updated from `esync.toml` to `.esync.toml`
- The `e` key always targets `./.esync.toml` in cwd

## Key Flow

### 1. Keypress

Dashboard handles `e` keypress in `updateNormal()` and sends `EditConfigMsg{}` up to `AppModel`. Only active in dashboard view (not logs). Already gated by `updateNormal` vs `updateFiltering` dispatch, so typing `e` during filter input is safe.

### 2. AppModel receives EditConfigMsg

- Target path: `./.esync.toml` (cwd)
- **File exists:** record SHA-256 checksum, open in editor via `tea.ExecProcess`
- **File does not exist:** write template to a temp file (with `.toml` suffix for syntax highlighting, e.g. `os.CreateTemp("", "esync-*.toml")`), record its checksum, open temp file in editor
- **Editor resolution:** check `$VISUAL`, then `$EDITOR`, fall back to `vi`

### 3. Editor exits (editorConfigFinishedMsg)

- **New file flow (was temp):** compare checksums. If unchanged, delete temp file, done. If changed, copy temp contents to `./.esync.toml`, delete temp file.
- **Existing file flow:** compare checksums. If unchanged, done.
- **Config changed:** parse with `config.Load()`.
  - **Parse fails:** push error to TUI status line (e.g., "config error: missing sync.remote"), keep old config running.
  - **Parse succeeds:** send new `*config.Config` on `configReloadCh` channel.

Note: `tea.ExecProcess` blocks the TUI program, so the user cannot press `e` again while the editor is open. This makes the capacity-1 channel safe without needing non-blocking sends.

### 4. cmd/sync.go handles reload

Listens on `app.ConfigReloadChan()`:

1. Stop existing watcher (blocks until `<-w.done`, ensuring no in-flight handler)
2. Wait for any in-flight sync to complete before proceeding
3. Rebuild syncer with new config
4. Create new watcher with new config values (local path, debounce, ignore patterns, includes)
5. Create new sync handler closure capturing the new syncer
6. Push status event: "config reloaded"

### 5. Flag-only mode (--local/--remote without config file)

When esync was started with `--local`/`--remote` flags and no config file, pressing `e` still opens `./.esync.toml`. If the file doesn't exist, the template is shown. After save, the reloaded config replaces the flag-derived config entirely (CLI flags are not re-applied on top). This lets users transition from quick flag-based usage to a persistent config file.

## Template Content

A new `EditTemplateTOML()` function in `internal/config/config.go`, separate from the existing `DefaultTOML()` used by `esync init`. The edit template is minimal with most options commented out, while `DefaultTOML()` remains unchanged for `esync init`'s string-replacement logic.

```toml
# esync configuration
# Docs: https://github.com/LouLouLibs/esync

[sync]
local = "."
remote = "user@host:/path/to/dest"
# interval = 1  # seconds between syncs

# [sync.ssh]
# key = "~/.ssh/id_ed25519"
# port = 22

[settings]
# watcher_debounce = 500   # ms
# initial_sync = false
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
| `editorConfigFinishedMsg` | `internal/tui/app.go` | Editor exit result (distinct from existing `editorFinishedMsg`) |
| `configReloadCh` | `AppModel` field | `chan *config.Config`, capacity 1 |
| `ConfigReloadChan()` | `AppModel` method | Exposes channel to `cmd/sync.go` |

## Error Handling

- Bad TOML or missing required fields: status line error, old config retained
- Editor not set: check `$VISUAL` → `$EDITOR` → `vi`
- Editor returns non-zero exit: treat as "no change", discard
- Watcher detects `.esync.toml` write: harmless (rsync transfers the small file). Not added to default ignore since users may intentionally sync config files.

## Files Modified

- `internal/config/config.go` — rename `esync.toml` → `.esync.toml` in `FindConfigFile()`, add `EditTemplateTOML()`
- `internal/tui/app.go` — new message types, `configReloadCh`, editor launch/return handling
- `internal/tui/dashboard.go` — `e` key binding, help line update
- `cmd/sync.go` — listen on `configReloadCh`, tear down and rebuild watcher/syncer
- `cmd/init.go` — update default output path to `./.esync.toml`, update `defaultIgnorePatterns`
- `esync.toml.example` — rename to `.esync.toml.example`
- `README.md` — update references from `esync.toml` to `.esync.toml`
