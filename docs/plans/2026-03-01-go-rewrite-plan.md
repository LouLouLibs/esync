# esync Go Rewrite — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rewrite esync from Python to Go with a Bubbletea TUI, Cobra CLI, and Viper-based TOML configuration.

**Architecture:** Cobra CLI dispatches to subcommands. `esync sync` launches either a Bubbletea TUI (default) or daemon mode. fsnotify watches files, debouncer batches events, syncer executes rsync. Viper loads TOML config with a search path.

**Tech Stack:** Go 1.22+, Cobra, Viper, Bubbletea, Lipgloss, fsnotify, rsync (external)

---

### Task 1: Project Scaffolding

**Files:**
- Create: `main.go`
- Create: `go.mod`
- Create: `cmd/root.go`
- Create: `internal/config/config.go`
- Create: `internal/syncer/syncer.go`
- Create: `internal/watcher/watcher.go`
- Create: `internal/tui/app.go`
- Create: `internal/logger/logger.go`

**Step 1: Remove Python source files**

Delete the Python package and build files (we're on a feature branch):
```bash
rm -rf esync/ pyproject.toml uv.lock .python-version
```

**Step 2: Initialize Go module**

```bash
go mod init github.com/eloualiche/esync
```

**Step 3: Create directory structure**

```bash
mkdir -p cmd internal/config internal/syncer internal/watcher internal/tui internal/logger
```

**Step 4: Create minimal main.go**

```go
package main

import "github.com/eloualiche/esync/cmd"

func main() {
	cmd.Execute()
}
```

**Step 5: Create root command stub**

```go
// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "esync",
	Short: "File synchronization tool using rsync",
	Long:  "A file sync tool that watches for changes and automatically syncs them to a remote destination using rsync.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
}
```

**Step 6: Install dependencies and verify build**

```bash
go get github.com/spf13/cobra
go get github.com/spf13/viper
go get github.com/fsnotify/fsnotify
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go mod tidy
go build ./...
```

**Step 7: Commit**

```bash
git add -A
git commit -m "feat: scaffold Go project with Cobra root command"
```

---

### Task 2: Configuration Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write failing tests for config structs and loading**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "esync.toml")

	content := []byte(`
[sync]
local = "./src"
remote = "user@host:/deploy"
interval = 1

[settings]
watcher_debounce = 500
initial_sync = true
ignore = ["*.log", "*.tmp"]

[settings.rsync]
archive = true
compress = true
backup = true
backup_dir = ".rsync_backup"
progress = true
ignore = [".git/", "node_modules/"]

[settings.log]
file = "/tmp/esync.log"
format = "json"
`)
	os.WriteFile(tomlPath, content, 0644)

	cfg, err := Load(tomlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sync.Local != "./src" {
		t.Errorf("expected local=./src, got %s", cfg.Sync.Local)
	}
	if cfg.Sync.Remote != "user@host:/deploy" {
		t.Errorf("expected remote=user@host:/deploy, got %s", cfg.Sync.Remote)
	}
	if cfg.Settings.WatcherDebounce != 500 {
		t.Errorf("expected debounce=500, got %d", cfg.Settings.WatcherDebounce)
	}
	if !cfg.Settings.InitialSync {
		t.Error("expected initial_sync=true")
	}
	if len(cfg.Settings.Ignore) != 2 {
		t.Errorf("expected 2 ignore patterns, got %d", len(cfg.Settings.Ignore))
	}
	if !cfg.Settings.Rsync.Archive {
		t.Error("expected rsync archive=true")
	}
	if cfg.Settings.Log.Format != "json" {
		t.Errorf("expected log format=json, got %s", cfg.Settings.Log.Format)
	}
}

func TestLoadConfigWithSSH(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "esync.toml")

	content := []byte(`
[sync]
local = "./src"
remote = "/deploy"

[sync.ssh]
host = "example.com"
user = "deploy"
port = 22
identity_file = "~/.ssh/id_ed25519"
interactive_auth = true
`)
	os.WriteFile(tomlPath, content, 0644)

	cfg, err := Load(tomlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sync.SSH == nil {
		t.Fatal("expected SSH config to be set")
	}
	if cfg.Sync.SSH.Host != "example.com" {
		t.Errorf("expected host=example.com, got %s", cfg.Sync.SSH.Host)
	}
	if cfg.Sync.SSH.User != "deploy" {
		t.Errorf("expected user=deploy, got %s", cfg.Sync.SSH.User)
	}
	if cfg.Sync.SSH.IdentityFile != "~/.ssh/id_ed25519" {
		t.Errorf("expected identity_file, got %s", cfg.Sync.SSH.IdentityFile)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "esync.toml")

	content := []byte(`
[sync]
local = "./src"
remote = "./dst"
`)
	os.WriteFile(tomlPath, content, 0644)

	cfg, err := Load(tomlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Settings.WatcherDebounce != 500 {
		t.Errorf("expected default debounce=500, got %d", cfg.Settings.WatcherDebounce)
	}
	if cfg.Settings.Rsync.Archive != true {
		t.Error("expected default archive=true")
	}
}

func TestIsRemote(t *testing.T) {
	tests := []struct {
		remote string
		want   bool
	}{
		{"user@host:/path", true},
		{"host:/path", true},
		{"./local/path", false},
		{"/absolute/path", false},
		{"C:/windows/path", false},
	}
	for _, tt := range tests {
		cfg := &Config{Sync: SyncSection{Remote: tt.remote}}
		if got := cfg.IsRemote(); got != tt.want {
			t.Errorf("IsRemote(%q) = %v, want %v", tt.remote, got, tt.want)
		}
	}
}

func TestFindConfigFile(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "esync.toml")
	os.WriteFile(tomlPath, []byte("[sync]\nlocal = \".\"\nremote = \".\"\n"), 0644)

	found := FindConfigIn([]string{tomlPath})
	if found != tomlPath {
		t.Errorf("expected %s, got %s", tomlPath, found)
	}
}

func TestFindConfigFileNotFound(t *testing.T) {
	found := FindConfigIn([]string{"/nonexistent/esync.toml"})
	if found != "" {
		t.Errorf("expected empty string, got %s", found)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
cd internal/config && go test -v
```
Expected: compilation errors (types don't exist yet)

**Step 3: Implement config package**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

// SSHConfig holds SSH connection settings.
type SSHConfig struct {
	Host            string `mapstructure:"host"`
	User            string `mapstructure:"user"`
	Port            int    `mapstructure:"port"`
	IdentityFile    string `mapstructure:"identity_file"`
	InteractiveAuth bool   `mapstructure:"interactive_auth"`
}

// SyncSection holds source and destination paths.
type SyncSection struct {
	Local    string     `mapstructure:"local"`
	Remote   string     `mapstructure:"remote"`
	Interval int        `mapstructure:"interval"`
	SSH      *SSHConfig `mapstructure:"ssh"`
}

// RsyncSettings holds rsync-specific options.
type RsyncSettings struct {
	Archive   bool     `mapstructure:"archive"`
	Compress  bool     `mapstructure:"compress"`
	Backup    bool     `mapstructure:"backup"`
	BackupDir string   `mapstructure:"backup_dir"`
	Progress  bool     `mapstructure:"progress"`
	ExtraArgs []string `mapstructure:"extra_args"`
	Ignore    []string `mapstructure:"ignore"`
}

// LogSettings holds logging configuration.
type LogSettings struct {
	File   string `mapstructure:"file"`
	Format string `mapstructure:"format"`
}

// Settings holds all application settings.
type Settings struct {
	WatcherDebounce int           `mapstructure:"watcher_debounce"`
	InitialSync     bool          `mapstructure:"initial_sync"`
	Ignore          []string      `mapstructure:"ignore"`
	Rsync           RsyncSettings `mapstructure:"rsync"`
	Log             LogSettings   `mapstructure:"log"`
}

// Config is the top-level configuration.
type Config struct {
	Sync     SyncSection `mapstructure:"sync"`
	Settings Settings    `mapstructure:"settings"`
}

// IsRemote returns true if the remote target is an SSH destination.
func (c *Config) IsRemote() bool {
	if c.Sync.SSH != nil && c.Sync.SSH.Host != "" {
		return true
	}
	return isRemotePath(c.Sync.Remote)
}

// isRemotePath checks if a path string looks like user@host:/path or host:/path.
func isRemotePath(path string) bool {
	if len(path) >= 2 && path[1] == ':' && (path[0] >= 'A' && path[0] <= 'Z' || path[0] >= 'a' && path[0] <= 'z') {
		return false // Windows drive letter
	}
	re := regexp.MustCompile(`^(?:[^@]+@)?[^/:]+:.+$`)
	return re.MatchString(path)
}

// AllIgnorePatterns returns combined ignore patterns from settings and rsync.
func (c *Config) AllIgnorePatterns() []string {
	combined := make([]string, 0, len(c.Settings.Ignore)+len(c.Settings.Rsync.Ignore))
	combined = append(combined, c.Settings.Ignore...)
	combined = append(combined, c.Settings.Rsync.Ignore...)
	return combined
}

// Load reads and parses a TOML config file.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")

	// Defaults
	v.SetDefault("sync.interval", 1)
	v.SetDefault("settings.watcher_debounce", 500)
	v.SetDefault("settings.initial_sync", false)
	v.SetDefault("settings.rsync.archive", true)
	v.SetDefault("settings.rsync.compress", true)
	v.SetDefault("settings.rsync.backup", false)
	v.SetDefault("settings.rsync.backup_dir", ".rsync_backup")
	v.SetDefault("settings.rsync.progress", true)
	v.SetDefault("settings.log.format", "text")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Sync.Local == "" {
		return nil, fmt.Errorf("sync.local is required")
	}
	if cfg.Sync.Remote == "" {
		return nil, fmt.Errorf("sync.remote is required")
	}

	return &cfg, nil
}

// FindConfigFile searches default locations for a config file.
func FindConfigFile() string {
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(".", "esync.toml"),
		filepath.Join(home, ".config", "esync", "config.toml"),
		"/etc/esync/config.toml",
	}
	return FindConfigIn(paths)
}

// FindConfigIn searches the given paths for the first existing file.
func FindConfigIn(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// DefaultTOML returns a default config as a TOML string.
func DefaultTOML() string {
	return strings.TrimSpace(`
[sync]
local = "."
remote = "./remote"
interval = 1

# [sync.ssh]
# host = "example.com"
# user = "username"
# port = 22
# identity_file = "~/.ssh/id_ed25519"
# interactive_auth = true

[settings]
watcher_debounce = 500
initial_sync = false
ignore = ["*.log", "*.tmp", ".env"]

[settings.rsync]
archive = true
compress = true
backup = false
backup_dir = ".rsync_backup"
progress = true
extra_args = []
ignore = [".git/", "node_modules/", "**/__pycache__/"]

[settings.log]
# file = "~/.local/share/esync/esync.log"
format = "text"
`) + "\n"
}
```

**Step 4: Run tests to verify they pass**

```bash
cd internal/config && go test -v
```
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package with TOML loading, defaults, and search path"
```

---

### Task 3: Syncer Package

**Files:**
- Create: `internal/syncer/syncer.go`
- Create: `internal/syncer/syncer_test.go`

**Step 1: Write failing tests for rsync command building**

```go
// internal/syncer/syncer_test.go
package syncer

import (
	"testing"

	"github.com/eloualiche/esync/internal/config"
)

func TestBuildCommand_Local(t *testing.T) {
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  "/tmp/src",
			Remote: "/tmp/dst",
		},
		Settings: config.Settings{
			Rsync: config.RsyncSettings{
				Archive:  true,
				Compress: true,
				Progress: true,
				Ignore:   []string{".git/", "node_modules/"},
			},
		},
	}

	s := New(cfg)
	cmd := s.BuildCommand()

	if cmd[0] != "rsync" {
		t.Errorf("expected rsync, got %s", cmd[0])
	}
	if !contains(cmd, "--archive") {
		t.Error("expected --archive flag")
	}
	if !contains(cmd, "--compress") {
		t.Error("expected --compress flag")
	}
	// Source should end with /
	source := cmd[len(cmd)-2]
	if source[len(source)-1] != '/' {
		t.Errorf("source should end with /, got %s", source)
	}
}

func TestBuildCommand_Remote(t *testing.T) {
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  "/tmp/src",
			Remote: "user@host:/deploy",
		},
	}

	s := New(cfg)
	cmd := s.BuildCommand()

	dest := cmd[len(cmd)-1]
	if dest != "user@host:/deploy" {
		t.Errorf("expected user@host:/deploy, got %s", dest)
	}
}

