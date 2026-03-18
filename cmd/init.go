package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/louloulibs/esync/internal/config"
)

// ---------------------------------------------------------------------------
// Default patterns (already present in DefaultTOML)
// ---------------------------------------------------------------------------

// defaultIgnorePatterns lists patterns that DefaultTOML() already includes
// in settings.ignore, so we can skip them when merging from .gitignore.
var defaultIgnorePatterns = map[string]bool{
	".git":         true,
	".git/":        true,
	"node_modules": true,
	"node_modules/": true,
	".DS_Store":    true,
}

// commonDirs lists directories to auto-detect and exclude.
var commonDirs = []string{
	".git",
	"node_modules",
	"__pycache__",
	"build",
	".venv",
	"dist",
	".tox",
	".mypy_cache",
}

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

var initRemote string

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate an .esync.toml configuration file",
	Long:  "Inspect the current directory to generate a smart .esync.toml with .gitignore import and common directory exclusion.",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initRemote, "remote", "r", "", "pre-fill remote destination")
	rootCmd.AddCommand(initCmd)
}

// ---------------------------------------------------------------------------
// Main logic
// ---------------------------------------------------------------------------

func runInit(cmd *cobra.Command, args []string) error {
	// 1. Determine output path
	outPath := cfgFile
	if outPath == "" {
		outPath = "./.esync.toml"
	}

	// 2. If file exists, prompt for overwrite confirmation
	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("File %s already exists. Overwrite? [y/N] ", outPath)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// 3. Start with default TOML content
	content := config.DefaultTOML()

	// 4. Read .gitignore patterns
	gitignorePatterns := readGitignore()

	// 5. Detect common directories that exist and aren't already in defaults
	detectedDirs := detectCommonDirs()

	// 6. Remote destination: use flag or prompt
	remote := initRemote
	if remote == "" {
		fmt.Print("Remote destination (e.g. user@host:/path/to/dest): ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		remote = strings.TrimSpace(line)
	}

	// Replace remote in TOML content if provided
	if remote != "" {
		content = strings.Replace(
			content,
			`remote = "user@host:/path/to/dest"`,
			fmt.Sprintf(`remote = %q`, remote),
			1,
		)
	}

	// 7. Merge extra ignore patterns into TOML content
	var extraPatterns []string
	extraPatterns = append(extraPatterns, gitignorePatterns...)
	extraPatterns = append(extraPatterns, detectedDirs...)

	// Deduplicate: remove any that are already in defaults or duplicated
	seen := make(map[string]bool)
	for k := range defaultIgnorePatterns {
		seen[k] = true
	}
	var uniqueExtras []string
	for _, p := range extraPatterns {
		// Normalize: strip trailing slash for comparison
		normalized := strings.TrimSuffix(p, "/")
		if seen[normalized] || seen[normalized+"/"] || seen[p] {
			continue
		}
		seen[normalized] = true
		seen[normalized+"/"] = true
		uniqueExtras = append(uniqueExtras, p)
	}

	if len(uniqueExtras) > 0 {
		// Build the new ignore list: default patterns + extras
		var quoted []string
		// Start with the defaults already in the TOML
		for _, d := range []string{".git", "node_modules", ".DS_Store"} {
			quoted = append(quoted, fmt.Sprintf("%q", d))
		}
		for _, p := range uniqueExtras {
			quoted = append(quoted, fmt.Sprintf("%q", p))
		}
		newIgnoreLine := "ignore           = [" + strings.Join(quoted, ", ") + "]"
		content = strings.Replace(
			content,
			`ignore           = [".git", "node_modules", ".DS_Store"]`,
			newIgnoreLine,
			1,
		)
	}

	// 8. Write to file
	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	// 9. Print summary
	fmt.Println()
	fmt.Printf("Created %s\n", outPath)
	fmt.Println()
	if len(gitignorePatterns) > 0 {
		fmt.Printf("  Imported %d pattern(s) from .gitignore\n", len(gitignorePatterns))
	}
	if len(detectedDirs) > 0 {
		fmt.Printf("  Auto-excluded %d common dir(s): %s\n",
			len(detectedDirs), strings.Join(detectedDirs, ", "))
	}
	if len(uniqueExtras) > 0 {
		fmt.Printf("  Total extra ignore patterns: %d\n", len(uniqueExtras))
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  esync check   — validate your configuration")
	fmt.Println("  esync edit    — open the config in your editor")

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// readGitignore reads .gitignore in the current directory and returns
// patterns, skipping comments, empty lines, and patterns already present
// in the default ignore list.
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

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip patterns already in the defaults
		normalized := strings.TrimSuffix(line, "/")
		if defaultIgnorePatterns[line] || defaultIgnorePatterns[normalized] || defaultIgnorePatterns[normalized+"/"] {
			continue
		}

		patterns = append(patterns, line)
	}

	return patterns
}

// detectCommonDirs checks for common directories that should typically be
// excluded, returns the ones that exist on disk and aren't already in the
// default ignore list.
func detectCommonDirs() []string {
	var found []string
	for _, dir := range commonDirs {
		// Skip if already in defaults
		if defaultIgnorePatterns[dir] || defaultIgnorePatterns[dir+"/"] {
			continue
		}

		// Check if directory actually exists
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}

		found = append(found, dir)
	}
	return found
}
