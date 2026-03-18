// Package config handles loading, validating, and providing defaults for
// esync TOML configuration files.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// Structs
// ---------------------------------------------------------------------------

// SSHConfig holds SSH connection parameters for remote syncing.
type SSHConfig struct {
	Host            string `mapstructure:"host"`
	User            string `mapstructure:"user"`
	Port            int    `mapstructure:"port"`
	IdentityFile    string `mapstructure:"identity_file"`
	InteractiveAuth bool   `mapstructure:"interactive_auth"`
}

// SyncSection defines the local/remote pair and optional SSH tunnel.
type SyncSection struct {
	Local    string     `mapstructure:"local"`
	Remote   string     `mapstructure:"remote"`
	Interval int        `mapstructure:"interval"`
	SSH      *SSHConfig `mapstructure:"ssh"`
}

// RsyncSettings controls rsync behaviour.
type RsyncSettings struct {
	Archive   bool     `mapstructure:"archive"`
	Compress  bool     `mapstructure:"compress"`
	Delete    bool     `mapstructure:"delete"`
	CopyLinks bool     `mapstructure:"copy_links"`
	Backup    bool     `mapstructure:"backup"`
	BackupDir string   `mapstructure:"backup_dir"`
	Progress  bool     `mapstructure:"progress"`
	ExtraArgs []string `mapstructure:"extra_args"`
	Ignore    []string `mapstructure:"ignore"`
}

// LogSettings controls logging output.
type LogSettings struct {
	File   string `mapstructure:"file"`
	Format string `mapstructure:"format"`
}

// Settings groups watcher, rsync, and log tunables.
type Settings struct {
	WatcherDebounce int           `mapstructure:"watcher_debounce"`
	InitialSync     bool          `mapstructure:"initial_sync"`
	Include         []string      `mapstructure:"include"`
	Ignore          []string      `mapstructure:"ignore"`
	Rsync           RsyncSettings `mapstructure:"rsync"`
	Log             LogSettings   `mapstructure:"log"`
}

// Config is the top-level configuration.
type Config struct {
	Sync     SyncSection `mapstructure:"sync"`
	Settings Settings    `mapstructure:"settings"`
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

// Load reads a TOML configuration file at path, applies defaults, validates
// required fields, and returns the populated Config.
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
	v.SetDefault("settings.rsync.delete", false)
	v.SetDefault("settings.rsync.copy_links", false)
	v.SetDefault("settings.rsync.backup", false)
	v.SetDefault("settings.rsync.backup_dir", ".rsync_backup")
	v.SetDefault("settings.rsync.progress", true)
	v.SetDefault("settings.log.format", "text")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Validation: local and remote are required.
	if strings.TrimSpace(cfg.Sync.Local) == "" {
		return nil, fmt.Errorf("sync.local is required")
	}
	if strings.TrimSpace(cfg.Sync.Remote) == "" {
		return nil, fmt.Errorf("sync.remote is required")
	}

	return &cfg, nil
}

// ---------------------------------------------------------------------------
// Config file search
// ---------------------------------------------------------------------------

// FindConfigFile searches the standard locations for an esync config file
// and returns the first one found, or an empty string.
func FindConfigFile() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"./.esync.toml",
		home + "/.config/esync/config.toml",
		"/etc/esync/config.toml",
	}
	return FindConfigIn(candidates)
}

// FindConfigIn returns the first path in the list that exists on disk,
// or an empty string if none exist.
func FindConfigIn(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// IsRemote returns true if the configuration targets a remote destination,
// either via an explicit SSH section or a remote string that looks like
// "user@host:/path" or "host:/path".
func (c *Config) IsRemote() bool {
	if c.Sync.SSH != nil && c.Sync.SSH.Host != "" {
		return true
	}
	return looksRemote(c.Sync.Remote)
}

// looksRemote returns true if remote resembles an scp-style address
// (e.g. "user@host:/path" or "host:/path") but not a Windows drive
// letter like "C:/".
func looksRemote(remote string) bool {
	idx := strings.Index(remote, ":")
	if idx < 0 {
		return false
	}
	// Single letter before colon is a Windows drive letter (e.g. "C:/")
	if idx == 1 {
		return false
	}
	return true
}

// AllIgnorePatterns returns the combined ignore list from both
// settings.ignore and settings.rsync.ignore, in that order.
func (c *Config) AllIgnorePatterns() []string {
	combined := make([]string, 0, len(c.Settings.Ignore)+len(c.Settings.Rsync.Ignore))
	combined = append(combined, c.Settings.Ignore...)
	combined = append(combined, c.Settings.Rsync.Ignore...)
	return combined
}

// ---------------------------------------------------------------------------
// DefaultTOML
// ---------------------------------------------------------------------------

// DefaultTOML returns a commented TOML template suitable for writing to a
// new configuration file.
func DefaultTOML() string {
	return `# esync configuration file

[sync]
local  = "."
remote = "user@host:/path/to/dest"
interval = 1

# [sync.ssh]
# host             = "myserver.com"
# user             = "deploy"
# port             = 22
# identity_file    = "~/.ssh/id_ed25519"
# interactive_auth = false

[settings]
watcher_debounce = 500
initial_sync     = false
# include: path prefixes to sync (relative to local). Empty means everything.
# Keep include simple and explicit; use ignore for fine-grained filtering.
include          = []
ignore           = [".git", "node_modules", ".DS_Store"]

[settings.rsync]
archive    = true
compress   = true
delete     = false
copy_links = false
backup     = false
backup_dir = ".rsync_backup"
progress   = true
extra_args = []
ignore     = []

[settings.log]
# file   = "/var/log/esync.log"
format = "text"
`
}