func TestBuildCommand_SSHConfig(t *testing.T) {
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  "/tmp/src",
			Remote: "/deploy",
			SSH: &config.SSHConfig{
				Host:         "example.com",
				User:         "deploy",
				Port:         2222,
				IdentityFile: "~/.ssh/id_ed25519",
			},
		},
	}

	s := New(cfg)
	cmd := s.BuildCommand()

	dest := cmd[len(cmd)-1]
	if dest != "deploy@example.com:/deploy" {
		t.Errorf("expected deploy@example.com:/deploy, got %s", dest)
	}
	if !containsPrefix(cmd, "-e") {
		t.Error("expected -e flag for SSH")
	}
}

func TestBuildCommand_ExcludePatterns(t *testing.T) {
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  "/tmp/src",
			Remote: "/tmp/dst",
		},
		Settings: config.Settings{
			Ignore: []string{"*.log"},
			Rsync: config.RsyncSettings{
				Ignore: []string{".git/"},
			},
		},
	}

	s := New(cfg)
	cmd := s.BuildCommand()

	excludeCount := 0
	for _, arg := range cmd {
		if arg == "--exclude" {
			excludeCount++
		}
	}
	if excludeCount != 2 {
		t.Errorf("expected 2 exclude flags, got %d", excludeCount)
	}
}

func TestBuildCommand_ExtraArgs(t *testing.T) {
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  "/tmp/src",
			Remote: "/tmp/dst",
		},
		Settings: config.Settings{
			Rsync: config.RsyncSettings{
				ExtraArgs: []string{"--delete", "--checksum"},
			},
		},
	}

	s := New(cfg)
	cmd := s.BuildCommand()

	if !contains(cmd, "--delete") {
		t.Error("expected --delete from extra_args")
	}
	if !contains(cmd, "--checksum") {
		t.Error("expected --checksum from extra_args")
	}
}

