package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/louloulibs/esync/internal/config"
	"github.com/louloulibs/esync/internal/logger"
	"github.com/louloulibs/esync/internal/syncer"
	"github.com/louloulibs/esync/internal/tui"
	"github.com/louloulibs/esync/internal/watcher"
)

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

var (
	localPath   string
	remotePath  string
	daemon      bool
	dryRun      bool
	initialSync bool
	verbose     bool
)

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Watch and sync files to a remote destination",
	Long:  "Watch a local directory for changes and automatically sync them to a remote destination using rsync.",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().StringVarP(&localPath, "local", "l", "", "local path to watch")
	syncCmd.Flags().StringVarP(&remotePath, "remote", "r", "", "remote destination path")
	syncCmd.Flags().BoolVar(&daemon, "daemon", false, "run in daemon mode (no TUI)")
	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be synced without syncing")
	syncCmd.Flags().BoolVar(&initialSync, "initial-sync", false, "force a full sync on startup")
	syncCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	rootCmd.AddCommand(syncCmd)
}

// reProgress2 matches the percentage in rsync --info=progress2 output lines.
var reProgress2 = regexp.MustCompile(`(\d+)%`)

// ---------------------------------------------------------------------------
// Config loading
// ---------------------------------------------------------------------------

// loadOrBuildConfig resolves configuration from CLI flags, a config file, or
// builds a minimal config in memory when --local and --remote are both given.
func loadOrBuildConfig() (*config.Config, error) {
	// 1. Explicit config file via -c flag
	if cfgFile != "" {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("loading config %s: %w", cfgFile, err)
		}
		applyCLIOverrides(cfg)
		return cfg, nil
	}

	// 2. Quick mode: both --local and --remote provided
	if localPath != "" && remotePath != "" {
		cfg := &config.Config{
			Sync: config.SyncSection{
				Local:    localPath,
				Remote:   remotePath,
				Interval: 1,
			},
			Settings: config.Settings{
				WatcherDebounce: 500,
				InitialSync:     initialSync,
				Ignore:          []string{".git", "node_modules", ".DS_Store"},
				Rsync: config.RsyncSettings{
					Archive:  true,
					Compress: true,
				},
			},
		}
		return cfg, nil
	}

	// 3. Auto-detect config file
	path := config.FindConfigFile()
	if path == "" {
		return nil, fmt.Errorf("no config file found; use -c, or provide both -l and -r")
	}

	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}
	applyCLIOverrides(cfg)
	return cfg, nil
}

// applyCLIOverrides applies command-line flag values onto a loaded config.
func applyCLIOverrides(cfg *config.Config) {
	if localPath != "" {
		cfg.Sync.Local = localPath
	}
	if remotePath != "" {
		cfg.Sync.Remote = remotePath
	}
	if initialSync {
		cfg.Settings.InitialSync = true
	}
}

// ---------------------------------------------------------------------------
// Run entry point
// ---------------------------------------------------------------------------

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := loadOrBuildConfig()
	if err != nil {
		return err
	}

	s := syncer.New(cfg)
	s.DryRun = dryRun

	// Optional initial sync
	if cfg.Settings.InitialSync {
		if verbose {
			fmt.Println("Running initial sync...")
		}
		result, err := s.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Initial sync error: %s\n", result.ErrorMessage)
		} else if verbose {
			fmt.Printf("Initial sync complete: %d files, %s\n", result.FilesCount, formatSize(result.BytesTotal))
		}
	}

	if daemon {
		return runDaemon(cfg, s)
	}
	return runTUI(cfg, s)
}

// ---------------------------------------------------------------------------
// TUI mode
// ---------------------------------------------------------------------------

func runTUI(cfg *config.Config, s *syncer.Syncer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := tui.NewApp(cfg.Sync.Local, cfg.Sync.Remote)
	syncCh := app.SyncEventChan()

	logCh := app.LogEntryChan()

	handler := func() {
		// Update header status to syncing
		syncCh <- tui.SyncEvent{Status: "status:syncing"}

		var lastPct string
		onLine := func(line string) {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				return
			}
			// Stream to log view
			select {
			case logCh <- tui.LogEntry{Time: time.Now(), Level: "INF", Message: trimmed}:
			default:
			}
			// Parse progress2 percentage and update header
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

		// Group files by top-level directory
		groups := groupFilesByTopLevel(result.Files)

		// Per-file sizes from --progress are unreliable with --info=progress2
		// (fast transfers may skip the 100% line), so when per-file sizes
		// are missing, distribute the rsync --stats total across groups
		// weighted by file count.
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

		// Fallback: rsync ran but no individual files parsed
		if len(groups) == 0 && result.FilesCount > 0 {
			syncCh <- tui.SyncEvent{
				File:     fmt.Sprintf("%d files", result.FilesCount),
				Size:     formatSize(result.BytesTotal),
				Duration: result.Duration,
				Status:   "synced",
				Time:     now,
			}
		}

		// Reset header status
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
		return fmt.Errorf("creating watcher: %w", err)
	}

	if err := w.Start(); err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}

	resyncCh := app.ResyncChan()
	go func() {
		for range resyncCh {
			handler()
		}
	}()

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		w.Stop()
		return fmt.Errorf("TUI error: %w", err)
	}

	w.Stop()
	return nil
}

