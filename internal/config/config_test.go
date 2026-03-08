package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Helper: write a TOML string to a temp file and return its path ---
func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "esync.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp TOML: %v", err)
	}
	return path
}

// -----------------------------------------------------------------------
// 1. TestLoadConfig — full TOML with all fields
// -----------------------------------------------------------------------
func TestLoadConfig(t *testing.T) {
	toml := `
[sync]
local  = "/home/user/project"
remote = "server:/data/project"
interval = 5

[settings]
watcher_debounce = 300
initial_sync     = true
ignore           = [".git", "node_modules"]

[settings.rsync]
archive    = true
compress   = false
backup     = true
backup_dir = ".my_backup"
progress   = false
extra_args = ["--delete", "--verbose"]
ignore     = ["*.tmp", "*.log"]

[settings.log]
file   = "/var/log/esync.log"
format = "json"
`
	path := writeTempTOML(t, toml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// sync section
	if cfg.Sync.Local != "/home/user/project" {
		t.Errorf("Sync.Local = %q, want %q", cfg.Sync.Local, "/home/user/project")
	}
	if cfg.Sync.Remote != "server:/data/project" {
		t.Errorf("Sync.Remote = %q, want %q", cfg.Sync.Remote, "server:/data/project")
	}
	if cfg.Sync.Interval != 5 {
		t.Errorf("Sync.Interval = %d, want 5", cfg.Sync.Interval)
	}

	// settings
	if cfg.Settings.WatcherDebounce != 300 {
		t.Errorf("Settings.WatcherDebounce = %d, want 300", cfg.Settings.WatcherDebounce)
	}
	if cfg.Settings.InitialSync != true {
		t.Errorf("Settings.InitialSync = %v, want true", cfg.Settings.InitialSync)
	}
	if len(cfg.Settings.Ignore) != 2 || cfg.Settings.Ignore[0] != ".git" || cfg.Settings.Ignore[1] != "node_modules" {
		t.Errorf("Settings.Ignore = %v, want [.git node_modules]", cfg.Settings.Ignore)
	}

	// rsync
	if cfg.Settings.Rsync.Archive != true {
		t.Errorf("Rsync.Archive = %v, want true", cfg.Settings.Rsync.Archive)
	}
	if cfg.Settings.Rsync.Compress != false {
		t.Errorf("Rsync.Compress = %v, want false", cfg.Settings.Rsync.Compress)
	}
	if cfg.Settings.Rsync.Backup != true {
		t.Errorf("Rsync.Backup = %v, want true", cfg.Settings.Rsync.Backup)
	}
	if cfg.Settings.Rsync.BackupDir != ".my_backup" {
		t.Errorf("Rsync.BackupDir = %q, want %q", cfg.Settings.Rsync.BackupDir, ".my_backup")
	}
	if cfg.Settings.Rsync.Progress != false {
		t.Errorf("Rsync.Progress = %v, want false", cfg.Settings.Rsync.Progress)
	}
	if len(cfg.Settings.Rsync.ExtraArgs) != 2 || cfg.Settings.Rsync.ExtraArgs[0] != "--delete" {
		t.Errorf("Rsync.ExtraArgs = %v, want [--delete --verbose]", cfg.Settings.Rsync.ExtraArgs)
	}
	if len(cfg.Settings.Rsync.Ignore) != 2 || cfg.Settings.Rsync.Ignore[0] != "*.tmp" {
		t.Errorf("Rsync.Ignore = %v, want [*.tmp *.log]", cfg.Settings.Rsync.Ignore)
	}

	// log
	if cfg.Settings.Log.File != "/var/log/esync.log" {
		t.Errorf("Log.File = %q, want %q", cfg.Settings.Log.File, "/var/log/esync.log")
	}
	if cfg.Settings.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Settings.Log.Format, "json")
	}
}