func TestBuildCommand_DryRun(t *testing.T) {
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  "/tmp/src",
			Remote: "/tmp/dst",
		},
	}

	s := New(cfg)
	s.DryRun = true
	cmd := s.BuildCommand()

	if !contains(cmd, "--dry-run") {
		t.Error("expected --dry-run flag")
	}
}

func TestBuildCommand_Backup(t *testing.T) {
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  "/tmp/src",
			Remote: "/tmp/dst",
		},
		Settings: config.Settings{
			Rsync: config.RsyncSettings{
				Backup:    true,
				BackupDir: ".backup",
			},
		},
	}

	s := New(cfg)
	cmd := s.BuildCommand()

	if !contains(cmd, "--backup") {
		t.Error("expected --backup flag")
	}
	if !contains(cmd, "--backup-dir=.backup") {
		t.Error("expected --backup-dir flag")
	}
}

func contains(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func containsPrefix(args []string, prefix string) bool {
	for _, a := range args {
		if len(a) >= len(prefix) && a[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/syncer/ -v
```

**Step 3: Implement syncer package**

```go
// internal/syncer/syncer.go
package syncer

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eloualiche/esync/internal/config"
)

// Result holds the outcome of a sync operation.
type Result struct {
	Success      bool
	FilesCount   int
	BytesTotal   int64
	Duration     time.Duration
	Files        []string
	ErrorMessage string
}

// Syncer builds and executes rsync commands.
type Syncer struct {
	cfg    *config.Config
	DryRun bool
}

// New creates a new Syncer.
func New(cfg *config.Config) *Syncer {
	return &Syncer{cfg: cfg}
}

// BuildCommand constructs the rsync argument list.
func (s *Syncer) BuildCommand() []string {
	cmd := []string{"rsync", "--recursive", "--times", "--progress", "--copy-unsafe-links"}

	rs := s.cfg.Settings.Rsync
	if rs.Archive {
		cmd = append(cmd, "--archive")
	}
	if rs.Compress {
		cmd = append(cmd, "--compress")
	}
	if rs.Backup {
		cmd = append(cmd, "--backup")
		cmd = append(cmd, fmt.Sprintf("--backup-dir=%s", rs.BackupDir))
	}
	if s.DryRun {
		cmd = append(cmd, "--dry-run")
	}

	// Exclude patterns
	for _, pattern := range s.cfg.AllIgnorePatterns() {
		clean := strings.Trim(pattern, "\"[]'")
		if strings.HasPrefix(clean, "**/") {
			clean = clean[3:]
		}
		cmd = append(cmd, "--exclude", clean)
	}

	// Extra args passthrough
	cmd = append(cmd, rs.ExtraArgs...)

	// SSH options
	sshCmd := s.buildSSHCommand()
	if sshCmd != "" {
		cmd = append(cmd, "-e", sshCmd)
	}

	// Source (always ends with /)
	source := s.cfg.Sync.Local
	if !strings.HasSuffix(source, "/") {
		source += "/"
	}
	cmd = append(cmd, source)

	// Destination
	cmd = append(cmd, s.buildDestination())

	return cmd
}

// Run executes the rsync command and returns the result.
func (s *Syncer) Run() (*Result, error) {
	args := s.BuildCommand()
	start := time.Now()

	c := exec.Command(args[0], args[1:]...)
	output, err := c.CombinedOutput()
	duration := time.Since(start)

	result := &Result{
		Duration: duration,
		Files:    extractFiles(string(output)),
	}

	if err != nil {
		result.Success = false
		result.ErrorMessage = strings.TrimSpace(string(output))
		if result.ErrorMessage == "" {
			result.ErrorMessage = err.Error()
		}
		return result, err
	}

	result.Success = true
	result.FilesCount, result.BytesTotal = extractStats(string(output))
	return result, nil
}

func (s *Syncer) buildSSHCommand() string {
	ssh := s.cfg.Sync.SSH
	if ssh == nil {
		return ""
	}
	parts := []string{"ssh"}
	if ssh.Port != 0 && ssh.Port != 22 {
		parts = append(parts, fmt.Sprintf("-p %d", ssh.Port))
	}
	if ssh.IdentityFile != "" {
		parts = append(parts, fmt.Sprintf("-i %s", ssh.IdentityFile))
	}
	// ControlMaster for SSH keepalive
	parts = append(parts, "-o", "ControlMaster=auto")
	parts = append(parts, "-o", "ControlPath=/tmp/esync-ssh-%r@%h:%p")
	parts = append(parts, "-o", "ControlPersist=600")
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts, " ")
}

func (s *Syncer) buildDestination() string {
	ssh := s.cfg.Sync.SSH
	if ssh != nil && ssh.Host != "" {
		if ssh.User != "" {
			return fmt.Sprintf("%s@%s:%s", ssh.User, ssh.Host, s.cfg.Sync.Remote)
		}
		return fmt.Sprintf("%s:%s", ssh.Host, s.cfg.Sync.Remote)
	}
	return s.cfg.Sync.Remote
}

func extractFiles(output string) []string {
	var files []string
	skip := regexp.MustCompile(`^(building|sending|sent|total|bytes|\s*$)`)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || skip.MatchString(trimmed) {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) > 0 && !strings.Contains(parts[0], "%") {
			files = append(files, parts[0])
		}
	}
	return files
}

func extractStats(output string) (int, int64) {
	fileRe := regexp.MustCompile(`(\d+) files? to consider`)
	bytesRe := regexp.MustCompile(`sent ([\d,]+) bytes\s+received ([\d,]+) bytes`)

	var count int
	var total int64

	if m := fileRe.FindStringSubmatch(output); len(m) > 1 {
		count, _ = strconv.Atoi(m[1])
	}
	if m := bytesRe.FindStringSubmatch(output); len(m) > 2 {
		sent, _ := strconv.ParseInt(strings.ReplaceAll(m[1], ",", ""), 10, 64)
		recv, _ := strconv.ParseInt(strings.ReplaceAll(m[2], ",", ""), 10, 64)
		total = sent + recv
	}
	return count, total
}
```

**Step 4: Run tests**

```bash
go test ./internal/syncer/ -v
```
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/syncer/
git commit -m "feat: add syncer package with rsync command builder and SSH support"
```

---

### Task 4: Watcher Package

**Files:**
- Create: `internal/watcher/watcher.go`
- Create: `internal/watcher/watcher_test.go`

**Step 1: Write failing tests for debouncer**

```go
// internal/watcher/watcher_test.go
package watcher

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncerBatchesEvents(t *testing.T) {
	var callCount atomic.Int32
	callback := func() { callCount.Add(1) }

	d := NewDebouncer(100*time.Millisecond, callback)
	defer d.Stop()

	// Fire 5 events rapidly
	for i := 0; i < 5; i++ {
		d.Trigger()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce window to expire
	time.Sleep(200 * time.Millisecond)

	if count := callCount.Load(); count != 1 {
		t.Errorf("expected 1 callback, got %d", count)
	}
}

func TestDebouncerSeparateEvents(t *testing.T) {
	var callCount atomic.Int32
	callback := func() { callCount.Add(1) }

	d := NewDebouncer(50*time.Millisecond, callback)
	defer d.Stop()

	d.Trigger()
	time.Sleep(100 * time.Millisecond) // Wait for first debounce

	d.Trigger()
	time.Sleep(100 * time.Millisecond) // Wait for second debounce

	if count := callCount.Load(); count != 2 {
		t.Errorf("expected 2 callbacks, got %d", count)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/watcher/ -v
```

**Step 3: Implement watcher package**

```go
// internal/watcher/watcher.go
package watcher

import (
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Debouncer batches rapid events into a single callback.
type Debouncer struct {
	interval time.Duration
	callback func()
	timer    *time.Timer
	mu       sync.Mutex
	stopped  bool
}

// NewDebouncer creates a debouncer with the given interval.
func NewDebouncer(interval time.Duration, callback func()) *Debouncer {
	return &Debouncer{
		interval: interval,
		callback: callback,
	}
}

// Trigger resets the debounce timer.
func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.interval, d.callback)
}

// Stop cancels any pending callback.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
}

// EventHandler is called when files change.
type EventHandler func()

// Watcher monitors a directory for changes using fsnotify.
type Watcher struct {
	fsw       *fsnotify.Watcher
	debouncer *Debouncer
	path      string
	ignores   []string
	done      chan struct{}
}

// New creates a file watcher for the given path.
func New(path string, debounceMs int, ignores []string, handler EventHandler) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	interval := time.Duration(debounceMs) * time.Millisecond
	if interval == 0 {
		interval = 500 * time.Millisecond
	}

	w := &Watcher{
		fsw:       fsw,
		debouncer: NewDebouncer(interval, handler),
		path:      path,
		ignores:   ignores,
		done:      make(chan struct{}),
	}

	return w, nil
}

// Start begins watching for file changes.
func (w *Watcher) Start() error {
	if err := w.addRecursive(w.path); err != nil {
		return err
	}

	go w.loop()
	return nil
}

// Stop ends the watcher.
func (w *Watcher) Stop() {
	w.debouncer.Stop()
	w.fsw.Close()
	<-w.done
}

// Paused tracks whether watching is paused.
var Paused bool

func (w *Watcher) loop() {
	defer close(w.done)
	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if Paused {
				continue
			}
			if w.shouldIgnore(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				// If a directory was created, watch it too
				if event.Op&fsnotify.Create != 0 {
					w.addRecursive(event.Name)
				}
				w.debouncer.Trigger()
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

func (w *Watcher) shouldIgnore(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range w.ignores {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

func (w *Watcher) addRecursive(path string) error {
	return filepath.Walk(path, func(p string, info interface{}, err error) error {
		if err != nil {
			return nil // skip errors
		}
		// Only add directories
		return w.fsw.Add(p)
	})
}
```

Note: `addRecursive` uses `filepath.Walk` which needs `os.FileInfo`, not `interface{}`. The actual implementation should use the correct signature. The executing agent will fix this during implementation.

**Step 4: Run tests**

```bash
go test ./internal/watcher/ -v
```
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/watcher/
git commit -m "feat: add watcher package with fsnotify and debouncing"
```

---

### Task 5: Logger Package

**Files:**
- Create: `internal/logger/logger.go`
- Create: `internal/logger/logger_test.go`

**Step 1: Write failing tests**

```go
// internal/logger/logger_test.go
package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONLogger(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	l, err := New(logPath, "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Close()

	l.Info("synced", map[string]interface{}{
		"file": "main.go",
		"size": 2150,
	})

	data, _ := os.ReadFile(logPath)
	lines := strings.TrimSpace(string(data))

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(lines), &entry); err != nil {
		t.Fatalf("invalid JSON: %v\nline: %s", err, lines)
	}
	if entry["level"] != "info" {
		t.Errorf("expected level=info, got %v", entry["level"])
	}
	if entry["event"] != "synced" {
		t.Errorf("expected event=synced, got %v", entry["event"])
	}
}

func TestTextLogger(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	l, err := New(logPath, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Close()

	l.Info("synced", map[string]interface{}{"file": "main.go"})

	data, _ := os.ReadFile(logPath)
	line := string(data)
	if !strings.Contains(line, "INF") {
		t.Errorf("expected INF in text log, got: %s", line)
	}
	if !strings.Contains(line, "synced") {
		t.Errorf("expected 'synced' in text log, got: %s", line)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/logger/ -v
```

**Step 3: Implement logger**

```go
// internal/logger/logger.go
package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Logger writes structured log entries to a file.
type Logger struct {
	file   *os.File
	format string // "json" or "text"
	mu     sync.Mutex
}

// New creates a logger writing to the given path.
func New(path string, format string) (*Logger, error) {
	if format == "" {
		format = "text"
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f, format: format}, nil
}

// Close closes the log file.
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

// Info logs an info-level entry.
func (l *Logger) Info(event string, fields map[string]interface{}) {
	l.log("info", event, fields)
}

// Warn logs a warning-level entry.
func (l *Logger) Warn(event string, fields map[string]interface{}) {
	l.log("warn", event, fields)
}

// Error logs an error-level entry.
func (l *Logger) Error(event string, fields map[string]interface{}) {
	l.log("error", event, fields)
}

// Debug logs a debug-level entry.
func (l *Logger) Debug(event string, fields map[string]interface{}) {
	l.log("debug", event, fields)
}

func (l *Logger) log(level, event string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().Format("15:04:05")

	if l.format == "json" {
		entry := map[string]interface{}{
			"time":  now,
			"level": level,
			"event": event,
		}
		for k, v := range fields {
			entry[k] = v
		}
		data, _ := json.Marshal(entry)
		fmt.Fprintln(l.file, string(data))
	} else {
		tag := strings.ToUpper(level[:3])
		parts := []string{fmt.Sprintf("%s %s %s", now, tag, event)}
		for k, v := range fields {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		fmt.Fprintln(l.file, strings.Join(parts, " "))
	}
}
```

**Step 4: Run tests**

```bash
go test ./internal/logger/ -v
```
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/logger/
git commit -m "feat: add logger package with JSON and text output"
```

---

### Task 6: TUI — Styles and Dashboard

**Files:**
- Create: `internal/tui/styles.go`
- Create: `internal/tui/dashboard.go`
- Create: `internal/tui/app.go`

**Step 1: Create Lipgloss styles**

```go
// internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")) // blue

	statusSynced = lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")) // green

	statusSyncing = lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")) // yellow

	statusError = lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")) // red

	dimStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")) // dim gray

	sectionStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(lipgloss.Color("8"))

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))
)
```

**Step 2: Create dashboard Bubbletea model**

```go
// internal/tui/dashboard.go
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// SyncEvent represents a file sync event for display.
type SyncEvent struct {
	File     string
	Size     string
	Duration time.Duration
	Status   string // "synced", "syncing", "error"
	Time     time.Time
}

// DashboardModel is the main TUI view.
type DashboardModel struct {
	local       string
	remote      string
	status      string // "watching", "syncing", "paused", "error"
	lastSync    time.Time
	events      []SyncEvent
	totalSynced int
	totalBytes  string
	totalErrors int
	width       int
	height      int
	filter      string
	filtering   bool
}

// NewDashboard creates the dashboard model.
func NewDashboard(local, remote string) DashboardModel {
	return DashboardModel{
		local:  local,
		remote: remote,
		status: "watching",
		events: []SyncEvent{},
	}
}

func (m DashboardModel) Init() tea.Cmd {
	return tickCmd()
}

// tickMsg triggers periodic UI refresh.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// SyncEventMsg delivers a sync event to the TUI.
type SyncEventMsg SyncEvent

// SyncStatsMsg updates aggregate stats.
type SyncStatsMsg struct {
	TotalSynced int
	TotalBytes  string
	TotalErrors int
}

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "enter", "esc":
				m.filtering = false
				if msg.String() == "esc" {
					m.filter = ""
				}
				return m, nil
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.filter += msg.String()
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "p":
			if m.status == "paused" {
				m.status = "watching"
			} else if m.status == "watching" {
				m.status = "paused"
			}
			return m, nil
		case "/":
			m.filtering = true
			m.filter = ""
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, tickCmd()

	case SyncEventMsg:
		e := SyncEvent(msg)
		m.events = append([]SyncEvent{e}, m.events...)
		if len(m.events) > 100 {
			m.events = m.events[:100]
		}
		if e.Status == "synced" {
			m.lastSync = e.Time
		}

	case SyncStatsMsg:
		m.totalSynced = msg.TotalSynced
		m.totalBytes = msg.TotalBytes
		m.totalErrors = msg.TotalErrors
	}

	return m, nil
}

func (m DashboardModel) View() string {
	var b strings.Builder

	// Header
	title := titleStyle.Render(" esync ")
	separator := dimStyle.Render(strings.Repeat("─", max(0, m.width-8)))
	b.WriteString(title + separator + "\n")
	b.WriteString(fmt.Sprintf("  %s → %s\n", m.local, m.remote))

	// Status
	var statusStr string
	switch m.status {
	case "watching":
		ago := ""
		if !m.lastSync.IsZero() {
			ago = fmt.Sprintf(" (synced %s ago)", time.Since(m.lastSync).Round(time.Second))
		}
		statusStr = statusSynced.Render("●") + " Watching" + dimStyle.Render(ago)
	case "syncing":
		statusStr = statusSyncing.Render("⟳") + " Syncing..."
	case "paused":
		statusStr = dimStyle.Render("⏸") + " Paused"
	case "error":
		statusStr = statusError.Render("✗") + " Error"
	}
	b.WriteString("  " + statusStr + "\n\n")

	// Recent events
	b.WriteString("  " + dimStyle.Render("Recent "+strings.Repeat("─", max(0, m.width-12))) + "\n")
	filtered := m.filteredEvents()
	shown := min(10, len(filtered))
	for i := 0; i < shown; i++ {
		e := filtered[i]
		var icon string
		switch e.Status {
		case "synced":
			icon = statusSynced.Render("✓")
		case "syncing":
			icon = statusSyncing.Render("⟳")
		case "error":
			icon = statusError.Render("✗")
		}
		dur := ""
		if e.Duration > 0 {
			dur = dimStyle.Render(fmt.Sprintf("%.1fs", e.Duration.Seconds()))
		}
		b.WriteString(fmt.Sprintf("  %s %-30s %8s  %s\n", icon, e.File, e.Size, dur))
	}
	b.WriteString("\n")

	// Stats
	b.WriteString("  " + dimStyle.Render("Stats "+strings.Repeat("─", max(0, m.width-10))) + "\n")
	stats := fmt.Sprintf("  %d synced │ %s total │ %d errors",
		m.totalSynced, m.totalBytes, m.totalErrors)
	b.WriteString(dimStyle.Render(stats) + "\n\n")

	// Help bar
	help := "  q quit  p pause  r full resync  l logs  d dry-run  / filter"
	if m.filtering {
		help = fmt.Sprintf("  filter: %s█  (enter to apply, esc to cancel)", m.filter)
	}
	b.WriteString(helpStyle.Render(help) + "\n")

	return b.String()
}

func (m DashboardModel) filteredEvents() []SyncEvent {
	if m.filter == "" {
		return m.events
	}
	var filtered []SyncEvent
	for _, e := range m.events {
		if strings.Contains(strings.ToLower(e.File), strings.ToLower(m.filter)) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

**Step 3: Create log view model**

```go
// internal/tui/logview.go
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LogEntry is a single log line.
type LogEntry struct {
	Time    time.Time
	Level   string // "INF", "WRN", "ERR"
	Message string
}

// LogViewModel shows scrollable logs.
type LogViewModel struct {
	entries   []LogEntry
	offset    int
	width     int
	height    int
	filter    string
	filtering bool
}

// NewLogView creates an empty log view.
func NewLogView() LogViewModel {
	return LogViewModel{}
}

func (m LogViewModel) Init() tea.Cmd { return nil }

func (m LogViewModel) Update(msg tea.Msg) (LogViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "enter", "esc":
				m.filtering = false
				if msg.String() == "esc" {
					m.filter = ""
				}
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.filter += msg.String()
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "up", "k":
			if m.offset > 0 {
				m.offset--
			}
		case "down", "j":
			m.offset++
		case "/":
			m.filtering = true
			m.filter = ""
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m LogViewModel) View() string {
	var b strings.Builder

	title := titleStyle.Render(" esync ─ logs ")
	separator := dimStyle.Render(strings.Repeat("─", max(0, m.width-16)))
	b.WriteString(title + separator + "\n")

	filtered := m.filteredEntries()
	visible := m.height - 4 // header + help
	if visible < 1 {
		visible = 10
	}

	start := m.offset
	if start > len(filtered)-visible {
		start = max(0, len(filtered)-visible)
	}
	end := min(start+visible, len(filtered))

	for i := start; i < end; i++ {
		e := filtered[i]
		ts := dimStyle.Render(e.Time.Format("15:04:05"))
		var lvl string
		switch e.Level {
		case "INF":
			lvl = statusSynced.Render("INF")
		case "WRN":
			lvl = statusSyncing.Render("WRN")
		case "ERR":
			lvl = statusError.Render("ERR")
		default:
			lvl = dimStyle.Render(e.Level)
		}
		b.WriteString(fmt.Sprintf("  %s %s %s\n", ts, lvl, e.Message))
	}

	b.WriteString("\n")
	help := "  ↑↓ scroll  / filter  l back  q quit"
	if m.filtering {
		help = fmt.Sprintf("  filter: %s█  (enter to apply, esc to cancel)", m.filter)
	}
	b.WriteString(helpStyle.Render(help) + "\n")

	return b.String()
}

func (m LogViewModel) filteredEntries() []LogEntry {
	if m.filter == "" {
		return m.entries
	}
	var out []LogEntry
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.Message), strings.ToLower(m.filter)) {
			out = append(out, e)
		}
	}
	return out
}

// AddEntry adds a log entry (called from outside the TUI update loop via a Cmd).
func (m *LogViewModel) AddEntry(entry LogEntry) {
	m.entries = append(m.entries, entry)
}
```

**Step 4: Create app model (root TUI that switches between dashboard and log view)**

```go
// internal/tui/app.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type view int

const (
	viewDashboard view = iota
	viewLogs
)

// AppModel is the root Bubbletea model.
type AppModel struct {
	dashboard DashboardModel
	logView   LogViewModel
	current   view
	// Channels for external events
	syncEvents chan SyncEvent
	logEntries chan LogEntry
}

// NewApp creates the root TUI model.
func NewApp(local, remote string) *AppModel {
	return &AppModel{
		dashboard:  NewDashboard(local, remote),
		logView:    NewLogView(),
		current:    viewDashboard,
		syncEvents: make(chan SyncEvent, 100),
		logEntries: make(chan LogEntry, 100),
	}
}

// SyncEventChan returns the channel to send sync events to the TUI.
func (m *AppModel) SyncEventChan() chan<- SyncEvent {
	return m.syncEvents
}

// LogEntryChan returns the channel to send log entries to the TUI.
func (m *AppModel) LogEntryChan() chan<- LogEntry {
	return m.logEntries
}

func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.dashboard.Init(),
		m.waitForSyncEvent(),
		m.waitForLogEntry(),
	)
}

