# Edit Config from TUI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `e` key to the TUI dashboard to open/create `.esync.toml` in `$EDITOR`, reload config on save, and rebuild watcher/syncer.

**Architecture:** Dashboard emits `EditConfigMsg` → AppModel handles editor lifecycle via `tea.ExecProcess` → on save, parsed config is sent on `configReloadCh` → `cmd/sync.go` tears down watcher/syncer and rebuilds. Separate from this, rename `esync.toml` → `.esync.toml` everywhere.

**Tech Stack:** Go, Bubbletea (TUI), Viper (TOML), crypto/sha256 (checksum), os/exec (editor)

**Spec:** `docs/superpowers/specs/2026-03-18-edit-config-design.md`

---

### Task 1: Rename esync.toml → .esync.toml in config search

**Files:**
- Modify: `internal/config/config.go:119-127` — change `"./esync.toml"` to `"./.esync.toml"` in `FindConfigFile()`
- Test: `internal/config/config_test.go` (create)

- [ ] **Step 1: Write test for FindConfigFile**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfigInPrefersDotFile(t *testing.T) {
	dir := t.TempDir()
	dotFile := filepath.Join(dir, ".esync.toml")
	os.WriteFile(dotFile, []byte("[sync]\n"), 0644)

	got := FindConfigIn([]string{
		filepath.Join(dir, ".esync.toml"),
		filepath.Join(dir, "esync.toml"),
	})
	if got != dotFile {
		t.Fatalf("expected %s, got %s", dotFile, got)
	}
}

