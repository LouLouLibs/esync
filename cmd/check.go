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

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	greenHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	yellowHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	dimText      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate config and preview included/excluded files",
	Long:  "Load the esync configuration, walk the local directory, and show which files would be included or excluded by the ignore patterns.",
	RunE:  runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

func runCheck(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return printPreview(cfg)
}

// ---------------------------------------------------------------------------
// Shared: loadConfig
// ---------------------------------------------------------------------------

// loadConfig loads configuration from the -c flag or auto-detects it.
func loadConfig() (*config.Config, error) {
	path := cfgFile
	if path == "" {
		path = config.FindConfigFile()
	}
	if path == "" {
		return nil, fmt.Errorf("no config file found; use -c to specify one, or run `esync init`")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}
	return cfg, nil
}

// ---------------------------------------------------------------------------
// Shared: printPreview
// ---------------------------------------------------------------------------

// fileEntry records a file path and (for excluded files) the rule that matched.
type fileEntry struct {
	path string
	rule string
}

// printPreview walks the local directory and displays included/excluded files.
func printPreview(cfg *config.Config) error {
	localDir := cfg.Sync.Local
	patterns := cfg.AllIgnorePatterns()

	var included []fileEntry
	var excluded []fileEntry
	var includedSize int64

	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return nil
		}

		// Skip the root directory itself
		if rel == "." {
			return nil
		}

		// Check against ignore patterns
		for _, pattern := range patterns {
			if matchesIgnorePattern(rel, info, pattern) {
				excluded = append(excluded, fileEntry{path: rel, rule: pattern})
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if !info.IsDir() {
			included = append(included, fileEntry{path: rel})
			includedSize += info.Size()
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking %s: %w", localDir, err)
	}

	// --- Config summary ---
	fmt.Println()
	fmt.Printf("  Local:  %s\n", cfg.Sync.Local)
	fmt.Printf("  Remote: %s\n", cfg.Sync.Remote)
	fmt.Println()

	// --- Included files ---
	fmt.Println(greenHeader.Render("  Included files:"))
	limit := 10
	for i, f := range included {
		if i >= limit {
			fmt.Printf("    ... %d more files\n", len(included)-limit)
			break
		}
		fmt.Printf("    %s\n", f.path)
	}
	if len(included) == 0 {
		fmt.Println("    (none)")
	}
	fmt.Println()

	// --- Excluded files ---
	fmt.Println(yellowHeader.Render("  Excluded files:"))
	for i, f := range excluded {
		if i >= limit {
			fmt.Printf("    ... %d more excluded\n", len(excluded)-limit)
			break
		}
		fmt.Printf("    %-40s %s\n", f.path, dimText.Render("← "+f.rule))
	}
	if len(excluded) == 0 {
		fmt.Println("    (none)")
	}
	fmt.Println()

	// --- Totals ---
	totals := fmt.Sprintf("  %d files included (%s) | %d excluded",
		len(included), formatSize(includedSize), len(excluded))
	fmt.Println(dimText.Render(totals))
	fmt.Println()

	return nil
}

// ---------------------------------------------------------------------------
// Pattern matching
// ---------------------------------------------------------------------------

// matchesIgnorePattern checks whether a file (given its relative path and
// file info) matches a single ignore pattern. It handles bracket/quote
// stripping, ** prefixes, and directory-specific patterns.
func matchesIgnorePattern(rel string, info os.FileInfo, pattern string) bool {
	// Strip surrounding quotes and brackets
	pattern = strings.Trim(pattern, `"'`)
	pattern = strings.Trim(pattern, "[]")
	pattern = strings.TrimSpace(pattern)

	if pattern == "" {
		return false
	}

	// Check if this is a directory-only pattern (ends with /)
	dirOnly := strings.HasSuffix(pattern, "/")
	cleanPattern := strings.TrimSuffix(pattern, "/")

	// Strip **/ prefix for simpler matching
	cleanPattern = strings.TrimPrefix(cleanPattern, "**/")

	if dirOnly && !info.IsDir() {
		return false
	}

	baseName := filepath.Base(rel)

	// Match against base name
	if matched, _ := filepath.Match(cleanPattern, baseName); matched {
		return true
	}

	// Match against full relative path
	if matched, _ := filepath.Match(cleanPattern, rel); matched {
		return true
	}

	// For directory patterns, also try matching directory components
	if info.IsDir() {
		if matched, _ := filepath.Match(cleanPattern, baseName); matched {
			return true
		}
	}

	return false
}