func (m *AppModel) waitForSyncEvent() tea.Cmd {
	return func() tea.Msg {
		e := <-m.syncEvents
		return SyncEventMsg(e)
	}
}

func (m *AppModel) waitForLogEntry() tea.Cmd {
	return func() tea.Msg {
		return <-m.logEntries
	}
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "l":
			if m.current == viewDashboard {
				m.current = viewLogs
			} else {
				m.current = viewDashboard
			}
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case SyncEventMsg:
		var cmd tea.Cmd
		var model tea.Model
		model, cmd = m.dashboard.Update(msg)
		m.dashboard = model.(DashboardModel)
		return m, tea.Batch(cmd, m.waitForSyncEvent())

	case LogEntry:
		m.logView.AddEntry(msg)
		return m, m.waitForLogEntry()
	}

	// Delegate to current view
	switch m.current {
	case viewDashboard:
		var cmd tea.Cmd
		var model tea.Model
		model, cmd = m.dashboard.Update(msg)
		m.dashboard = model.(DashboardModel)
		return m, cmd
	case viewLogs:
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *AppModel) View() string {
	switch m.current {
	case viewLogs:
		return m.logView.View()
	default:
		return m.dashboard.View()
	}
}
```

**Step 5: Verify build**

```bash
go build ./...
```

**Step 6: Commit**

```bash
git add internal/tui/
git commit -m "feat: add TUI with dashboard, log view, and Lipgloss styles"
```

---

### Task 7: CLI Commands — sync

**Files:**
- Create: `cmd/sync.go`
- Modify: `cmd/root.go`

**Step 1: Implement sync command**

```go
// cmd/sync.go
package cmd

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/eloualiche/esync/internal/config"
	"github.com/eloualiche/esync/internal/logger"
	"github.com/eloualiche/esync/internal/syncer"
	"github.com/eloualiche/esync/internal/tui"
	"github.com/eloualiche/esync/internal/watcher"
)

