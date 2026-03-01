package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
	app := tui.NewApp(cfg.Sync.Local, cfg.Sync.Remote)
	syncCh := app.SyncEventChan()

	handler := func() {
		// Send a "syncing" event before starting
		syncCh <- tui.SyncEvent{
			File:   cfg.Sync.Local,
			Status: "syncing",
			Time:   time.Now(),
		}

		result, err := s.Run()
		now := time.Now()

		if err != nil {
			syncCh <- tui.SyncEvent{
				File:   cfg.Sync.Local,
				Status: "error",
				Time:   now,
			}
			return
		}

		// Send individual file events
		for _, f := range result.Files {
			syncCh <- tui.SyncEvent{
				File:     f,
				Size:     formatSize(result.BytesTotal),
				Duration: result.Duration,
				Status:   "synced",
				Time:     now,
			}
		}

		// If no individual files reported, send a summary event
		if len(result.Files) == 0 && result.FilesCount > 0 {
			syncCh <- tui.SyncEvent{
				File:     fmt.Sprintf("%d files", result.FilesCount),
				Size:     formatSize(result.BytesTotal),
				Duration: result.Duration,
				Status:   "synced",
				Time:     now,
			}
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
