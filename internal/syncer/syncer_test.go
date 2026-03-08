package syncer

import (
	"strings"
	"testing"

	"github.com/louloulibs/esync/internal/config"
)

// ---------------------------------------------------------------------------
// Helper: build a minimal Config for testing
// ---------------------------------------------------------------------------
func minimalConfig(local, remote string) *config.Config {
	return &config.Config{
		Sync: config.SyncSection{
			Local:  local,
			Remote: remote,
		},
		Settings: config.Settings{
			Rsync: config.RsyncSettings{
				Archive:  true,
				Compress: true,
				Progress: true,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// 1. TestBuildCommand_Local — verify rsync flags for local sync
// ---------------------------------------------------------------------------
func TestBuildCommand_Local(t *testing.T) {
	cfg := minimalConfig("/home/user/src", "/data/dest")

	s := New(cfg)
	cmd := s.BuildCommand()

	// Should start with rsync (possibly absolute path)
	if !strings.HasSuffix(cmd[0], "rsync") {
		t.Errorf("cmd[0] = %q, want rsync binary", cmd[0])
	}

	// Must contain base flags
	for _, flag := range []string{"--recursive", "--times", "--progress", "--info=progress2", "--copy-unsafe-links"} {
		if !containsArg(cmd, flag) {
			t.Errorf("missing flag %q in %v", flag, cmd)
		}
	}

	// Archive and compress are true by default
	if !containsArg(cmd, "--archive") {
		t.Error("missing --archive flag")
	}
	if !containsArg(cmd, "--compress") {
		t.Error("missing --compress flag")
	}

	// Source must end with /
	source := cmd[len(cmd)-2]
	if !strings.HasSuffix(source, "/") {
		t.Errorf("source = %q, must end with /", source)
	}
	if source != "/home/user/src/" {
		t.Errorf("source = %q, want %q", source, "/home/user/src/")
	}

	// Destination is last argument
	dest := cmd[len(cmd)-1]
	if dest != "/data/dest" {
		t.Errorf("destination = %q, want %q", dest, "/data/dest")
	}

	// No -e flag for local sync without SSH config
	if containsArgPrefix(cmd, "-e") {
		t.Error("should not have -e flag for local sync")
	}
}

// ---------------------------------------------------------------------------
// 2. TestBuildCommand_Remote — verify remote destination format
// ---------------------------------------------------------------------------
func TestBuildCommand_Remote(t *testing.T) {
	cfg := minimalConfig("/home/user/src", "user@server:/data/dest")

	s := New(cfg)
	cmd := s.BuildCommand()

	// Destination should be the raw remote string
	dest := cmd[len(cmd)-1]
	if dest != "user@server:/data/dest" {
		t.Errorf("destination = %q, want %q", dest, "user@server:/data/dest")
	}
}

// ---------------------------------------------------------------------------
// 3. TestBuildCommand_SSHConfig — verify -e flag with SSH options
// ---------------------------------------------------------------------------
func TestBuildCommand_SSHConfig(t *testing.T) {
	cfg := minimalConfig("/home/user/src", "/data/dest")
	cfg.Sync.SSH = &config.SSHConfig{
		Host:         "myserver.com",
		User:         "deploy",
		Port:         2222,
		IdentityFile: "~/.ssh/id_ed25519",
	}

	s := New(cfg)
	cmd := s.BuildCommand()

	// Should contain -e flag
	eIdx := indexOfArg(cmd, "-e")
	if eIdx < 0 {
		t.Fatal("missing -e flag")
	}

	// The SSH command string follows -e
	sshCmd := cmd[eIdx+1]
	if !strings.Contains(sshCmd, "ssh") {
		t.Errorf("SSH command should start with ssh, got %q", sshCmd)
	}
	if !strings.Contains(sshCmd, "-p 2222") {
		t.Errorf("SSH command missing port, got %q", sshCmd)
	}
	if !strings.Contains(sshCmd, "-i ~/.ssh/id_ed25519") {
		t.Errorf("SSH command missing identity file, got %q", sshCmd)
	}
	// ControlMaster options
	if !strings.Contains(sshCmd, "-o ControlMaster=auto") {
		t.Errorf("SSH command missing ControlMaster, got %q", sshCmd)
	}
	if !strings.Contains(sshCmd, "-o ControlPath=/tmp/esync-ssh-%r@%h:%p") {
		t.Errorf("SSH command missing ControlPath, got %q", sshCmd)
	}
	if !strings.Contains(sshCmd, "-o ControlPersist=600") {
		t.Errorf("SSH command missing ControlPersist, got %q", sshCmd)
	}

	// Destination should be user@host:/path when SSH is configured
	dest := cmd[len(cmd)-1]
	if dest != "deploy@myserver.com:/data/dest" {
		t.Errorf("destination = %q, want %q", dest, "deploy@myserver.com:/data/dest")
	}
}

// ---------------------------------------------------------------------------
// 4. TestBuildCommand_ExcludePatterns — verify --exclude for combined patterns
// ---------------------------------------------------------------------------
func TestBuildCommand_ExcludePatterns(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	cfg.Settings.Ignore = []string{".git", "node_modules"}
	cfg.Settings.Rsync.Ignore = []string{"**/*.tmp", "*.log"}

	s := New(cfg)
	cmd := s.BuildCommand()

	// Should have --exclude for each pattern
	// **/*.tmp should be stripped to *.tmp
	expectedExcludes := []string{".git", "node_modules", "*.tmp", "*.log"}
	for _, pattern := range expectedExcludes {
		expected := "--exclude=" + pattern
		if !containsArg(cmd, expected) {
			t.Errorf("missing %q in %v", expected, cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. TestBuildCommand_ExtraArgs — verify passthrough of extra_args
// ---------------------------------------------------------------------------
func TestBuildCommand_ExtraArgs(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	cfg.Settings.Rsync.ExtraArgs = []string{"--delete", "--verbose"}

	s := New(cfg)
	cmd := s.BuildCommand()

	if !containsArg(cmd, "--delete") {
		t.Errorf("missing --delete in %v", cmd)
	}
	if !containsArg(cmd, "--verbose") {
		t.Errorf("missing --verbose in %v", cmd)
	}
}

// ---------------------------------------------------------------------------
// 6. TestBuildCommand_DryRun — verify --dry-run flag
// ---------------------------------------------------------------------------
func TestBuildCommand_DryRun(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")

	s := New(cfg)
	s.DryRun = true
	cmd := s.BuildCommand()

	if !containsArg(cmd, "--dry-run") {
		t.Errorf("missing --dry-run in %v", cmd)
	}
}

// ---------------------------------------------------------------------------
// 7. TestBuildCommand_Backup — verify --backup and --backup-dir flags
// ---------------------------------------------------------------------------
func TestBuildCommand_Backup(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	cfg.Settings.Rsync.Backup = true
	cfg.Settings.Rsync.BackupDir = ".my_backup"

	s := New(cfg)
	cmd := s.BuildCommand()

	if !containsArg(cmd, "--backup") {
		t.Errorf("missing --backup in %v", cmd)
	}
	if !containsArg(cmd, "--backup-dir=.my_backup") {
		t.Errorf("missing --backup-dir=.my_backup in %v", cmd)
	}
}

// ---------------------------------------------------------------------------
// Additional tests for helper functions
// ---------------------------------------------------------------------------

func TestExtractFiles(t *testing.T) {
	output := `sending incremental file list
src/main.go
src/utils.go
config.toml

sent 1,234 bytes  received 56 bytes  2,580.00 bytes/sec
total size is 5,678  speedup is 4.40
`
	s := New(minimalConfig("/src", "/dst"))
	files := s.extractFiles(output)

	// Should extract the file lines (not the header or stats)
	if len(files) != 3 {
		t.Fatalf("extractFiles returned %d files, want 3: %v", len(files), files)
	}
	expected := []string{"src/main.go", "src/utils.go", "config.toml"}
	for i, f := range files {
		if f.Name != expected[i] {
			t.Errorf("files[%d].Name = %q, want %q", i, f.Name, expected[i])
		}
	}
}

func TestExtractStats(t *testing.T) {
	output := `sending incremental file list
src/main.go

Number of files: 10
Number of regular files transferred: 3
Total file size: 99,999 bytes
Total transferred file size: 5,678 bytes
sent 1,234 bytes  received 56 bytes  2,580.00 bytes/sec
total size is 5,678  speedup is 4.40
`
	s := New(minimalConfig("/src", "/dst"))
	count, bytes := s.extractStats(output)

	if count != 3 {
		t.Errorf("extractStats count = %d, want 3", count)
	}
	if bytes != 5678 {
		t.Errorf("extractStats bytes = %d, want 5678", bytes)
	}
}

func TestBuildSSHCommand_NoSSH(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	s := New(cfg)
	sshCmd := s.buildSSHCommand()
	if sshCmd != "" {
		t.Errorf("buildSSHCommand() = %q, want empty string for no SSH config", sshCmd)
	}
}

func TestBuildDestination_Local(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	s := New(cfg)
	dest := s.buildDestination()
	if dest != "/dst" {
		t.Errorf("buildDestination() = %q, want %q", dest, "/dst")
	}
}

func TestBuildDestination_SSHWithUser(t *testing.T) {
	cfg := minimalConfig("/src", "/remote/path")
	cfg.Sync.SSH = &config.SSHConfig{
		Host: "myserver.com",
		User: "deploy",
	}
	s := New(cfg)
	dest := s.buildDestination()
	if dest != "deploy@myserver.com:/remote/path" {
		t.Errorf("buildDestination() = %q, want %q", dest, "deploy@myserver.com:/remote/path")
	}
}

func TestBuildDestination_SSHWithoutUser(t *testing.T) {
	cfg := minimalConfig("/src", "/remote/path")
	cfg.Sync.SSH = &config.SSHConfig{
		Host: "myserver.com",
	}
	s := New(cfg)
	dest := s.buildDestination()
	if dest != "myserver.com:/remote/path" {
		t.Errorf("buildDestination() = %q, want %q", dest, "myserver.com:/remote/path")
	}
}

// ---------------------------------------------------------------------------
// 8. TestBuildCommand_IncludePatterns — verify include/exclude filter rules
// ---------------------------------------------------------------------------
func TestBuildCommand_IncludePatterns(t *testing.T) {
	cfg := minimalConfig("/src", "/dst")
	cfg.Settings.Include = []string{"src", "docs/api"}
	cfg.Settings.Ignore = []string{".git"}

	s := New(cfg)
	cmd := s.BuildCommand()

	// Should have include rules for parent dirs, subtrees, then excludes, then catch-all
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

// ---------------------------------------------------------------------------
// 9. TestBuildCommand_NoIncludeMeansNoFilterRules — no include = no catch-all
// ---------------------------------------------------------------------------
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

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func containsArgPrefix(args []string, prefix string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

func indexOfArg(args []string, target string) int {
	for i, a := range args {
		if a == target {
			return i
		}
	}
	return -1
}
