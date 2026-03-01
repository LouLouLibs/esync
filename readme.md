# esync

A lightweight file synchronization tool that watches your local directory for changes and automatically syncs them to a local or remote destination using rsync.

## Installation

Install with `go install`:

```bash
go install github.com/eloualiche/esync@latest
```

Or build from source:

```bash
git clone https://github.com/eloualiche/esync.git
cd esync
go build -o esync .
```

## Quick Start

```bash
# 1. Generate a config file (imports .gitignore, detects common dirs)
esync init -r user@host:/path/to/dest

# 2. Preview what will be synced
esync check

# 3. Start watching and syncing
esync sync
```

## Commands Reference

### `esync sync`

Watch a local directory for changes and sync them to a destination using rsync. Launches an interactive TUI by default.

```bash
esync sync                          # use config file, launch TUI
esync sync -c project.toml          # use a specific config file
esync sync -l ./src -r server:/opt  # quick mode, no config file needed
esync sync --daemon                 # run in background (no TUI)
esync sync --dry-run                # show what would sync, don't transfer
esync sync --initial-sync           # force a full sync on startup
esync sync -v                       # verbose output (daemon mode)
```

| Flag              | Short | Description                              |
|-------------------|-------|------------------------------------------|
| `--local`         | `-l`  | Local path to watch                      |
| `--remote`        | `-r`  | Remote destination path                  |
| `--daemon`        |       | Run in daemon mode (no TUI)              |
| `--dry-run`       |       | Show what would be synced without syncing |
| `--initial-sync`  |       | Force a full sync on startup             |
| `--verbose`       | `-v`  | Verbose output                           |
| `--config`        | `-c`  | Config file path (global flag)           |

When both `-l` and `-r` are provided, esync runs without a config file (quick mode). Otherwise it searches for a config file automatically.

### `esync init`

Generate an `esync.toml` configuration file in the current directory. Inspects the project for `.gitignore` patterns and common directories (`.venv`, `build`, `__pycache__`, etc.) to auto-populate ignore rules.

```bash
esync init                          # interactive prompt for remote
esync init -r user@host:/path       # pre-fill the remote destination
esync init -c ~/.config/esync/config.toml -r server:/data  # custom path
```

| Flag       | Short | Description                        |
|------------|-------|------------------------------------|
| `--remote` | `-r`  | Pre-fill remote destination        |
| `--config` | `-c`  | Output file path (default: `./esync.toml`) |

### `esync check`

Validate your configuration and preview which files would be included or excluded by the ignore patterns.

```bash
esync check                         # auto-detect config
esync check -c project.toml         # check a specific config file
```

| Flag       | Short | Description                        |
|------------|-------|------------------------------------|
| `--config` | `-c`  | Config file path                   |

### `esync edit`

Open the config file in your `$EDITOR` (defaults to `vi`). After saving, the config is validated and a file preview is shown. If validation fails, you can re-edit or cancel.

```bash
esync edit                          # auto-detect config
esync edit -c project.toml          # edit a specific config file
```

| Flag       | Short | Description                        |
|------------|-------|------------------------------------|
| `--config` | `-c`  | Config file path                   |

### `esync status`

Check if an esync daemon is currently running. Reads the PID file from the system temp directory.

```bash
esync status
# esync daemon running (PID 12345)
# — or —
# No esync daemon running.
```

## Configuration

esync uses TOML configuration files. The config file is searched in this order:

1. Path given via `-c` / `--config` flag
2. `./esync.toml` (current directory)
3. `~/.config/esync/config.toml`
4. `/etc/esync/config.toml`

### Full Annotated Example

This shows every available field with explanatory comments:

