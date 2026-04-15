# CLAUDE.md

## Workflow
- **Branch per issue.** Every change lives on its own feature branch tied to a GitHub issue; do not commit directly to `main`.
- Branch naming: `fix/<issue-number>-<slug>` or `feat/<issue-number>-<slug>` (e.g. `fix/14-filter-ordering`).
- Open a PR against `main` when the branch is ready; reference the issue so it auto-closes on merge.
- If there is no tracking issue for the work, open one first.

## Build & Test
- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o esync-darwin-arm64 .` — trimmed release binary

## Gotchas

### macOS rsync
- `/usr/bin/rsync` is Apple's `openrsync` — it lacks `--info=progress2` and other modern flags
- `rsyncBin()` in `internal/syncer/syncer.go` resolves to homebrew rsync (`/opt/homebrew/bin/rsync`) when available
- `exec.Command` does not use shell aliases, so the binary path must be resolved explicitly
- `CheckRsync()` validates rsync >= 3.1.0 on startup

### rsync output parsing
- With `--info=progress2`, both per-file and overall progress lines contain `xfr#`/`to-chk=`
- In `extractFiles()`, the 100% size-extraction check MUST come before the progress2 skip guard, or per-file sizes are lost
- Fast transfers may skip the 100% progress line entirely, leaving per-file `Bytes: 0`
- When per-file sizes are missing, `cmd/sync.go` distributes `BytesTotal` across groups weighted by file count
- `extractStats()` must match `Total transferred file size:` (actual bytes sent), NOT `Total file size:` (entire source tree size)

### TUI channels
- Status-only messages use `"status:..."` prefix (e.g. `"status:syncing 45%"`)
- Channel sends from the sync handler use non-blocking `select/default` to avoid deadlocks
- `syncEvents` and `logEntries` channels are buffered (capacity 64)