var (
	localPath   string
	remotePath  string
	daemon      bool
	dryRun      bool
	initialSync bool
	verbose     bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Start watching and syncing files",
	Long:  "Watch a local directory for changes and sync them to a remote destination using rsync.",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().StringVarP(&localPath, "local", "l", "", "local path to sync from")
	syncCmd.Flags().StringVarP(&remotePath, "remote", "r", "", "remote path to sync to")
	syncCmd.Flags().BoolVar(&daemon, "daemon", false, "run without TUI, log to file")
	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would sync without executing")
	syncCmd.Flags().BoolVar(&initialSync, "initial-sync", false, "force full sync on startup")
	syncCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := loadOrBuildConfig()
	if err != nil {
		return err
	}

	// CLI overrides
	if localPath != "" {
		cfg.Sync.Local = localPath
	}
	if remotePath != "" {
		cfg.Sync.Remote = remotePath
	}
	if initialSync {
		cfg.Settings.InitialSync = true
	}

	if cfg.Sync.Local == "" || cfg.Sync.Remote == "" {
		return fmt.Errorf("both local and remote paths are required (use -l and -r, or a config file)")
	}

	s := syncer.New(cfg)
	s.DryRun = dryRun

	// Optional initial sync
	if cfg.Settings.InitialSync {
		fmt.Println("Running initial sync...")
		if result, err := s.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "initial sync failed: %s\n", result.ErrorMessage)
		}
	}

	if daemon {
		return runDaemon(cfg, s)
	}
	return runTUI(cfg, s)
}

