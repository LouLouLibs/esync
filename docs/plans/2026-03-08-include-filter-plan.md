# Include Filter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `settings.include` path-prefix filtering so users can narrow syncs to specific subtrees, applied to both watcher and rsync.

**Architecture:** A single `Include []string` field on `Settings` feeds into both the watcher (skip directories outside included prefixes) and the syncer (rsync `--include`/`--exclude` filter rules). Empty list means include everything (backwards compatible).

**Tech Stack:** Go, fsnotify, rsync filter rules

---

### Task 1: Add `Include` field to config

**Files:**
- Modify: `internal/config/config.go:54-60` (Settings struct)
- Modify: `internal/config/config.go:183-218` (DefaultTOML)
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestLoadConfigWithInclude(t *testing.T) {
	toml := `
[sync]
local  = "/src"
remote = "/dst"

[settings]
include = ["src", "docs/api"]
ignore  = [".git"]
`
	path := writeTempTOML(t, toml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Settings.Include) != 2 {
		t.Fatalf("Settings.Include length = %d, want 2", len(cfg.Settings.Include))
	}
	if cfg.Settings.Include[0] != "src" || cfg.Settings.Include[1] != "docs/api" {
		t.Errorf("Settings.Include = %v, want [src docs/api]", cfg.Settings.Include)
	}
}

