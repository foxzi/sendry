package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User management commands",
}

var userCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new user",
	RunE:  runUserCreate,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE:  runUserList,
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete [email]",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserDelete,
}

var userResetPasswordCmd = &cobra.Command{
	Use:   "reset-password [email]",
	Short: "Reset user password",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserResetPassword,
}

var (
	userEmail    string
	userPassword string
	userName     string
)

func init() {
	userCreateCmd.Flags().StringVar(&userEmail, "email", "", "User email")
	userCreateCmd.Flags().StringVar(&userPassword, "password", "", "User password (will prompt if not provided)")
	userCreateCmd.Flags().StringVar(&userName, "name", "", "User name")
	userCreateCmd.MarkFlagRequired("email")

	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userDeleteCmd)
	userCmd.AddCommand(userResetPasswordCmd)

	userCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "/etc/sendry/web.yaml", "Path to configuration file")
}

func runUserCreate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	// Prompt for password if not provided
	password := userPassword
	if password == "" {
		fmt.Print("Enter password: ")
		pwBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()
		password = string(pwBytes)

		fmt.Print("Confirm password: ")
		pwBytes2, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()

		if password != string(pwBytes2) {
			return fmt.Errorf("passwords do not match")
		}
	}

	if len(password) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	id := uuid.New().String()
	_, err = database.Exec(
		"INSERT INTO users (id, email, password_hash, name) VALUES (?, ?, ?, ?)",
		id, userEmail, string(hash), userName,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Errorf("user with email %s already exists", userEmail)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	fmt.Printf("User %s created successfully\n", userEmail)
	return nil
}

func runUserList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	rows, err := database.Query("SELECT id, email, name, created_at FROM users ORDER BY created_at")
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Printf("%-36s  %-30s  %-20s  %s\n", "ID", "Email", "Name", "Created")
	fmt.Println(strings.Repeat("-", 100))

	for rows.Next() {
		var id, email, createdAt string
		var name *string
		if err := rows.Scan(&id, &email, &name, &createdAt); err != nil {
			return err
		}
		nameStr := ""
		if name != nil {
			nameStr = *name
		}
		fmt.Printf("%-36s  %-30s  %-20s  %s\n", id, email, nameStr, createdAt)
	}

	return nil
}

func runUserDelete(cmd *cobra.Command, args []string) error {
	email := args[0]

	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	// Confirm deletion
	fmt.Printf("Are you sure you want to delete user %s? [y/N]: ", email)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Cancelled")
		return nil
	}

	result, err := database.Exec("DELETE FROM users WHERE email = ?", email)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user %s not found", email)
	}

	fmt.Printf("User %s deleted\n", email)
	return nil
}

func runUserResetPassword(cmd *cobra.Command, args []string) error {
	email := args[0]

	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	// Check user exists
	var id string
	err = database.QueryRow("SELECT id FROM users WHERE email = ?", email).Scan(&id)
	if err != nil {
		return fmt.Errorf("user %s not found", email)
	}

	// Prompt for new password
	fmt.Print("Enter new password: ")
	pwBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	fmt.Println()

	fmt.Print("Confirm password: ")
	pwBytes2, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	fmt.Println()

	password := string(pwBytes)
	if password != string(pwBytes2) {
		return fmt.Errorf("passwords do not match")
	}

	if len(password) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	_, err = database.Exec(
		"UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE email = ?",
		string(hash), email,
	)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	fmt.Printf("Password for %s updated successfully\n", email)
	return nil
}
