// Package syncer builds and executes rsync commands based on esync
// configuration, handling local and remote (SSH) destinations.
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

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Result captures the outcome of a sync operation.
type Result struct {
	Success      bool
	FilesCount   int
	BytesTotal   int64
	Duration     time.Duration
	Files        []string
	ErrorMessage string
}

// Syncer builds and executes rsync commands from a config.Config.
type Syncer struct {
	cfg    *config.Config
	DryRun bool
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// New returns a Syncer configured from the given Config.
func New(cfg *config.Config) *Syncer {
	return &Syncer{cfg: cfg}
}

// ---------------------------------------------------------------------------
// Public methods
// ---------------------------------------------------------------------------

// BuildCommand constructs the rsync argument list with all flags, excludes,
// SSH options, extra_args, source (trailing /), and destination.
func (s *Syncer) BuildCommand() []string {
	args := []string{"rsync", "--recursive", "--times", "--progress", "--copy-unsafe-links"}

	rsync := s.cfg.Settings.Rsync

	// Conditional flags
	if rsync.Archive {
		args = append(args, "--archive")
	}
	if rsync.Compress {
		args = append(args, "--compress")
	}
	if rsync.Backup {
		args = append(args, "--backup")
		if rsync.BackupDir != "" {
			args = append(args, "--backup-dir="+rsync.BackupDir)
		}
	}
	if s.DryRun {
		args = append(args, "--dry-run")
	}

	// Exclude patterns (strip **/ prefix)
	for _, pattern := range s.cfg.AllIgnorePatterns() {
		cleaned := strings.TrimPrefix(pattern, "**/")
		args = append(args, "--exclude="+cleaned)
	}

	// Extra args passthrough
	args = append(args, rsync.ExtraArgs...)

	// SSH transport
	if sshCmd := s.buildSSHCommand(); sshCmd != "" {
		args = append(args, "-e", sshCmd)
	}

	// Source (must end with /)
	source := s.cfg.Sync.Local
	if !strings.HasSuffix(source, "/") {
		source += "/"
	}
	args = append(args, source)

	// Destination
	args = append(args, s.buildDestination())

	return args
}

// Run executes the rsync command, captures output, and parses stats.
func (s *Syncer) Run() (*Result, error) {
	args := s.BuildCommand()

	start := time.Now()

	// args[0] is "rsync", the rest are arguments
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	outStr := string(output)

	result := &Result{
		Duration: duration,
		Files:    s.extractFiles(outStr),
	}

	count, bytes := s.extractStats(outStr)
	result.FilesCount = count
	result.BytesTotal = bytes

	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("rsync error: %v\n%s", err, outStr)
		return result, err
	}

	result.Success = true
	return result, nil
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

// buildSSHCommand builds the SSH command string with port, identity file,
// and ControlMaster keepalive options. Returns empty string if no SSH config.
func (s *Syncer) buildSSHCommand() string {
	ssh := s.cfg.Sync.SSH
	if ssh == nil || ssh.Host == "" {
		return ""
	}

	parts := []string{"ssh"}

	if ssh.Port != 0 {
		parts = append(parts, fmt.Sprintf("-p %d", ssh.Port))
	}

	if ssh.IdentityFile != "" {
		parts = append(parts, fmt.Sprintf("-i %s", ssh.IdentityFile))
	}

	// ControlMaster keepalive options
	parts = append(parts,
		"-o ControlMaster=auto",
		"-o ControlPath=/tmp/esync-ssh-%r@%h:%p",
		"-o ControlPersist=600",
	)

	return strings.Join(parts, " ")
}

// buildDestination builds the destination string from SSH config or the raw
// remote string. When SSH config is present, it constructs user@host:path.
func (s *Syncer) buildDestination() string {
	ssh := s.cfg.Sync.SSH
	if ssh == nil || ssh.Host == "" {
		return s.cfg.Sync.Remote
	}

	remote := s.cfg.Sync.Remote
	if ssh.User != "" {
		return fmt.Sprintf("%s@%s:%s", ssh.User, ssh.Host, remote)
	}
	return fmt.Sprintf("%s:%s", ssh.Host, remote)
}

// extractFiles extracts file names from rsync output.
// rsync lists transferred files one per line between the header
// "sending incremental file list" and the blank line before stats.
func (s *Syncer) extractFiles(output string) []string {
	var files []string
	lines := strings.Split(output, "\n")
	inList := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "sending incremental file list" {
			inList = true
			continue
		}

		if !inList {
			continue
		}

		// End of file list: blank line or stats line
		if line == "" || strings.HasPrefix(line, "sent ") || strings.HasPrefix(line, "total size") {
			if line == "" {
				continue
			}
			break
		}

		// Skip progress lines (contain % or bytes/sec mid-line)
		if strings.Contains(line, "%") || strings.Contains(line, "bytes/sec") {
			continue
		}

		// Skip lines starting with "Number of" (stats)
		if strings.HasPrefix(line, "Number of") {
			break
		}

		files = append(files, line)
	}

	return files
}

// extractStats extracts the file count and total bytes from rsync output.
// It looks for "Number of regular files transferred: N" and
// "Total file size: N bytes" patterns.
func (s *Syncer) extractStats(output string) (int, int64) {
	var count int
	var totalBytes int64

	// Match "Number of regular files transferred: 3"
	reCount := regexp.MustCompile(`Number of regular files transferred:\s*([\d,]+)`)
	if m := reCount.FindStringSubmatch(output); len(m) > 1 {
		cleaned := strings.ReplaceAll(m[1], ",", "")
		if n, err := strconv.Atoi(cleaned); err == nil {
			count = n
		}
	}

	// Match "Total file size: 5,678 bytes"
	reBytes := regexp.MustCompile(`Total file size:\s*([\d,]+)`)
	if m := reBytes.FindStringSubmatch(output); len(m) > 1 {
		cleaned := strings.ReplaceAll(m[1], ",", "")
		if n, err := strconv.ParseInt(cleaned, 10, 64); err == nil {
			totalBytes = n
		}
	}

	return count, totalBytes
}
