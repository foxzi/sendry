package main

import (
	"fmt"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	RunE:  runMigrate,
}

func init() {
	migrateCmd.Flags().StringVarP(&configFile, "config", "c", "/etc/sendry/web.yaml", "Path to configuration file")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		return err
	}

	fmt.Println("Migrations completed successfully")
	return nil
}
