package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if an esync daemon is running",
	Long:  "Read the PID file and report whether an esync daemon process is currently alive.",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

func runStatus(cmd *cobra.Command, args []string) error {
	pidPath := filepath.Join(os.TempDir(), "esync.pid")

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No esync daemon running.")
			return nil
		}
		return fmt.Errorf("reading PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid PID file content: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("No esync daemon running (stale PID file).")
		os.Remove(pidPath)
		return nil
	}

	// Signal 0 checks whether the process is alive without actually sending a signal.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("No esync daemon running (stale PID file).")
		os.Remove(pidPath)
		return nil
	}

	fmt.Printf("esync daemon running (PID %d)\n", pid)
	return nil
}