func TestLoadConfigIncludeDefaultsToEmpty(t *testing.T) {
	toml := `
[sync]
local  = "/src"
remote = "/dst"
`
	path := writeTempTOML(t, toml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Settings.Include == nil {
		// nil is fine — treated as "include everything"
	} else if len(cfg.Settings.Include) != 0 {
		t.Errorf("Settings.Include = %v, want empty", cfg.Settings.Include)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadConfigWithInclude -v`
Expected: FAIL — `cfg.Settings.Include` has no such field

**Step 3: Write minimal implementation**

In `internal/config/config.go`, add `Include` field to `Settings` struct (line 57, after `InitialSync`):

```go
type Settings struct {
	WatcherDebounce int           `mapstructure:"watcher_debounce"`
	InitialSync     bool          `mapstructure:"initial_sync"`
	Include         []string      `mapstructure:"include"`
	Ignore          []string      `mapstructure:"ignore"`
	Rsync           RsyncSettings `mapstructure:"rsync"`
	Log             LogSettings   `mapstructure:"log"`
}
```

Update `DefaultTOML()` — add `include` line after `initial_sync` with a comment:

```go
// In the DefaultTOML string, after initial_sync line:
initial_sync     = false
# include: path prefixes to sync (relative to local). Empty means everything.
# Keep include simple and explicit; use ignore for fine-grained filtering.
include          = []
ignore           = [".git", "node_modules", ".DS_Store"]
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run "TestLoadConfigWithInclude|TestLoadConfigIncludeDefaultsToEmpty" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add Include field to Settings config"
```

---

### Task 2: Add `shouldInclude` to watcher

**Files:**
- Modify: `internal/watcher/watcher.go:88-94` (Watcher struct)
- Modify: `internal/watcher/watcher.go:100-120` (New function)
- Modify: `internal/watcher/watcher.go:205-224` (addRecursive)
- Modify: `internal/watcher/watcher.go:145-181` (eventLoop)
- Test: `internal/watcher/watcher_test.go`

**Step 1: Write the failing test**

Add to `internal/watcher/watcher_test.go`:

```go
func TestShouldInclude(t *testing.T) {
	w := &Watcher{
		rootPath: "/project",
		includes: []string{"src", "docs/api"},
	}

	tests := []struct {
		path   string
		expect bool
	}{
		// Files/dirs inside included prefixes
		{"/project/src/main.go", true},
		{"/project/src/pkg/util.go", true},
		{"/project/docs/api/readme.md", true},
		// Ancestor dirs needed for traversal
		{"/project/docs", true},
		// Outside included prefixes
		{"/project/tmp/cache.bin", false},
		{"/project/build/out.o", false},
		// Root itself is always included
		{"/project", true},
	}

	for _, tt := range tests {
		got := w.shouldInclude(tt.path)
		if got != tt.expect {
			t.Errorf("shouldInclude(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

func TestShouldIncludeEmptyMeansAll(t *testing.T) {
	w := &Watcher{
		rootPath: "/project",
		includes: nil,
	}

	tests := []struct {
		path   string
		expect bool
	}{
		{"/project/anything/at/all.go", true},
		{"/project/tmp/cache.bin", true},
	}

	for _, tt := range tests {
		got := w.shouldInclude(tt.path)
		if got != tt.expect {
			t.Errorf("shouldInclude(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/watcher/ -run "TestShouldInclude" -v`
Expected: FAIL — `rootPath` and `includes` fields don't exist, `shouldInclude` method doesn't exist

**Step 3: Write minimal implementation**

Add `includes` and `rootPath` fields to `Watcher` struct:

```go
type Watcher struct {
	fsw       *fsnotify.Watcher
	debouncer *Debouncer
	path      string
	rootPath  string
	ignores   []string
	includes  []string
	done      chan struct{}
}
```

Update `New()` signature to accept includes:

```go
func New(path string, debounceMs int, ignores []string, includes []string, handler EventHandler) (*Watcher, error) {
```

Set `rootPath` and `includes` in the constructor:

```go
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	w := &Watcher{
		fsw:      fsw,
		path:     path,
		rootPath: abs,
		ignores:  ignores,
		includes: includes,
		done:     make(chan struct{}),
	}
```

Add `shouldInclude` method:

```go
// shouldInclude checks whether path falls under an included prefix.
// If includes is empty, everything is included. A path is included if:
// - it IS a prefix of an include path (ancestor dir needed for traversal), or
// - it is prefixed BY an include path (file/dir inside included subtree).
func (w *Watcher) shouldInclude(path string) bool {
	if len(w.includes) == 0 {
		return true
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	rel, err := filepath.Rel(w.rootPath, abs)
	if err != nil || rel == "." {
		return true // root itself is always included
	}

	for _, inc := range w.includes {
		incClean := filepath.Clean(inc)
		// Path is inside the included prefix (e.g. rel="src/main.go", inc="src")
		if rel == incClean || strings.HasPrefix(rel, incClean+string(filepath.Separator)) {
			return true
		}
		// Path is an ancestor of the included prefix (e.g. rel="docs", inc="docs/api")
		if strings.HasPrefix(incClean, rel+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
```

Update `addRecursive` to check includes before watching:

```go
func (w *Watcher) addRecursive(path string) error {
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if w.shouldIgnore(p) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !w.shouldInclude(p) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return w.fsw.Add(p)
		}

		return nil
	})
}
```

Update `eventLoop` to check includes:

```go
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}

			if !isRelevantOp(event.Op) {
				continue
			}

			if w.shouldIgnore(event.Name) {
				continue
			}

			if !w.shouldInclude(event.Name) {
				continue
			}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/watcher/ -run "TestShouldInclude" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/watcher/watcher.go internal/watcher/watcher_test.go
git commit -m "feat: add shouldInclude path-prefix filtering to watcher"
```

---

### Task 3: Update watcher.New callers

**Files:**
- Modify: `cmd/sync.go:263-266` (TUI mode watcher init)
- Modify: `cmd/sync.go:359-362` (daemon mode watcher init)
- Modify: `integration_test.go:112` (integration test)

**Step 1: Update all `watcher.New()` call sites**

The signature changed from `New(path, debounceMs, ignores, handler)` to `New(path, debounceMs, ignores, includes, handler)`.

In `cmd/sync.go`, both call sites (around lines 263 and 359) currently look like:

```go
w, err := watcher.New(
    cfg.Sync.Local,
    cfg.Settings.WatcherDebounce,
    cfg.AllIgnorePatterns(),
    syncHandler,
)
```

Change to:

```go
w, err := watcher.New(
    cfg.Sync.Local,
    cfg.Settings.WatcherDebounce,
    cfg.AllIgnorePatterns(),
    cfg.Settings.Include,
    syncHandler,
)
```

In `integration_test.go` (line 112):

```go
w, err := watcher.New(src, 100, nil, nil, handler)
```

**Step 2: Verify it compiles and tests pass**

Run: `go build ./... && go test ./...`
Expected: all pass

**Step 3: Commit**

```bash
git add cmd/sync.go integration_test.go
git commit -m "feat: pass include patterns to watcher from config"
```

---

### Task 4: Add rsync include filter rules to syncer

**Files:**
- Modify: `internal/syncer/syncer.go:156-160` (exclude patterns section in BuildCommand)
- Test: `internal/syncer/syncer_test.go`

**Step 1: Write the failing test**

Add to `internal/syncer/syncer_test.go`:

```go
func TestBuildCommand_IncludePatterns(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	cfg.Settings.Include = []string{"src", "docs/api"}
	cfg.Settings.Ignore = []string{".git"}

	s := New(cfg)
	cmd := s.BuildCommand()

	// Should have include rules for parent dirs, subtrees, then excludes, then catch-all
	// Order: --include=src/ --include=src/** --include=docs/ --include=docs/api/ --include=docs/api/** --exclude=.git --exclude=*
	if !containsArg(cmd, "--include=src/") {
		t.Errorf("missing --include=src/ in %v", cmd)
	}
	if !containsArg(cmd, "--include=src/**") {
		t.Errorf("missing --include=src/** in %v", cmd)
	}
	if !containsArg(cmd, "--include=docs/") {
		t.Errorf("missing --include=docs/ in %v", cmd)
	}
	if !containsArg(cmd, "--include=docs/api/") {
		t.Errorf("missing --include=docs/api/ in %v", cmd)
	}
	if !containsArg(cmd, "--include=docs/api/**") {
		t.Errorf("missing --include=docs/api/** in %v", cmd)
	}
	if !containsArg(cmd, "--exclude=.git") {
		t.Errorf("missing --exclude=.git in %v", cmd)
	}
	if !containsArg(cmd, "--exclude=*") {
		t.Errorf("missing --exclude=* catch-all in %v", cmd)
	}

	// Verify ordering: all --include before --exclude=*
	lastInclude := -1
	catchAllExclude := -1
	for i, a := range cmd {
		if strings.HasPrefix(a, "--include=") {
			lastInclude = i
		}
		if a == "--exclude=*" {
			catchAllExclude = i
		}
	}
	if lastInclude >= catchAllExclude {
		t.Errorf("--include rules must come before --exclude=* catch-all")
	}
}

func TestBuildCommand_NoIncludeMeansNoFilterRules(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	cfg.Settings.Ignore = []string{".git"}

	s := New(cfg)
	cmd := s.BuildCommand()

	// Should NOT have --include or --exclude=* catch-all
	for _, a := range cmd {
		if strings.HasPrefix(a, "--include=") {
			t.Errorf("unexpected --include in %v", cmd)
		}
	}
	if containsArg(cmd, "--exclude=*") {
		t.Errorf("unexpected --exclude=* catch-all in %v", cmd)
	}
	// Regular excludes still present
	if !containsArg(cmd, "--exclude=.git") {
		t.Errorf("missing --exclude=.git in %v", cmd)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/syncer/ -run "TestBuildCommand_IncludePatterns|TestBuildCommand_NoIncludeMeansNoFilterRules" -v`
Expected: FAIL — no include logic exists yet

**Step 3: Write minimal implementation**

Replace the exclude patterns section in `BuildCommand()` (lines 156-160) with:

```go
	// Include/exclude filter rules
	if len(s.cfg.Settings.Include) > 0 {
		// Emit include rules: ancestor dirs + subtree for each prefix
		seen := make(map[string]bool)
		for _, inc := range s.cfg.Settings.Include {
			inc = filepath.Clean(inc)
			// Add ancestor directories (e.g. "docs/api" needs "docs/")
			parts := strings.Split(inc, string(filepath.Separator))
			for i := 1; i < len(parts); i++ {
				ancestor := strings.Join(parts[:i], "/") + "/"
				if !seen[ancestor] {
					args = append(args, "--include="+ancestor)
					seen[ancestor] = true
				}
			}
			// Add the prefix dir and everything underneath
			args = append(args, "--include="+inc+"/")
			args = append(args, "--include="+inc+"/**")
		}

		// Exclude patterns from ignore lists (applied within included paths)
		for _, pattern := range s.cfg.AllIgnorePatterns() {
			cleaned := strings.TrimPrefix(pattern, "**/")
			args = append(args, "--exclude="+cleaned)
		}

		// Catch-all exclude: block everything not explicitly included
		args = append(args, "--exclude=*")
	} else {
		// No include filter — just exclude patterns as before
		for _, pattern := range s.cfg.AllIgnorePatterns() {
			cleaned := strings.TrimPrefix(pattern, "**/")
			args = append(args, "--exclude="+cleaned)
		}
	}
```

Add `"path/filepath"` to the imports in `syncer.go` if not already present.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/syncer/ -run "TestBuildCommand_IncludePatterns|TestBuildCommand_NoIncludeMeansNoFilterRules" -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./...`
Expected: all pass

**Step 6: Commit**

```bash
git add internal/syncer/syncer.go internal/syncer/syncer_test.go
git commit -m "feat: emit rsync include/exclude filter rules from config"
```

---

### Task 5: Update DefaultTOML test and verify end-to-end

**Files:**
- Modify: `internal/config/config_test.go` (TestDefaultTOML)

**Step 1: Update TestDefaultTOML to verify include is documented**

Add a check in the existing `TestDefaultTOML`:

```go
// In TestDefaultTOML, add to the sections check:
if !containsString(toml, "include") {
    t.Error("DefaultTOML() missing include field")
}
```

**Step 2: Run all tests**

Run: `go test ./...`
Expected: all pass

**Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test: verify DefaultTOML includes the include field"
```