```toml
# =============================================================================
# esync configuration file
# =============================================================================

[sync]
# Local directory to watch for changes (required)
local  = "/home/user/projects/myapp"

# Remote destination — can be a local path or an scp-style remote (required)
# Examples:
#   "/backup/myapp"                   — local path
#   "server:/opt/myapp"               — remote using SSH config alias
#   "user@192.168.1.50:/opt/myapp"    — remote with explicit user
remote = "deploy@prod.example.com:/var/www/myapp"

# Polling interval in seconds (default: 1)
# This is used internally; the watcher reacts to filesystem events,
# so you rarely need to change this.
interval = 1

# --- SSH Configuration (optional) ---
# Use this section for fine-grained SSH control.
# If omitted, esync infers SSH from the remote string (e.g. user@host:/path).
[sync.ssh]
host             = "prod.example.com"
user             = "deploy"
port             = 22
identity_file    = "~/.ssh/id_ed25519"
interactive_auth = false   # set to true for 2FA / keyboard-interactive auth

# =============================================================================
[settings]

# Debounce interval in milliseconds (default: 500)
# After a file change, esync waits this long for more changes before syncing.
# Lower = more responsive, higher = fewer rsync invocations during rapid edits.
watcher_debounce = 500

# Run a full sync when esync starts (default: false)
initial_sync = false

# Patterns to ignore — applied to both the watcher and rsync --exclude flags.
# Supports glob patterns. Matched against file/directory base names.
ignore = [
    ".git",
    "node_modules",
    ".DS_Store",
    "__pycache__",
    "*.pyc",
    ".venv",
    "build",
    "dist",
    ".tox",
    ".mypy_cache",
]

# --- Rsync Settings ---
[settings.rsync]
archive    = true           # rsync --archive (preserves symlinks, permissions, timestamps)
compress   = true           # rsync --compress (compress data during transfer)
backup     = false          # rsync --backup (make backups of replaced files)
backup_dir = ".rsync_backup"  # directory for backup files when backup = true
progress   = true           # rsync --progress (show transfer progress)

# Extra arguments passed directly to rsync.
# Useful for flags esync doesn't expose directly.
extra_args = []

# Additional rsync-specific ignore patterns (merged with settings.ignore).
ignore = []

# --- Logging ---
[settings.log]
# Log file path. If omitted, no log file is written.
# Logs are only written in daemon mode.
# file = "/var/log/esync.log"

# Log format: "text" or "json" (default: "text")
format = "text"
```

### Minimal Config

The smallest usable config file:

```toml
[sync]
local  = "."
remote = "user@host:/path/to/dest"
```

Everything else uses sensible defaults: archive mode, compression, 500ms debounce, and standard ignore patterns (`.git`, `node_modules`, `.DS_Store`).

### SSH Config Example

For remote servers with a specific SSH key and non-standard port:

```toml
[sync]
local  = "."
remote = "/var/www/myapp"

[sync.ssh]
host          = "myserver.example.com"
user          = "deploy"
port          = 2222
identity_file = "~/.ssh/deploy_key"
```

When `[sync.ssh]` is present, esync constructs the full destination as `deploy@myserver.example.com:/var/www/myapp` and passes SSH options (port, identity file, ControlMaster) to rsync automatically.

### 2FA / Keyboard-Interactive Authentication

If your server requires two-factor authentication:

```toml
[sync]
local  = "."
remote = "/home/user/project"

[sync.ssh]
host             = "secure-server.example.com"
user             = "admin"
identity_file    = "~/.ssh/id_ed25519"
interactive_auth = true
```

### Custom Rsync Flags

Pass extra arguments directly to rsync using `extra_args`:

```toml
[sync]
local  = "./src"
remote = "server:/opt/app/src"

[settings.rsync]
archive    = true
compress   = true
extra_args = [
    "--delete",           # delete files on remote that don't exist locally
    "--chmod=Du=rwx,Dgo=rx,Fu=rw,Fgo=r",  # set permissions on remote
    "--exclude-from=.rsyncignore",          # additional exclude file
    "--bwlimit=5000",     # bandwidth limit in KBytes/sec
]
```

### Separate Watcher and Rsync Ignore Patterns

The top-level `settings.ignore` patterns are used by both the file watcher and rsync. If you need rsync-specific excludes (patterns the watcher should still see), use `settings.rsync.ignore`:

```toml
[settings]
# These patterns are used by BOTH the watcher and rsync
ignore = [".git", "node_modules", ".DS_Store"]

[settings.rsync]
# These patterns are ONLY passed to rsync as --exclude flags
ignore = ["*.log", "*.tmp", "cache/"]
```

### Logging Config

```toml
[settings.log]
file   = "/var/log/esync.log"
format = "json"
```

Text format output:

```
15:04:05 INF started local=/home/user/project pid=12345 remote=server:/opt/app
15:04:07 INF sync_complete bytes=2048 duration=150ms files=3
15:04:12 ERR sync_failed error=rsync error: ...
```

JSON format output:

```json
{"time":"15:04:05","level":"info","event":"started","local":"/home/user/project","remote":"server:/opt/app","pid":12345}
{"time":"15:04:07","level":"info","event":"sync_complete","files":3,"bytes":2048,"duration":"150ms"}
```

## TUI Keyboard Shortcuts

The interactive TUI (default mode) provides two views: Dashboard and Logs.

### Dashboard View