func TestFindConfigInReturnsEmpty(t *testing.T) {
	got := FindConfigIn([]string{"/nonexistent/path"})
	if got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it passes (testing FindConfigIn which is already correct)**

Run: `go test ./internal/config/ -run TestFindConfigIn -v`
Expected: PASS — `FindConfigIn` is path-agnostic, so these tests validate it works.

- [ ] **Step 3: Update FindConfigFile to search .esync.toml**

In `internal/config/config.go:122`, change:

```go
// Before
"./esync.toml",
// After
"./.esync.toml",
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: rename config search from esync.toml to .esync.toml"
```

---

### Task 2: Update cmd/init.go, .gitignore, and demo files for .esync.toml

**Files:**
- Modify: `cmd/init.go:52-53` — update help text
- Modify: `cmd/init.go:70` — change default path
- Modify: `.gitignore:23` — change `/esync.toml` to `/.esync.toml`
- Modify: `demo/demo.tape:39` — change `bat esync.toml` to `bat .esync.toml`
- Rename: `esync.toml.example` → `.esync.toml.example`
- Rename: `demo/esync.toml` → `demo/.esync.toml`

- [ ] **Step 1: Update cmd/init.go default output path**

In `cmd/init.go:70`, change:

```go
// Before
outPath = "./esync.toml"
// After
outPath = "./.esync.toml"
```

- [ ] **Step 2: Update cmd/init.go command description**

In `cmd/init.go:52-53`, change:

```go
// Before
Short: "Generate an esync.toml configuration file",
Long:  "Inspect the current directory to generate a smart esync.toml with .gitignore import and common directory exclusion.",
// After
Short: "Generate an .esync.toml configuration file",
Long:  "Inspect the current directory to generate a smart .esync.toml with .gitignore import and common directory exclusion.",
```

- [ ] **Step 3: Update .gitignore**

In `.gitignore:23`, change:

```
# Before
/esync.toml
# After
/.esync.toml
```

- [ ] **Step 4: Update demo/demo.tape**

In `demo/demo.tape:39`, change:

```
# Before
Type "bat esync.toml"
# After
Type "bat .esync.toml"
```

- [ ] **Step 5: Rename files**

```bash
git mv esync.toml.example .esync.toml.example
git mv demo/esync.toml demo/.esync.toml
```

- [ ] **Step 6: Build to verify**

Run: `go build ./...`
Expected: Success

- [ ] **Step 7: Commit**

```bash
git add cmd/init.go .gitignore demo/demo.tape .esync.toml.example demo/.esync.toml
git commit -m "feat: update init, gitignore, demo files for .esync.toml rename"
```

---

### Task 3: Update README.md references

**Files:**
- Modify: `README.md` — replace `esync.toml` with `.esync.toml` (multiple occurrences)

- [ ] **Step 1: Replace all references**

In `README.md`, replace all occurrences of `esync.toml` with `.esync.toml`. Key locations:

- Line 75: `` `esync.toml` `` → `` `.esync.toml` ``
- Line 86: `./esync.toml` → `./.esync.toml`
- Line 130: `./esync.toml` → `./.esync.toml`
- Lines 480, 508, 532: `# esync.toml` → `# .esync.toml`

Also update any `esync.toml.example` to `.esync.toml.example`.

Be careful not to double-dot paths that already have a leading dot.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README references from esync.toml to .esync.toml"
```

---

### Task 4: Add EditTemplateTOML() to config package

**Files:**
- Modify: `internal/config/config.go` — add `EditTemplateTOML()` function after `DefaultTOML()`
- Modify: `internal/config/config_test.go` — add test

- [ ] **Step 1: Write test for EditTemplateTOML**

Add to `internal/config/config_test.go`:

```go
import (
	"strings"

	"github.com/spf13/viper"
)
```

```go
func TestEditTemplateTOMLIsValidTOML(t *testing.T) {
	content := EditTemplateTOML()
	if content == "" {
		t.Fatal("EditTemplateTOML returned empty string")
	}
	// Verify it contains the required fields
	if !strings.Contains(content, `local = "."`) {
		t.Fatal("missing local field")
	}
	if !strings.Contains(content, `remote = "user@host:/path/to/dest"`) {
		t.Fatal("missing remote field")
	}
	// Verify it parses as valid TOML
	v := viper.New()
	v.SetConfigType("toml")
	if err := v.ReadConfig(strings.NewReader(content)); err != nil {
		t.Fatalf("EditTemplateTOML is not valid TOML: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestEditTemplateTOML -v`
Expected: FAIL — `EditTemplateTOML` undefined

- [ ] **Step 3: Implement EditTemplateTOML**

Add to `internal/config/config.go` after `DefaultTOML()`:

```go
// EditTemplateTOML returns a minimal commented TOML template used by the
// TUI "e" key when no .esync.toml exists. Unlike DefaultTOML (used by
// esync init), most fields are commented out.
func EditTemplateTOML() string {
	return `# esync configuration
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
`
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add EditTemplateTOML for TUI config editing"
```

---

### Task 5: Add config reload channel and message types to AppModel

**Files:**
- Modify: `internal/tui/app.go` — add types, channel, accessor, resolveEditor, UpdatePaths

- [ ] **Step 1: Add new message types**

In `internal/tui/app.go`, add after the `editorFinishedMsg` type (line 30):

```go
// EditConfigMsg signals that the user wants to edit the config file.
type EditConfigMsg struct{}

// editorConfigFinishedMsg is sent when the config editor exits.
type editorConfigFinishedMsg struct{ err error }

// ConfigReloadedMsg signals that the config was reloaded with new paths.
type ConfigReloadedMsg struct {
	Local  string
	Remote string
}
```

- [ ] **Step 2: Add fields to AppModel**

Add to the `AppModel` struct (after `resyncCh` on line 44):

```go
configReloadCh chan *config.Config

// Config editor state
configTempFile string
configChecksum [32]byte
```

This requires adding `"crypto/sha256"` and `"github.com/louloulibs/esync/internal/config"` to the imports.

- [ ] **Step 3: Initialize channel in NewApp**

In `NewApp()`, add after `resyncCh` initialization:

```go
configReloadCh: make(chan *config.Config, 1),
```

- [ ] **Step 4: Add ConfigReloadChan accessor**

Add after `ResyncChan()`:

```go
// ConfigReloadChan returns a channel that receives a new config when the user
// edits and saves the config file from the TUI.
func (m *AppModel) ConfigReloadChan() <-chan *config.Config {
	return m.configReloadCh
}
```

- [ ] **Step 5: Add resolveEditor helper**

Add as a package-level function:

```go
// resolveEditor returns the user's preferred editor: $VISUAL, $EDITOR, or "vi".
func resolveEditor() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}
```

- [ ] **Step 6: Add UpdatePaths method**

Add after `ConfigReloadChan()`:

```go
// UpdatePaths updates the local and remote paths displayed in the dashboard.
// This must be called from the Bubbletea Update loop (via ConfigReloadedMsg),
// not from an external goroutine.
func (m *AppModel) updatePaths(local, remote string) {
	m.dashboard.local = local
	m.dashboard.remote = remote
}
```

Note: this is a private method — it will be called from within `Update()` when handling `ConfigReloadedMsg`, keeping all field mutations on the Bubbletea goroutine.

- [ ] **Step 7: Build to verify**

Run: `go build ./...`
Expected: Success (some new types unused for now, but Go only errors on unused imports, not unused types)

- [ ] **Step 8: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat: add config reload channel, message types, and helpers to AppModel"
```

---

### Task 6: Implement editor launch and config reload in AppModel.Update()

**Files:**
- Modify: `internal/tui/app.go` — handle `EditConfigMsg`, `editorConfigFinishedMsg`, and `ConfigReloadedMsg` in `Update()`

- [ ] **Step 1: Handle EditConfigMsg in Update()**

In the `Update()` switch (after the `OpenFileMsg` case block ending at line 137), add:

```go
case EditConfigMsg:
	configPath := ".esync.toml"
	var targetPath string

	if _, err := os.Stat(configPath); err == nil {
		// Existing file: checksum and edit in place
		data, err := os.ReadFile(configPath)
		if err != nil {
			return m, nil
		}
		m.configChecksum = sha256.Sum256(data)
		m.configTempFile = ""
		targetPath = configPath
	} else {
		// New file: write template to temp file
		tmpFile, err := os.CreateTemp("", "esync-*.toml")
		if err != nil {
			return m, nil
		}
		tmpl := config.EditTemplateTOML()
		tmpFile.WriteString(tmpl)
		tmpFile.Close()
		m.configChecksum = sha256.Sum256([]byte(tmpl))
		m.configTempFile = tmpFile.Name()
		targetPath = tmpFile.Name()
	}

	editor := resolveEditor()
	c := exec.Command(editor, targetPath)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return editorConfigFinishedMsg{err}
	})
```

- [ ] **Step 2: Handle editorConfigFinishedMsg in Update()**

Add after the `EditConfigMsg` case:

```go
case editorConfigFinishedMsg:
	if msg.err != nil {
		// Editor exited with error — discard
		if m.configTempFile != "" {
			os.Remove(m.configTempFile)
			m.configTempFile = ""
		}
		return m, nil
	}

	configPath := ".esync.toml"
	editedPath := configPath
	if m.configTempFile != "" {
		editedPath = m.configTempFile
	}

	data, err := os.ReadFile(editedPath)
	if err != nil {
		if m.configTempFile != "" {
			os.Remove(m.configTempFile)
			m.configTempFile = ""
		}
		return m, nil
	}

	newChecksum := sha256.Sum256(data)
	if newChecksum == m.configChecksum {
		// No changes
		if m.configTempFile != "" {
			os.Remove(m.configTempFile)
			m.configTempFile = ""
		}
		return m, nil
	}

	// Changed — if temp, persist to .esync.toml
	if m.configTempFile != "" {
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			m.dashboard.status = "error: could not write " + configPath
			os.Remove(m.configTempFile)
			m.configTempFile = ""
			return m, nil
		}
		os.Remove(m.configTempFile)
		m.configTempFile = ""
	}

	// Parse the new config
	cfg, err := config.Load(configPath)
	if err != nil {
		m.dashboard.status = "config error: " + err.Error()
		return m, nil
	}

	// Send to reload channel (non-blocking)
	select {
	case m.configReloadCh <- cfg:
	default:
	}
	return m, nil
```

- [ ] **Step 3: Handle ConfigReloadedMsg in Update()**

Add after the `editorConfigFinishedMsg` case:

```go
case ConfigReloadedMsg:
	m.updatePaths(msg.Local, msg.Remote)
	return m, nil
```

- [ ] **Step 4: Build to verify**

Run: `go build ./...`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go
git commit -m "feat: implement config editor launch and reload in AppModel"
```

---

### Task 7: Add "e" key binding and help line to dashboard

**Files:**
- Modify: `internal/tui/dashboard.go:143-215` — add `e` key case in `updateNormal()`
- Modify: `internal/tui/dashboard.go:370-378` — add `e config` to help line

- [ ] **Step 1: Add "e" key in updateNormal**

In `internal/tui/dashboard.go`, in `updateNormal()`, add a new case before `case "/"` (before line 208):

```go
case "e":
	return m, func() tea.Msg { return EditConfigMsg{} }
```

- [ ] **Step 2: Add "e config" to help line**

In the help line section (around lines 376-377), insert `e config` between `v view` and `l logs`:

```go
// Before
helpKey("v") + helpDesc("view") +
helpKey("l") + helpDesc("logs") +
// After
helpKey("v") + helpDesc("view") +
helpKey("e") + helpDesc("config") +
helpKey("l") + helpDesc("logs") +
```

- [ ] **Step 3: Build and run tests**

Run: `go build ./... && go test ./internal/tui/ -v`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/tui/dashboard.go
git commit -m "feat: add 'e' key binding for config editing in dashboard"
```

---

### Task 8: Add TriggerSync to watcher

**Files:**
- Modify: `internal/watcher/watcher.go` — add `TriggerSync()` method

- [ ] **Step 1: Add TriggerSync**

In `internal/watcher/watcher.go`, add after `Stop()` (after line 149):

```go
// TriggerSync immediately invokes the sync handler (bypasses debounce).
func (w *Watcher) TriggerSync() {
	w.debouncer.callback()
}
```

Note: This calls the callback directly rather than going through `Trigger()`, which would add a debounce delay. This preserves the existing resync-key behavior of immediate execution.

- [ ] **Step 2: Build to verify**

Run: `go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/watcher/watcher.go
git commit -m "feat: add TriggerSync for immediate sync without debounce"
```

---

### Task 9: Refactor runTUI to support config reload

**Files:**
- Modify: `cmd/sync.go:163-290` — extract `startWatching` helper, add reload goroutine with proper synchronization

This is the largest task. It refactors `runTUI` to:
1. Extract watcher/syncer setup into a reusable `startWatching` helper
2. Add a goroutine that listens for config reload and rebuilds
3. Protect shared `watchState` with a mutex
4. Use a `sync.WaitGroup` to wait for in-flight syncs during teardown
5. Route path updates through `p.Send()` to stay on the Bubbletea goroutine

- [ ] **Step 1: Add watchState struct and startWatching helper**

Add before `runTUI` in `cmd/sync.go`:

```go
// watchState holds the watcher and syncer that can be torn down and rebuilt.
type watchState struct {
	watcher  *watcher.Watcher
	cancel   context.CancelFunc
	inflight sync.WaitGroup
}

// startWatching creates a syncer, watcher, and sync handler from the given config.
// The handler pushes events to syncCh and logCh. Returns the watchState for teardown.
func startWatching(cfg *config.Config, syncCh chan<- tui.SyncEvent, logCh chan<- tui.LogEntry) (*watchState, error) {
	ctx, cancel := context.WithCancel(context.Background())

	s := syncer.New(cfg)
	s.DryRun = dryRun

	ws := &watchState{cancel: cancel}

	handler := func() {
		ws.inflight.Add(1)
		defer ws.inflight.Done()

		syncCh <- tui.SyncEvent{Status: "status:syncing"}

		var lastPct string
		onLine := func(line string) {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				return
			}
			select {
			case logCh <- tui.LogEntry{Time: time.Now(), Level: "INF", Message: trimmed}:
			default:
			}
			if m := reProgress2.FindStringSubmatch(trimmed); len(m) > 1 {
				pct := m[1]
				if pct != lastPct {
					lastPct = pct
					select {
					case syncCh <- tui.SyncEvent{Status: "status:syncing " + pct + "%"}:
					default:
					}
				}
			}
		}

		result, err := s.RunWithProgress(ctx, onLine)
		now := time.Now()

		if err != nil {
			syncCh <- tui.SyncEvent{
				File:   "sync error",
				Status: "error",
				Time:   now,
			}
			syncCh <- tui.SyncEvent{Status: "status:watching"}
			return
		}

		groups := groupFilesByTopLevel(result.Files)

		totalGroupBytes := int64(0)
		totalGroupFiles := 0
		for _, g := range groups {
			totalGroupBytes += g.bytes
			totalGroupFiles += g.count
		}

		for _, g := range groups {
			file := g.name
			bytes := g.bytes
			if totalGroupBytes == 0 && result.BytesTotal > 0 && totalGroupFiles > 0 {
				bytes = result.BytesTotal * int64(g.count) / int64(totalGroupFiles)
			}
			size := formatSize(bytes)
			syncCh <- tui.SyncEvent{
				File:      file,
				Size:      size,
				Duration:  result.Duration,
				Status:    "synced",
				Time:      now,
				Files:     truncateFiles(g.files, 10),
				FileCount: g.count,
			}
		}

		if len(groups) == 0 && result.FilesCount > 0 {
			syncCh <- tui.SyncEvent{
				File:     fmt.Sprintf("%d files", result.FilesCount),
				Size:     formatSize(result.BytesTotal),
				Duration: result.Duration,
				Status:   "synced",
				Time:     now,
			}
		}

		syncCh <- tui.SyncEvent{Status: "status:watching"}
	}

	w, err := watcher.New(
		cfg.Sync.Local,
		cfg.Settings.WatcherDebounce,
		cfg.AllIgnorePatterns(),
		cfg.Settings.Include,
		handler,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating watcher: %w", err)
	}

	if err := w.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting watcher: %w", err)
	}

	ws.watcher = w
	return ws, nil
}
```

- [ ] **Step 2: Rewrite runTUI**

Replace the entire `runTUI` function:

```go
func runTUI(cfg *config.Config, s *syncer.Syncer) error {
	app := tui.NewApp(cfg.Sync.Local, cfg.Sync.Remote)
	syncCh := app.SyncEventChan()
	logCh := app.LogEntryChan()

	ws, err := startWatching(cfg, syncCh, logCh)
	if err != nil {
		return err
	}

	var wsMu sync.Mutex

	// Handle resync requests
	resyncCh := app.ResyncChan()
	go func() {
		for range resyncCh {
			wsMu.Lock()
			w := ws
			wsMu.Unlock()
			w.watcher.TriggerSync()
		}
	}()

	p := tea.NewProgram(app, tea.WithAltScreen())

	// Handle config reload
	configCh := app.ConfigReloadChan()
	go func() {
		for newCfg := range configCh {
			wsMu.Lock()
			oldWs := ws
			wsMu.Unlock()

			// Tear down: stop watcher, wait for in-flight syncs
			oldWs.watcher.Stop()
			oldWs.inflight.Wait()
			oldWs.cancel()

			// Rebuild with new config
			newWs, err := startWatching(newCfg, syncCh, logCh)
			if err != nil {
				select {
				case syncCh <- tui.SyncEvent{Status: "status:error"}:
				default:
				}
				continue
			}

			wsMu.Lock()
			ws = newWs
			wsMu.Unlock()

			// Update paths via Bubbletea message (safe — goes through Update loop)
			p.Send(tui.ConfigReloadedMsg{
				Local:  newCfg.Sync.Local,
				Remote: newCfg.Sync.Remote,
			})
		}
	}()

	if _, err := p.Run(); err != nil {
		wsMu.Lock()
		w := ws
		wsMu.Unlock()
		w.watcher.Stop()
		w.cancel()
		return fmt.Errorf("TUI error: %w", err)
	}

	wsMu.Lock()
	w := ws
	wsMu.Unlock()
	w.watcher.Stop()
	w.cancel()
	return nil
}
```

Add `"sync"` to the imports in `cmd/sync.go`.

- [ ] **Step 3: Build and test**

Run: `go build ./... && go test ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add cmd/sync.go
git commit -m "refactor: extract startWatching, add config reload with mutex and WaitGroup"
```

---

### Task 10: End-to-end manual test

- [ ] **Step 1: Build the binary**

Run: `GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o esync-darwin-arm64 .`

- [ ] **Step 2: Test with existing config**

1. Create `.esync.toml` with valid local/remote
2. Run `./esync-darwin-arm64 sync`
3. Press `e` — verify editor opens with the config
4. Make a change (e.g., add an ignore pattern), save, exit
5. Verify TUI shows "watching" (config reloaded)

- [ ] **Step 3: Test new config creation**

1. Remove `.esync.toml`
2. Run `./esync-darwin-arm64 sync -l . -r user@host:/tmp/test`
3. Press `e` — verify editor opens with the template
4. Fill in `local`/`remote`, save, exit
5. Verify `.esync.toml` was created and TUI continues running

- [ ] **Step 4: Test discard on exit without save**

1. Remove `.esync.toml`
2. Run `./esync-darwin-arm64 sync -l . -r user@host:/tmp/test`
3. Press `e` — editor opens with template
4. Exit without saving (`:q!` in vim)
5. Verify no `.esync.toml` was created

- [ ] **Step 5: Test invalid config**

1. Press `e`, introduce a TOML syntax error, save
2. Verify TUI shows error in status line and keeps running with old config

- [ ] **Step 6: Run full test suite**

Run: `go test ./...`
Expected: All pass