// -----------------------------------------------------------------------
// 2. TestLoadConfigWithSSH — TOML with [sync.ssh] section
// -----------------------------------------------------------------------
func TestLoadConfigWithSSH(t *testing.T) {
	toml := `
[sync]
local  = "/home/user/src"
remote = "/data/dest"

[sync.ssh]
host             = "myserver.com"
user             = "deploy"
port             = 2222
identity_file    = "~/.ssh/id_ed25519"
interactive_auth = true
`
	path := writeTempTOML(t, toml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Sync.SSH == nil {
		t.Fatal("Sync.SSH is nil, expected SSH config")
	}
	if cfg.Sync.SSH.Host != "myserver.com" {
		t.Errorf("SSH.Host = %q, want %q", cfg.Sync.SSH.Host, "myserver.com")
	}
	if cfg.Sync.SSH.User != "deploy" {
		t.Errorf("SSH.User = %q, want %q", cfg.Sync.SSH.User, "deploy")
	}
	if cfg.Sync.SSH.Port != 2222 {
		t.Errorf("SSH.Port = %d, want 2222", cfg.Sync.SSH.Port)
	}
	if cfg.Sync.SSH.IdentityFile != "~/.ssh/id_ed25519" {
		t.Errorf("SSH.IdentityFile = %q, want %q", cfg.Sync.SSH.IdentityFile, "~/.ssh/id_ed25519")
	}
	if cfg.Sync.SSH.InteractiveAuth != true {
		t.Errorf("SSH.InteractiveAuth = %v, want true", cfg.Sync.SSH.InteractiveAuth)
	}

	// IsRemote should return true when SSH is configured
	if !cfg.IsRemote() {
		t.Error("IsRemote() = false, want true (SSH config present)")
	}
}

// -----------------------------------------------------------------------
// 3. TestLoadConfigDefaults — minimal TOML, verify defaults applied
// -----------------------------------------------------------------------
func TestLoadConfigDefaults(t *testing.T) {
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

	if cfg.Sync.Interval != 1 {
		t.Errorf("default Sync.Interval = %d, want 1", cfg.Sync.Interval)
	}
	if cfg.Settings.WatcherDebounce != 500 {
		t.Errorf("default WatcherDebounce = %d, want 500", cfg.Settings.WatcherDebounce)
	}
	if cfg.Settings.InitialSync != false {
		t.Errorf("default InitialSync = %v, want false", cfg.Settings.InitialSync)
	}
	if cfg.Settings.Rsync.Archive != true {
		t.Errorf("default Rsync.Archive = %v, want true", cfg.Settings.Rsync.Archive)
	}
	if cfg.Settings.Rsync.Compress != true {
		t.Errorf("default Rsync.Compress = %v, want true", cfg.Settings.Rsync.Compress)
	}
	if cfg.Settings.Rsync.Backup != false {
		t.Errorf("default Rsync.Backup = %v, want false", cfg.Settings.Rsync.Backup)
	}
	if cfg.Settings.Rsync.BackupDir != ".rsync_backup" {
		t.Errorf("default Rsync.BackupDir = %q, want %q", cfg.Settings.Rsync.BackupDir, ".rsync_backup")
	}
	if cfg.Settings.Rsync.Progress != true {
		t.Errorf("default Rsync.Progress = %v, want true", cfg.Settings.Rsync.Progress)
	}
	if cfg.Settings.Log.Format != "text" {
		t.Errorf("default Log.Format = %q, want %q", cfg.Settings.Log.Format, "text")
	}

	// SSH should be nil when not specified
	if cfg.Sync.SSH != nil {
		t.Errorf("Sync.SSH = %v, want nil", cfg.Sync.SSH)
	}
}

// -----------------------------------------------------------------------
// 4. TestLoadConfigValidation — missing required fields
// -----------------------------------------------------------------------
func TestLoadConfigValidation(t *testing.T) {
	t.Run("missing local", func(t *testing.T) {
		toml := `
[sync]
remote = "/dst"
`
		path := writeTempTOML(t, toml)
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for missing local, got nil")
		}
	})

	t.Run("missing remote", func(t *testing.T) {
		toml := `
[sync]
local = "/src"
`
		path := writeTempTOML(t, toml)
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for missing remote, got nil")
		}
	})

	t.Run("missing both", func(t *testing.T) {
		toml := `
[settings]
watcher_debounce = 100
`
		path := writeTempTOML(t, toml)
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for missing local and remote, got nil")
		}
	})
}

