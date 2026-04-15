// Package syncer builds and executes rsync commands based on esync
// configuration, handling local and remote (SSH) destinations.
package syncer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/louloulibs/esync/internal/config"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ProgressFunc is called for each line of rsync output during RunWithProgress.
type ProgressFunc func(line string)

// FileEntry records a transferred file and its size in bytes.
type FileEntry struct {
	Name  string
	Bytes int64
}

// Result captures the outcome of a sync operation.
type Result struct {
	Success      bool
	FilesCount   int
	BytesTotal   int64
	Duration     time.Duration
	Files        []FileEntry
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

// rsyncBin returns the path to the rsync binary, preferring a homebrew
// install over the macOS system openrsync (which lacks --info=progress2).
func rsyncBin() string {
	candidates := []string{
		"/opt/homebrew/bin/rsync",
		"/usr/local/bin/rsync",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "rsync" // fallback to PATH
}

// minRsyncVersion is the minimum rsync version required for --info=progress2.
const minRsyncVersion = "3.1.0"

// CheckRsync verifies that rsync is installed and returns its version string.
// Returns an error if rsync is not found on PATH or if the version is too old
// (--info=progress2 requires rsync >= 3.1.0).
func CheckRsync() (string, error) {
	out, err := exec.Command(rsyncBin(), "--version").Output()
	if err != nil {
		return "", fmt.Errorf("rsync not found: %w\nInstall rsync 3.1+ (e.g. brew install rsync, apt install rsync) and try again", err)
	}
	// First line is "rsync  version X.Y.Z  protocol version N"
	firstLine := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])

	// Extract version number
	if m := reRsyncVersion.FindStringSubmatch(firstLine); len(m) > 1 {
		if compareVersions(m[1], minRsyncVersion) < 0 {
			return firstLine, fmt.Errorf("rsync %s is too old (need %s+); install a newer rsync (e.g. brew install rsync)", m[1], minRsyncVersion)
		}
	}

	return firstLine, nil
}

// reRsyncVersion extracts the version number from rsync --version output.
var reRsyncVersion = regexp.MustCompile(`version\s+(\d+\.\d+\.\d+)`)

// compareVersions compares two dotted version strings (e.g. "3.1.0" vs "2.6.9").
// Returns -1, 0, or 1.
func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, _ := strconv.Atoi(pa[i])
		nb, _ := strconv.Atoi(pb[i])
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return len(pa) - len(pb)
}

