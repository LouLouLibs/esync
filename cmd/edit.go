package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/louloulibs/esync/internal/config"
)

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config in $EDITOR, then validate and preview",
	Long:  "Open the esync configuration file in your editor. After saving, the config is validated and a file preview is shown.",
	RunE:  runEdit,
}

func init() {
	rootCmd.AddCommand(editCmd)
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

func runEdit(cmd *cobra.Command, args []string) error {
	// 1. Find config file
	path := cfgFile
	if path == "" {
		path = config.FindConfigFile()
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "No config file found. Run `esync init` first to create one.")
		return nil
	}

	// 2. Determine editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	reader := bufio.NewReader(os.Stdin)

	// 3. Edit loop
	for {
		// Open editor
		editorCmd := exec.Command(editor, path)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor exited with error: %w", err)
		}

		// Validate config
		cfg, err := config.Load(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nConfig error: %s\n", err)
			fmt.Print("Press 'e' to edit again, or 'q' to cancel: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer == "q" {
				fmt.Println("Cancelled.")
				return nil
			}
			continue
		}

		// Valid config — show preview
		if err := printPreview(cfg); err != nil {
			return err
		}

		// Ask user what to do
		fmt.Print("Press Enter to accept, 'e' to edit again, or 'q' to cancel: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		switch answer {
		case "q":
			fmt.Println("Cancelled.")
			return nil
		case "e":
			continue
		default:
			// Enter or anything else: accept
			fmt.Println("Config saved.")
			return nil
		}
	}
}