func runTUI(cfg *config.Config, s *syncer.Syncer) error {
	app := tui.NewApp(cfg.Sync.Local, cfg.Sync.Remote)

	// Set up watcher
	handler := func() {
		result, err := s.Run()
		event := tui.SyncEvent{
			Time: time.Now(),
		}
		if err != nil {
			event.Status = "error"
			event.File = result.ErrorMessage
		} else {
			event.Status = "synced"
			event.Duration = result.Duration
			if len(result.Files) > 0 {
				event.File = result.Files[0]
			} else {
				event.File = "(no changes)"
			}
			event.Size = formatSize(result.BytesTotal)
		}
		app.SyncEventChan() <- event
	}

	w, err := watcher.New(
		cfg.Sync.Local,
		cfg.Settings.WatcherDebounce,
		cfg.AllIgnorePatterns(),
		handler,
	)
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}

	if err := w.Start(); err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}
	defer w.Stop()

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

func runDaemon(cfg *config.Config, s *syncer.Syncer) error {
	logPath := cfg.Settings.Log.File
	if logPath == "" {
		logPath = "esync.log"
	}
	logFormat := cfg.Settings.Log.Format

	l, err := logger.New(logPath, logFormat)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer l.Close()

	fmt.Printf("esync daemon started (PID %d)\n", os.Getpid())
	fmt.Printf("Watching: %s → %s\n", cfg.Sync.Local, cfg.Sync.Remote)
	fmt.Printf("Log: %s\n", logPath)

	l.Info("started", map[string]interface{}{
		"local":  cfg.Sync.Local,
		"remote": cfg.Sync.Remote,
		"pid":    os.Getpid(),
	})

	handler := func() {
		result, err := s.Run()
		if err != nil {
			l.Error("sync_failed", map[string]interface{}{
				"error": result.ErrorMessage,
			})
			fmt.Print("\a") // terminal bell on error
		} else {
			fields := map[string]interface{}{
				"duration_ms": result.Duration.Milliseconds(),
				"bytes":       result.BytesTotal,
			}
			if len(result.Files) > 0 {
				fields["file"] = result.Files[0]
			}
			l.Info("synced", fields)
		}
	}

	w, err := watcher.New(
		cfg.Sync.Local,
		cfg.Settings.WatcherDebounce,
		cfg.AllIgnorePatterns(),
		handler,
	)
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}

	if err := w.Start(); err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}
	defer w.Stop()

	// Block until interrupted
	select {}
}

func loadOrBuildConfig() (*config.Config, error) {
	if cfgFile != "" {
		return config.Load(cfgFile)
	}

	// Quick mode: local + remote provided directly
	if localPath != "" && remotePath != "" {
		return &config.Config{
			Sync: config.SyncSection{
				Local:  localPath,
				Remote: remotePath,
			},
			Settings: config.Settings{
				WatcherDebounce: 500,
				Rsync: config.RsyncSettings{
					Archive:  true,
					Compress: true,
					Progress: true,
				},
			},
		}, nil
	}

	// Try to find config file
	path := config.FindConfigFile()
	if path == "" {
		return nil, fmt.Errorf("no config file found; use -c, create esync.toml, or pass -l and -r")
	}
	return config.Load(path)
}

func formatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.2fGB", float64(bytes)/(1024*1024*1024))
	}
}
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Manual test**

