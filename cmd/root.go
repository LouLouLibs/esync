package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/louloulibs/esync/internal/syncer"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "esync",
	Short: "File synchronization tool using rsync",
	Long:  "A file sync tool that watches for changes and automatically syncs them to a remote destination using rsync.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		_, err := syncer.CheckRsync()
		if err != nil {
			return err
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
}
