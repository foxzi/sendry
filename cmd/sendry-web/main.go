package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "sendry-web",
	Short: "Sendry Web - web interface for Sendry email server",
	Long:  `Sendry Web provides a web interface for managing templates, campaigns, and monitoring Sendry email servers.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sendry-web %s (built %s)\n", version, buildTime)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(cleanupCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