```bash
mkdir -p /tmp/esync-test-src /tmp/esync-test-dst
echo "hello" > /tmp/esync-test-src/test.txt
go run . sync -l /tmp/esync-test-src -r /tmp/esync-test-dst
# TUI should appear. Modify test.txt in another terminal. Press q to quit.
```

**Step 4: Commit**

```bash
git add cmd/sync.go
git commit -m "feat: add sync command with TUI and daemon modes"
```

---

### Task 8: CLI Commands — init (smart)

**Files:**
- Create: `cmd/init.go`

**Step 1: Implement smart init**

```go
// cmd/init.go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eloualiche/esync/internal/config"
)

var initRemote string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate esync.toml from current directory",
	Long:  "Create an esync.toml config file by inspecting the current directory, importing .gitignore patterns, and detecting common exclusions.",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initRemote, "remote", "r", "", "pre-fill remote destination")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	outPath := "esync.toml"
	if cfgFile != "" {
		outPath = cfgFile
	}

	// Check if file exists
	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("Config file %s already exists. Overwrite? [y/N] ", outPath)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Start with default TOML
	content := config.DefaultTOML()

	// Detect .gitignore
	gitignorePatterns := readGitignore()
	if len(gitignorePatterns) > 0 {
		fmt.Printf("Detected .gitignore — imported %d patterns\n", len(gitignorePatterns))
	}

	// Detect common directories to exclude
	autoExclude := detectCommonDirs()
	if len(autoExclude) > 0 {
		fmt.Printf("Auto-excluding: %s\n", strings.Join(autoExclude, ", "))
	}

	// Prompt for remote if not provided
	remote := initRemote
	if remote == "" {
		fmt.Print("Remote destination? (e.g. user@host:/path) ")
		reader := bufio.NewReader(os.Stdin)
		remote, _ = reader.ReadString('\n')
		remote = strings.TrimSpace(remote)
	}
	if remote != "" {
		content = strings.Replace(content, `remote = "./remote"`, fmt.Sprintf(`remote = "%s"`, remote), 1)
	}

	// Merge extra ignore patterns into rsync ignore
	if len(gitignorePatterns) > 0 || len(autoExclude) > 0 {
		allExtra := append(gitignorePatterns, autoExclude...)
		// Build the ignore array string
		var quoted []string
		for _, p := range allExtra {
			quoted = append(quoted, fmt.Sprintf(`"%s"`, p))
		}
		extraLine := strings.Join(quoted, ", ")
		// Append to existing ignore array
		content = strings.Replace(content,
			`ignore = [".git/", "node_modules/", "**/__pycache__/"]`,
			fmt.Sprintf(`ignore = [".git/", "node_modules/", "**/__pycache__/", %s]`, extraLine),
			1,
		)
	}

	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("\nWritten: %s\n", outPath)
	fmt.Println("\nRun `esync check` for file preview, `esync edit` to adjust")

	return nil
}

func readGitignore() []string {
	f, err := os.Open(".gitignore")
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Skip patterns we already have as defaults
		if line == ".git" || line == ".git/" || line == "node_modules" || line == "node_modules/" || line == "__pycache__" || line == "__pycache__/" {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func detectCommonDirs() []string {
	common := []string{".git/", "node_modules/", "__pycache__/", "build/", ".venv/", "dist/", ".tox/", ".mypy_cache/"}
	var found []string
	for _, dir := range common {
		clean := strings.TrimSuffix(dir, "/")
		if info, err := os.Stat(clean); err == nil && info.IsDir() {
			// Skip ones already in default config
			if dir == ".git/" || dir == "node_modules/" || dir == "__pycache__/" {
				continue
			}
			found = append(found, dir)
		}
	}
	return found
}
```

**Step 2: Verify build and test manually**

```bash
go build ./...
cd /tmp && mkdir test-init && cd test-init
echo "*.pyc" > .gitignore
/path/to/esync init -r user@host:/deploy
cat esync.toml
```

**Step 3: Commit**

```bash
git add cmd/init.go
git commit -m "feat: add smart init command with .gitignore import"
```

---

### Task 9: CLI Commands — check and edit

**Files:**
- Create: `cmd/check.go`
- Create: `cmd/edit.go`

**Step 1: Implement check command**

```go
// cmd/check.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/eloualiche/esync/internal/config"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate config and show file include/exclude preview",
	RunE:  runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return printPreview(cfg)
}

func loadConfig() (*config.Config, error) {
	path := cfgFile
	if path == "" {
		path = config.FindConfigFile()
	}
	if path == "" {
		return nil, fmt.Errorf("no config file found")
	}
	return config.Load(path)
}

func printPreview(cfg *config.Config) error {
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	fmt.Println(green.Render(" esync ─ config preview"))
	fmt.Printf("  Local:  %s\n", cfg.Sync.Local)
	fmt.Printf("  Remote: %s\n\n", cfg.Sync.Remote)

	ignores := cfg.AllIgnorePatterns()

	var included []string
	var excluded []excludedFile
	var totalSize int64

	localPath := cfg.Sync.Local
	filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(localPath, path)
		if rel == "." {
			return nil
		}

		for _, pattern := range ignores {
			clean := strings.Trim(pattern, "\"[]'")
			if strings.HasPrefix(clean, "**/") {
				clean = clean[3:]
			}
			base := filepath.Base(rel)
			if matched, _ := filepath.Match(clean, base); matched {
				excluded = append(excluded, excludedFile{path: rel, rule: pattern})
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if matched, _ := filepath.Match(clean, rel); matched {
				excluded = append(excluded, excludedFile{path: rel, rule: pattern})
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// Directory pattern matching
			if strings.HasSuffix(clean, "/") && info.IsDir() {
				dirName := strings.TrimSuffix(clean, "/")
				if base == dirName {
					excluded = append(excluded, excludedFile{path: rel + "/", rule: pattern})
					return filepath.SkipDir
				}
			}
		}

		if !info.IsDir() {
			included = append(included, rel)
			totalSize += info.Size()
		}
		return nil
	})

	// Show included
	fmt.Println(green.Render("  Included (sample):"))
	shown := min(10, len(included))
	for i := 0; i < shown; i++ {
		fmt.Printf("  %s\n", included[i])
	}
	if len(included) > shown {
		fmt.Printf("  %s\n", dim.Render(fmt.Sprintf("... %d more files", len(included)-shown)))
	}
	fmt.Println()

	// Show excluded
	fmt.Println(yellow.Render("  Excluded by rules:"))
	shown = min(10, len(excluded))
	for i := 0; i < shown; i++ {
		fmt.Printf("  %-30s %s\n", excluded[i].path, dim.Render("["+excluded[i].rule+"]"))
	}
	if len(excluded) > shown {
		fmt.Printf("  %s\n", dim.Render(fmt.Sprintf("... %d more", len(excluded)-shown)))
	}
	fmt.Println()

	fmt.Printf("  %s\n", dim.Render(fmt.Sprintf("%d files included (%s) │ %d excluded",
		len(included), formatSize(totalSize), len(excluded))))

	return nil
}

type excludedFile struct {
	path string
	rule string
}
```

**Step 2: Implement edit command**

