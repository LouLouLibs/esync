# esync Go Rewrite — Design Document

Date: 2026-03-01

## Motivation

Rewrite esync from Python to Go for three equal priorities:

1. **Single binary distribution** — no Python/pip dependency; download and run
2. **Performance** — faster startup, lower memory, better for long-running watch processes
3. **Better TUI** — polished interactive dashboard using Bubbletea/Lipgloss

## Technology Stack

| Component | Library | Purpose |
|-----------|---------|---------|
| CLI framework | [Cobra](https://github.com/spf13/cobra) | Subcommands, flags, help generation |
| Configuration | [Viper](https://github.com/spf13/viper) | TOML loading, config file search path, env vars |
| TUI framework | [Bubbletea](https://github.com/charmbracelet/bubbletea) | Interactive terminal UI |
| TUI styling | [Lipgloss](https://github.com/charmbracelet/lipgloss) | Borders, colors, layout |
| File watching | [fsnotify](https://github.com/fsnotify/fsnotify) | Cross-platform filesystem events |
| Sync engine | rsync (external) | File transfer via subprocess |

## Project Structure

```
esync/
├── cmd/
│   ├── root.go           # Root command, global flags
│   ├── sync.go           # esync sync — main watch+sync command
│   ├── init.go           # esync init — smart config generation
│   ├── check.go          # esync check — validate config + preview
│   ├── edit.go           # esync edit — open in $EDITOR + preview
│   └── status.go         # esync status — check running daemon
├── internal/
│   ├── config/           # TOML config models, loading, validation
│   ├── watcher/          # fsnotify wrapper with debouncing
│   ├── syncer/           # rsync command builder and executor
│   ├── tui/              # Bubbletea models, views, styles
│   │   ├── app.go        # Root Bubbletea model
│   │   ├── dashboard.go  # Main dashboard view
│   │   ├── logview.go    # Scrollable log view
│   │   └── styles.go     # Lipgloss style definitions
│   └── logger/           # Structured logging (file + JSON)
├── main.go               # Entry point
├── esync.toml            # Example config
└── go.mod
```

## CLI Commands

```
esync sync [flags]          Start watching and syncing
  -c, --config PATH         Config file path
  -l, --local PATH          Override local path
  -r, --remote PATH         Override remote path
  --daemon                  Run without TUI, log to file
  --dry-run                 Show what would sync without executing
  --initial-sync            Force full sync on startup
  -v, --verbose             Verbose output

esync init [flags]          Generate config from current directory
  -c, --config PATH         Output path (default: ./esync.toml)
  -r, --remote PATH         Pre-fill remote destination

esync check [flags]         Validate config and show file include/exclude preview
  -c, --config PATH         Config file path

esync edit [flags]          Open config in $EDITOR, then show preview
  -c, --config PATH         Config file path

esync status                Check if daemon is running, show last sync info
```

### Quick usage (no config file)

`esync sync -l ./src -r user@host:/deploy` works without a config file when both local and remote are provided as flags.

## Configuration

### Format

TOML. Search order:

1. `-c` / `--config` flag
2. `./esync.toml`
3. `~/.config/esync/config.toml`
4. `/etc/esync/config.toml`

### Schema

```toml
[sync]
local = "./src"
remote = "user@host:/deploy/src"
interval = 1                        # debounce interval in seconds

[sync.ssh]
host = "example.com"
user = "deploy"
port = 22
identity_file = "~/.ssh/id_ed25519"
interactive_auth = true             # for 2FA prompts

[settings]
watcher_debounce = 500              # ms, batch rapid changes
initial_sync = true                 # full rsync on startup
ignore = ["*.log", "*.tmp", ".env"]

[settings.rsync]
archive = true
compress = true
backup = true
backup_dir = ".rsync_backup"
progress = true
extra_args = ["--delete"]           # pass-through for any rsync flags
ignore = [".git/", "node_modules/", "**/__pycache__/"]

[settings.log]
file = "~/.local/share/esync/esync.log"
format = "json"                     # "json" or "text"
```

### Smart init

`esync init` inspects the current directory:

- Detects `.gitignore` and imports patterns into `settings.rsync.ignore`
- Auto-excludes common directories (`.git/`, `node_modules/`, `__pycache__/`, `build/`, `.venv/`)
- Pre-fills `sync.local` with `.`
- Accepts `-r` flag or prompts for remote destination
- Shows `esync check` preview after generating

## TUI Design

### Main dashboard view

```
 esync ─────────────────────────────────────────
  ./src → user@host:/deploy/src
  ● Watching (synced 3s ago)

  Recent ──────────────────────────────────────
  ✓ src/main.go              2.1KB    0.3s
  ✓ src/config.go            1.4KB    0.2s
  ⟳ src/handler.go           syncing...
  ✓ src/util.go                890B   0.1s

  Stats ───────────────────────────────────────
  142 synced │ 3.2MB total │ 0 errors

  q quit  p pause  r full resync  l logs  d dry-run  / filter
```

### Log view (toggle with `l`)

```
 esync ─ logs ──────────────────────────────────
  14:23:01 INF synced src/main.go (2.1KB, 0.3s)
  14:23:03 INF synced src/config.go (1.4KB, 0.2s)
  14:23:05 INF syncing src/handler.go...
  14:23:06 INF synced src/handler.go (890B, 0.4s)
  14:23:06 WRN permission denied: .env (skipped)
  14:23:15 INF idle, watching for changes

  ↑↓ scroll  / filter  l back  q quit
```

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `q` | Quit |
| `p` | Pause/resume watching |
| `r` | Trigger full resync |
| `l` | Toggle log view |
| `d` | Dry-run next sync |
| `/` | Filter file list / log entries |

### Styling

Lipgloss with a subtle color palette:
- Green: success/synced
- Yellow: in-progress/syncing
- Red: errors
- Dim: timestamps, stats

Clean and minimal — not flashy.

## Runtime Modes

### Interactive (default)

`esync sync` launches the Bubbletea TUI dashboard. All events render live.

### Daemon

`esync sync --daemon` runs without TUI:
- Writes JSON lines to log file
- Prints PID on startup
- Terminal bell on sync errors

Log format:
```json
{"time":"14:23:01","level":"info","event":"synced","file":"src/main.go","size":2150,"duration_ms":300}
{"time":"14:23:06","level":"warn","event":"skipped","file":".env","reason":"permission denied"}
```

Check with `esync status`:
```
esync daemon running (PID 42351)
  Watching: ./src → user@host:/deploy/src
  Last sync: 3s ago (src/main.go)
  Session: 142 files synced, 0 errors
```

## Data Flow

```
fsnotify event
  → debouncer (batches events over configurable window, default 500ms)
  → syncer (builds rsync command, executes)
  → result (parsed rsync output: files, sizes, duration, errors)
  → TUI update OR log write
```

## Features

### Carried from Python version
- File watching with configurable ignore patterns
- rsync-based sync with SSH support
- TOML configuration with search path
- Archive, compress, backup options
- SSH authentication (key, password, interactive/2FA)
- CLI flag overrides for local/remote paths

### New in Go version
- **Debouncing** — batch rapid file changes into single rsync call
- **Initial sync on start** — optional full rsync before entering watch mode
- **Dry-run mode** — `--dry-run` flag and `d` key in TUI
- **Daemon mode** — `--daemon` with JSON log output and PID tracking
- **`esync status`** — check running daemon state
- **`esync check`** — validate config and show file include/exclude preview
- **`esync edit`** — open config in `$EDITOR`, then show preview
- **Smart `esync init`** — generate config from current directory, import .gitignore
- **rsync extra_args** — pass-through for arbitrary rsync flags
- **Pause/resume** — `p` key in TUI
- **Scrollable log view** — `l` key with `/` filter
- **SSH ControlMaster** — keep SSH connections alive between syncs
- **Sync sound** — terminal bell on errors
- **File filter in TUI** — `/` to search recent events and logs

### Dropped from Python version
- Watchman backend (fsnotify only)
- YAML dependency
- Dual watcher abstraction layer

## System Requirements

- Go 1.22+ (build time only)
- rsync 3.x
- macOS / Linux (fsnotify supports both)
