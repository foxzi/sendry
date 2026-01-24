package main

import (
	"fmt"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration commands",
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	RunE:  runConfigValidate,
}

func init() {
	configValidateCmd.Flags().StringVarP(&configFile, "config", "c", "/etc/sendry/web.yaml", "Path to configuration file")
	configCmd.AddCommand(configValidateCmd)
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	fmt.Println("Configuration is valid")
	fmt.Printf("  Listen address: %s\n", cfg.Server.ListenAddr)
	fmt.Printf("  Database path: %s\n", cfg.Database.Path)
	fmt.Printf("  Local auth: %v\n", cfg.Auth.LocalEnabled)
	fmt.Printf("  OIDC auth: %v\n", cfg.Auth.OIDC.Enabled)
	fmt.Printf("  Sendry servers: %d\n", len(cfg.Sendry.Servers))

	for _, s := range cfg.Sendry.Servers {
		fmt.Printf("    - %s (%s) [%s]\n", s.Name, s.BaseURL, s.Env)
	}

	return nil
}