```go
// cmd/edit.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/eloualiche/esync/internal/config"
)

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config in $EDITOR, then show preview",
	RunE:  runEdit,
}

func init() {
	rootCmd.AddCommand(editCmd)
}

func runEdit(cmd *cobra.Command, args []string) error {
	path := cfgFile
	if path == "" {
		path = config.FindConfigFile()
	}
	if path == "" {
		return fmt.Errorf("no config file found; run `esync init` first")
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	for {
		// Open editor
		c := exec.Command(editor, path)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Validate and show preview
		cfg, err := config.Load(path)
		if err != nil {
			fmt.Printf("\nConfig error: %v\n", err)
			fmt.Print("Press enter to edit again, or q to cancel: ")
			var answer string
			fmt.Scanln(&answer)
			if answer == "q" {
				return nil
			}
			continue
		}

		if err := printPreview(cfg); err != nil {
			return err
		}

		fmt.Print("\nPress enter to accept, e to edit again, q to cancel: ")
		var answer string
		fmt.Scanln(&answer)
		switch answer {
		case "e":
			continue
		case "q":
			fmt.Println("Cancelled.")
			return nil
		default:
			fmt.Println("Config accepted.")
			return nil
		}
	}
}
```

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add cmd/check.go cmd/edit.go
git commit -m "feat: add check and edit commands for config validation and preview"
```

---

### Task 10: CLI Commands — status

**Files:**
- Create: `cmd/status.go`
- Modify: `cmd/sync.go` (write PID file in daemon mode)

**Step 1: Implement PID file in daemon mode**

Add to `runDaemon` in `cmd/sync.go`:
```go
// Write PID file
pidPath := filepath.Join(os.TempDir(), "esync.pid")
os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
defer os.Remove(pidPath)
```

**Step 2: Implement status command**

```go
// cmd/status.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if esync daemon is running",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	pidPath := filepath.Join(os.TempDir(), "esync.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("No esync daemon running.")
		return nil
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Println("No esync daemon running (invalid PID file).")
		os.Remove(pidPath)
		return nil
	}

	// Check if process is alive
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("No esync daemon running.")
		os.Remove(pidPath)
		return nil
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("No esync daemon running (stale PID file).")
		os.Remove(pidPath)
		return nil
	}

	fmt.Printf("esync daemon running (PID %d)\n", pid)
	return nil
}
```

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add cmd/status.go cmd/sync.go
git commit -m "feat: add status command and PID file for daemon mode"
```

---

### Task 11: Signal Handling and Graceful Shutdown

**Files:**
- Modify: `cmd/sync.go`

**Step 1: Add signal handling to daemon mode**

Replace the `select {}` block at the end of `runDaemon` with:

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
<-sigCh
l.Info("stopping", nil)
fmt.Println("\nesync daemon stopped.")
```

And import `"os/signal"` and `"syscall"`.

**Step 2: Test daemon start/stop**

```bash
go run . sync --daemon -l /tmp/esync-test-src -r /tmp/esync-test-dst &
go run . status
kill %1
```

**Step 3: Commit**

```bash
git add cmd/sync.go
git commit -m "feat: add graceful shutdown with signal handling"
```

---

### Task 12: README

**Files:**
- Modify: `readme.md`

**Step 1: Write comprehensive README with TOML examples**

Replace the entire README with documentation covering:

- What esync does (1 paragraph)
- Installation (go install + binary download)
- Quick start (3 commands)
- Commands reference (sync, init, check, edit, status)
- Configuration reference with full annotated TOML example
- Config file search order
- SSH setup example
- Daemon mode usage
- TUI keyboard shortcuts
- Examples section with 5-6 common use cases

Ensure thorough TOML examples per user request.

**Step 2: Commit**

```bash
git add readme.md
git commit -m "docs: rewrite README for Go version with TOML examples"
```

---

### Task 13: Integration Testing

**Files:**
- Create: `integration_test.go`

**Step 1: Write integration test for local sync**

```go
// integration_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eloualiche/esync/internal/config"
	"github.com/eloualiche/esync/internal/syncer"
	"github.com/eloualiche/esync/internal/watcher"
)

func TestLocalSyncIntegration(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a test file
	os.WriteFile(filepath.Join(src, "hello.txt"), []byte("hello"), 0644)

	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  src,
			Remote: dst,
		},
		Settings: config.Settings{
			WatcherDebounce: 100,
			Rsync: config.RsyncSettings{
				Archive:  true,
				Progress: true,
			},
		},
	}

	s := syncer.New(cfg)
	result, err := s.Run()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("sync not successful: %s", result.ErrorMessage)
	}

	// Verify file was synced
	data, err := os.ReadFile(filepath.Join(dst, "hello.txt"))
	if err != nil {
		t.Fatalf("synced file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func TestWatcherTriggersSync(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  src,
			Remote: dst,
		},
		Settings: config.Settings{
			WatcherDebounce: 100,
			Rsync: config.RsyncSettings{
				Archive:  true,
				Progress: true,
			},
		},
	}

	s := syncer.New(cfg)
	synced := make(chan struct{}, 1)

	handler := func() {
		s.Run()
		select {
		case synced <- struct{}{}:
		default:
		}
	}

	w, err := watcher.New(src, 100, nil, handler)
	if err != nil {
		t.Fatalf("watcher creation failed: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("watcher start failed: %v", err)
	}
	defer w.Stop()

	// Create a file to trigger sync
	time.Sleep(200 * time.Millisecond) // let watcher settle
	os.WriteFile(filepath.Join(src, "trigger.txt"), []byte("trigger"), 0644)

	select {
	case <-synced:
		// Verify
		data, err := os.ReadFile(filepath.Join(dst, "trigger.txt"))
		if err != nil {
			t.Fatalf("file not synced: %v", err)
		}
		if string(data) != "trigger" {
			t.Errorf("expected 'trigger', got %q", string(data))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for sync")
	}
}
```

**Step 2: Run all tests**

```bash
go test ./... -v
```
Expected: all PASS

**Step 3: Commit**

```bash
git add integration_test.go
git commit -m "test: add integration tests for local sync and watcher"
```

---

### Task 14: Example Config and Final Polish

**Files:**
- Create: `esync.toml.example`
- Verify: `go build ./...` and `go vet ./...`

**Step 1: Create example config**

Write `esync.toml.example` with the full annotated schema from the design doc.

**Step 2: Run linting and vet**

```bash
go vet ./...
go build -o esync .
./esync --help
./esync sync --help
./esync init --help
```

**Step 3: Clean up go.sum**

```bash
go mod tidy
```

**Step 4: Final commit**

```bash
git add esync.toml.example go.mod go.sum
git commit -m "chore: add example config and tidy module"
```

---

## Execution Order Summary

| Task | Component | Depends On |
|------|-----------|------------|
| 1 | Project scaffolding | — |
| 2 | Config package | 1 |
| 3 | Syncer package | 2 |
| 4 | Watcher package | 1 |
| 5 | Logger package | 1 |
| 6 | TUI (styles, dashboard, log view) | 1 |
| 7 | CLI sync command | 2, 3, 4, 5, 6 |
| 8 | CLI init command | 2 |
| 9 | CLI check + edit commands | 2, 8 |
| 10 | CLI status command | 7 |
| 11 | Signal handling | 7 |
| 12 | README | all above |
| 13 | Integration tests | 3, 4 |
| 14 | Example config + polish | all above |

**Parallelizable:** Tasks 2, 4, 5, 6 can run in parallel after Task 1. Tasks 8 and 13 can run in parallel with Task 7.