// ---------------------------------------------------------------------------
// Daemon mode
// ---------------------------------------------------------------------------

func runDaemon(cfg *config.Config, s *syncer.Syncer) error {
	// Write PID file so `esync status` can find us
	pidPath := filepath.Join(os.TempDir(), "esync.pid")
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	defer os.Remove(pidPath)

	var log *logger.Logger
	if cfg.Settings.Log.File != "" {
		var err error
		log, err = logger.New(cfg.Settings.Log.File, cfg.Settings.Log.Format)
		if err != nil {
			return fmt.Errorf("creating logger: %w", err)
		}
		defer log.Close()
	}

	fmt.Printf("esync daemon started (PID %d)\n", os.Getpid())
	fmt.Printf("Watching: %s -> %s\n", cfg.Sync.Local, cfg.Sync.Remote)

	if log != nil {
		log.Info("started", map[string]interface{}{
			"local":  cfg.Sync.Local,
			"remote": cfg.Sync.Remote,
			"pid":    os.Getpid(),
		})
	}

	handler := func() {
		result, err := s.Run()

		if err != nil {
			msg := result.ErrorMessage
			if verbose {
				fmt.Fprintf(os.Stderr, "Sync error: %s\n", msg)
			}
			if log != nil {
				log.Error("sync_failed", map[string]interface{}{
					"error": msg,
				})
			}
			// Terminal bell on error
			fmt.Print("\a")
			return
		}

		if verbose {
			fmt.Printf("Synced %d files (%s) in %s\n",
				result.FilesCount,
				formatSize(result.BytesTotal),
				result.Duration.Truncate(time.Millisecond),
			)
		}
		if log != nil {
			log.Info("sync_complete", map[string]interface{}{
				"files":    result.FilesCount,
				"bytes":    result.BytesTotal,
				"duration": result.Duration.String(),
			})
		}
	}

	w, err := watcher.New(
		cfg.Sync.Local,
		cfg.Settings.WatcherDebounce,
		cfg.AllIgnorePatterns(),
		cfg.Settings.Include,
		handler,
	)
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}

	if err := w.Start(); err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}
	defer w.Stop()

	// Block until SIGINT or SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	if log != nil {
		log.Info("stopping", nil)
	}
	fmt.Println("\nesync daemon stopped.")
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// formatSize converts a byte count to a human-readable string (B, KB, MB, GB).
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// groupedEvent represents a top-level directory or root file for the TUI.
type groupedEvent struct {
	name  string   // "cmd/" or "main.go"
	count int      // number of files (1 for root files)
	bytes int64    // total bytes
	files []string // individual file paths within the group
}

// groupFilesByTopLevel collapses file entries into top-level directories
// and root files. "cmd/sync.go" + "cmd/init.go" become one entry "cmd/" with count=2.
// When a directory contains only one file, the full relative path is kept.
func groupFilesByTopLevel(files []syncer.FileEntry) []groupedEvent {
	dirMap := make(map[string]*groupedEvent)
	// Track the original filename for single-file groups.
	dirFirstFile := make(map[string]string)
	var rootFiles []groupedEvent
	var dirOrder []string

	for _, f := range files {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) == 1 {
			// Root-level file
			rootFiles = append(rootFiles, groupedEvent{
				name:  f.Name,
				count: 1,
				bytes: f.Bytes,
			})
		} else {
			dir := parts[0] + "/"
			if g, ok := dirMap[dir]; ok {
				g.count++
				g.bytes += f.Bytes
				g.files = append(g.files, f.Name)
			} else {
				dirMap[dir] = &groupedEvent{
					name:  dir,
					count: 1,
					bytes: f.Bytes,
					files: []string{f.Name},
				}
				dirFirstFile[dir] = f.Name
				dirOrder = append(dirOrder, dir)
			}
		}
	}

	var out []groupedEvent
	for _, dir := range dirOrder {
		g := *dirMap[dir]
		if g.count == 1 {
			g.name = dirFirstFile[dir]
			g.files = nil
		}
		out = append(out, g)
	}
	out = append(out, rootFiles...)
	return out
}

// truncateFiles returns at most n elements from files.
func truncateFiles(files []string, n int) []string {
	if len(files) <= n {
		return files
	}
	return files[:n]
}
