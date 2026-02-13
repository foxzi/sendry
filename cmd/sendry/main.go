package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/app"
	"github.com/foxzi/sendry/internal/config"
)

var (
	cfgFile   string
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "sendry",
	Short: "Sendry - MTA Server",
	Long:  `Sendry is a Mail Transfer Agent (MTA) server for sending emails.`,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MTA server",
	Long:  `Start the Sendry MTA server with SMTP and HTTP API.`,
	RunE:  runServe,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration commands",
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	RunE:  runConfigValidate,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sendry version %s\n", version)
		if commit != "unknown" {
			fmt.Printf("  commit: %s\n", commit)
		}
		if buildTime != "unknown" {
			fmt.Printf("  built:  %s\n", buildTime)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")

	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(serveCmd, configCmd, versionCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set up dynamic domains file path in data directory (writable by service)
	dataDir := filepath.Dir(cfg.Storage.Path) // e.g., /var/lib/sendry
	domainsFile := filepath.Join(dataDir, "domains.yaml")
	cfg.SetDomainsFile(domainsFile)

	// Load any previously saved domain configurations
	if err := cfg.LoadDynamicDomains(); err != nil {
		return fmt.Errorf("failed to load dynamic domains: %w", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}

	return application.Run(context.Background())
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	if cfgFile == "" {
		return fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("configuration is invalid: %w", err)
	}

	fmt.Printf("Configuration is valid\n")
	fmt.Printf("  Hostname: %s\n", cfg.Server.Hostname)
	fmt.Printf("  SMTP: %s\n", cfg.SMTP.ListenAddr)
	fmt.Printf("  Submission: %s\n", cfg.SMTP.SubmissionAddr)
	fmt.Printf("  API: %s\n", cfg.API.ListenAddr)
	fmt.Printf("  Storage: %s\n", cfg.Storage.Path)

	return nil
}