| Key       | Action                         |
|-----------|--------------------------------|
| `q`       | Quit                           |
| `Ctrl+C`  | Quit                           |
| `p`       | Pause / resume watching        |
| `l`       | Switch to log view             |
| `/`       | Enter filter mode              |
| `Enter`   | Apply filter (in filter mode)  |
| `Esc`     | Clear filter (in filter mode)  |

### Log View

| Key       | Action                         |
|-----------|--------------------------------|
| `q`       | Quit                           |
| `Ctrl+C`  | Quit                           |
| `l`       | Switch back to dashboard       |
| `j` / `Down` | Scroll down                 |
| `k` / `Up`   | Scroll up                   |
| `/`       | Enter filter mode              |
| `Enter`   | Apply filter (in filter mode)  |
| `Esc`     | Clear filter (in filter mode)  |

## Daemon Mode

Run esync in the background without the TUI:

```bash
# Start daemon
esync sync --daemon

# Start daemon with verbose output and JSON logging
esync sync --daemon -v -c project.toml

# Check if the daemon is running
esync status

# Stop the daemon
kill $(cat /tmp/esync.pid)
```

The daemon writes its PID to `/tmp/esync.pid` so you can check status and stop it later. On receiving `SIGINT` or `SIGTERM` the daemon shuts down gracefully.

When a log file is configured, the daemon writes structured entries for every sync event:

```bash
# Monitor logs in real-time
tail -f /var/log/esync.log
```

## SSH Setup

esync uses rsync's SSH transport for remote syncing. There are two ways to configure SSH.

### Inline (via remote string)

If your `~/.ssh/config` is already set up, just use the host alias:

```toml
[sync]
local  = "."
remote = "myserver:/opt/app"
```

This works when `myserver` is defined in `~/.ssh/config`:

```
Host myserver
    HostName 192.168.1.50
    User deploy
    IdentityFile ~/.ssh/id_ed25519
```

### Explicit SSH Section

For full control without relying on `~/.ssh/config`:

```toml
[sync.ssh]
host          = "192.168.1.50"
user          = "deploy"
port          = 22
identity_file = "~/.ssh/id_ed25519"
```

When the `[sync.ssh]` section is present, esync automatically enables SSH ControlMaster with these options:

- `ControlMaster=auto` -- reuse existing SSH connections
- `ControlPath=/tmp/esync-ssh-%r@%h:%p` -- socket path for multiplexing
- `ControlPersist=600` -- keep the connection alive for 10 minutes

This avoids re-authenticating on every sync and significantly speeds up repeated transfers.

### 2FA Authentication

Set `interactive_auth = true` in the SSH config to enable keyboard-interactive authentication for servers that require a second factor:

```toml
[sync.ssh]
host             = "secure.example.com"
user             = "admin"
identity_file    = "~/.ssh/id_ed25519"
interactive_auth = true
```

## Examples

### Local directory sync

Sync a source directory to a local backup:

```bash
esync sync -l ./src -r /backup/src
```

### Remote sync with SSH

Sync to a remote server using a config file:

```toml
# esync.toml
[sync]
local  = "."
remote = "deploy@prod.example.com:/var/www/mysite"

[settings]
ignore = [".git", "node_modules", ".DS_Store", ".env"]
```

```bash
esync sync
```

### Quick sync (no config file)

Sync without a config file by passing both paths on the command line:

```bash
esync sync -l ./project -r user@server:/opt/project
```

This uses sensible defaults: archive mode, compression, 500ms debounce, and ignores `.git`, `node_modules`, `.DS_Store`.

### Daemon mode with JSON logs

Run in the background with structured logging:

```toml
# esync.toml
[sync]
local  = "/home/user/code"
remote = "server:/opt/code"

[settings]
initial_sync = true

[settings.log]
file   = "/var/log/esync.log"
format = "json"
```

```bash
esync sync --daemon -v
# esync daemon started (PID 54321)
# Watching: /home/user/code -> server:/opt/code
```

### Custom rsync flags (delete extraneous files)

Keep the remote directory in exact sync by deleting files that no longer exist locally:

```toml
# esync.toml
[sync]
local  = "./dist"
remote = "cdn-server:/var/www/static"

[settings.rsync]
extra_args = ["--delete", "--chmod=Fu=rw,Fgo=r,Du=rwx,Dgo=rx"]
```

```bash
esync sync --initial-sync
```

### Dry run to preview changes

See what rsync would do without actually transferring anything:

```bash
esync sync --dry-run
```

## System Requirements

- **Go** 1.22+ (for building from source)
- **rsync** 3.x
- **macOS** or **Linux** (uses fsnotify for filesystem events)