// BuildCommand constructs the rsync argument list with all flags, excludes,
// SSH options, extra_args, source (trailing /), and destination.
func (s *Syncer) BuildCommand() []string {
	args := []string{rsyncBin(), "--recursive", "--times", "--progress", "--stats", "--info=progress2"}

	rsync := s.cfg.Settings.Rsync

	// Symlink handling: --copy-links dereferences all symlinks,
	// --copy-unsafe-links only dereferences symlinks pointing outside the tree.
	if rsync.CopyLinks {
		args = append(args, "--copy-links")
	} else {
		args = append(args, "--copy-unsafe-links")
	}

	// Conditional flags
	if rsync.Archive {
		args = append(args, "--archive")
	}
	if rsync.Compress {
		args = append(args, "--compress")
	}
	if rsync.Delete {
		args = append(args, "--delete")
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

	// Include/exclude filter rules.
	//
	// rsync filter rules are first-match-wins, so ignore --exclude rules
	// MUST be emitted before --include rules. Otherwise a wildcard include
	// like --include=worktree/** shadows a later --exclude=.venv/ and lets
	// ignored files through (issue #14).
	if len(s.cfg.Settings.Include) > 0 {
		// 1. Ignore excludes first — take precedence over includes.
		for _, pattern := range s.cfg.AllIgnorePatterns() {
			cleaned := strings.TrimPrefix(pattern, "**/")
			args = append(args, "--exclude="+cleaned)
		}

		// 2. Include rules: ancestor dirs + prefix + subtree for each entry.
		seen := make(map[string]bool)
		for _, inc := range s.cfg.Settings.Include {
			inc = filepath.Clean(inc)
			parts := strings.Split(inc, string(filepath.Separator))
			for i := 1; i < len(parts); i++ {
				ancestor := strings.Join(parts[:i], "/") + "/"
				if !seen[ancestor] {
					args = append(args, "--include="+ancestor)
					seen[ancestor] = true
				}
			}
			args = append(args, "--include="+inc)
			args = append(args, "--include="+inc+"/")
			args = append(args, "--include="+inc+"/**")
		}

		// 3. Catch-all exclude: block everything not explicitly included.
		args = append(args, "--exclude=*")
	} else {
		// No include filter — just emit excludes.
		for _, pattern := range s.cfg.AllIgnorePatterns() {
			cleaned := strings.TrimPrefix(pattern, "**/")
			args = append(args, "--exclude="+cleaned)
		}
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
	return s.RunContext(context.Background())
}

// RunContext executes the rsync command with a context for cancellation.
func (s *Syncer) RunContext(ctx context.Context) (*Result, error) {
	args := s.BuildCommand()

	start := time.Now()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
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

// RunWithProgress executes rsync while streaming each output line to onLine.
// The context allows cancellation (e.g. when the TUI exits).
// If onLine is nil it falls through to RunContext().
func (s *Syncer) RunWithProgress(ctx context.Context, onLine ProgressFunc) (*Result, error) {
	if onLine == nil {
		return s.RunContext(ctx)
	}

	args := s.BuildCommand()
	start := time.Now()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("creating pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return nil, fmt.Errorf("starting rsync: %w", err)
	}
	pw.Close() // parent closes write end so scanner sees EOF

	var buf strings.Builder
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line + "\n")
		onLine(line)
	}
	pr.Close()

	waitErr := cmd.Wait()
	duration := time.Since(start)
	outStr := buf.String()

	result := &Result{
		Duration: duration,
		Files:    s.extractFiles(outStr),
	}
	count, bytes := s.extractStats(outStr)
	result.FilesCount = count
	result.BytesTotal = bytes

	if waitErr != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("rsync error: %v\n%s", waitErr, outStr)
		return result, waitErr
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

// reProgressSize matches the final size in a progress line, e.g.
// "  8772 100%  61.99MB/s  00:00:00 (xfer#1, to-check=2/4)"
var reProgressSize = regexp.MustCompile(`^\s*([\d,]+)\s+100%`)

// extractFiles extracts transferred file names and per-file sizes from
// rsync --progress output. Each filename line is followed by one or more
// progress lines; the final one (with "100%") contains the file size.
func (s *Syncer) extractFiles(output string) []FileEntry {
	var files []FileEntry
	lines := strings.Split(output, "\n")

	var pending string // last seen filename awaiting a size

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || trimmed == "sending incremental file list" {
			continue
		}

		// Stop at stats section
		if strings.HasPrefix(trimmed, "Number of") ||
			strings.HasPrefix(trimmed, "sent ") ||
			strings.HasPrefix(trimmed, "total size") {
			break
		}

		// Skip directory entries
		if strings.HasSuffix(trimmed, "/") || trimmed == "." || trimmed == "./" {
			continue
		}

		// Check if this is a per-file 100% progress line (extract size).
		// Must come before the progress2 guard since both contain xfr#/to-chk=.
		if m := reProgressSize.FindStringSubmatch(trimmed); len(m) > 1 && pending != "" {
			cleaned := strings.ReplaceAll(m[1], ",", "")
			size, _ := strconv.ParseInt(cleaned, 10, 64)
			files = append(files, FileEntry{Name: pending, Bytes: size})
			pending = ""
			continue
		}

		// Skip --info=progress2 summary lines (partial %, e.g. "1,234  56%  1.23MB/s  0:00:01 (xfr#1, to-chk=2/4)")
		if strings.Contains(trimmed, "xfr#") || strings.Contains(trimmed, "to-chk=") {
			continue
		}

		// Skip other progress lines (partial %, bytes/sec)
		if strings.Contains(trimmed, "%") || strings.Contains(trimmed, "bytes/sec") {
			continue
		}

		// Flush any pending file without a matched size
		if pending != "" {
			files = append(files, FileEntry{Name: pending})
			pending = ""
		}

		// This looks like a filename
		pending = trimmed
	}

	// Flush last pending
	if pending != "" {
		files = append(files, FileEntry{Name: pending})
	}

	return files
}

// extractStats extracts the file count and total bytes from rsync output.
// It looks for "Number of regular files transferred: N" and
// "Total file size: N bytes" patterns.
func (s *Syncer) extractStats(output string) (int, int64) {
	var count int
	var totalBytes int64

	// Match "Number of regular files transferred: 3" or "Number of files transferred: 2"
	reCount := regexp.MustCompile(`Number of (?:regular )?files transferred:\s*([\d,]+)`)
	if m := reCount.FindStringSubmatch(output); len(m) > 1 {
		cleaned := strings.ReplaceAll(m[1], ",", "")
		if n, err := strconv.Atoi(cleaned); err == nil {
			count = n
		}
	}

	// Match "Total transferred file size: 5,678 bytes" (actual bytes sent,
	// not the total source tree size reported by "Total file size:")
	reBytes := regexp.MustCompile(`Total transferred file size:\s*([\d,]+)`)
	if m := reBytes.FindStringSubmatch(output); len(m) > 1 {
		cleaned := strings.ReplaceAll(m[1], ",", "")
		if n, err := strconv.ParseInt(cleaned, 10, 64); err == nil {
			totalBytes = n
		}
	}

	return count, totalBytes
}