// -----------------------------------------------------------------------
// 5. TestIsRemote — various remote patterns
// -----------------------------------------------------------------------
func TestIsRemote(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		ssh    *SSHConfig
		want   bool
	}{
		{"user@host:/path", "user@host:/path", nil, true},
		{"host:/path", "host:/path", nil, true},
		{"local relative", "./local", nil, false},
		{"local absolute", "/absolute", nil, false},
		{"windows path", "C:/windows", nil, false},
		{"ssh config present", "/data/dest", &SSHConfig{Host: "myserver"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Sync: SyncSection{
					Remote: tt.remote,
					SSH:    tt.ssh,
				},
			}
			got := cfg.IsRemote()
			if got != tt.want {
				t.Errorf("IsRemote() = %v, want %v (remote=%q, ssh=%v)", got, tt.want, tt.remote, tt.ssh)
			}
		})
	}
}

// -----------------------------------------------------------------------
// 6. TestFindConfigFile / TestFindConfigFileNotFound
// -----------------------------------------------------------------------
func TestFindConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "esync.toml")
	if err := os.WriteFile(configPath, []byte("[sync]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	found := FindConfigIn([]string{
		filepath.Join(dir, "nonexistent.toml"),
		configPath,
		"/also/nonexistent.toml",
	})
	if found != configPath {
		t.Errorf("FindConfigIn = %q, want %q", found, configPath)
	}
}

func TestFindConfigFileNotFound(t *testing.T) {
	found := FindConfigIn([]string{
		"/does/not/exist/esync.toml",
		"/also/nonexistent/config.toml",
	})
	if found != "" {
		t.Errorf("FindConfigIn = %q, want empty string", found)
	}
}

// -----------------------------------------------------------------------
// 7. TestAllIgnorePatterns — combines both ignore lists
// -----------------------------------------------------------------------
func TestAllIgnorePatterns(t *testing.T) {
	cfg := &Config{
		Settings: Settings{
			Ignore: []string{".git", "node_modules"},
			Rsync: RsyncSettings{
				Ignore: []string{"*.tmp", "*.log"},
			},
		},
	}

	patterns := cfg.AllIgnorePatterns()
	expected := []string{".git", "node_modules", "*.tmp", "*.log"}

	if len(patterns) != len(expected) {
		t.Fatalf("AllIgnorePatterns length = %d, want %d", len(patterns), len(expected))
	}
	for i, p := range patterns {
		if p != expected[i] {
			t.Errorf("AllIgnorePatterns[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

func TestAllIgnorePatternsEmpty(t *testing.T) {
	cfg := &Config{}
	patterns := cfg.AllIgnorePatterns()
	if len(patterns) != 0 {
		t.Errorf("AllIgnorePatterns = %v, want empty", patterns)
	}
}

// -----------------------------------------------------------------------
// 8. TestLoadConfigWithInclude — include field parsed correctly
// -----------------------------------------------------------------------
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

// -----------------------------------------------------------------------
// 9. TestLoadConfigIncludeDefaultsToEmpty — omitted include is nil/empty
// -----------------------------------------------------------------------
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

// -----------------------------------------------------------------------
// 10. TestDefaultTOML — returns a non-empty template
// -----------------------------------------------------------------------
func TestDefaultTOML(t *testing.T) {
	toml := DefaultTOML()
	if toml == "" {
		t.Error("DefaultTOML() returned empty string")
	}
	// Should contain key sections
	for _, section := range []string{"[sync]", "[settings]", "[settings.rsync]", "[settings.log]"} {
		if !containsString(toml, section) {
			t.Errorf("DefaultTOML() missing section %q", section)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
