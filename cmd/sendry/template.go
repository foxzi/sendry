package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/foxzi/sendry/internal/config"
	"github.com/foxzi/sendry/internal/queue"
	"github.com/foxzi/sendry/internal/template"
)

var (
	templateName        string
	templateDescription string
	templateSubject     string
	templateHTMLFile    string
	templateTextFile    string
	templateDataJSON    string
	templateOutputDir   string
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Template management commands",
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all templates",
	RunE:  runTemplateList,
}

var templateCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new template",
	RunE:  runTemplateCreate,
}

var templateShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show template details",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateShow,
}

var templatePreviewCmd = &cobra.Command{
	Use:   "preview <id>",
	Short: "Preview template with test data",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplatePreview,
}

var templateDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateDelete,
}

var templateExportCmd = &cobra.Command{
	Use:   "export <id>",
	Short: "Export template to files",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateExport,
}

var templateImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import template from files",
	RunE:  runTemplateImport,
}

func init() {
	// Flags for create
	templateCreateCmd.Flags().StringVar(&templateName, "name", "", "Template name (required)")
	templateCreateCmd.Flags().StringVar(&templateDescription, "description", "", "Template description")
	templateCreateCmd.Flags().StringVar(&templateSubject, "subject", "", "Subject template (required)")
	templateCreateCmd.Flags().StringVar(&templateHTMLFile, "html", "", "HTML template file")
	templateCreateCmd.Flags().StringVar(&templateTextFile, "text", "", "Text template file")
	templateCreateCmd.MarkFlagRequired("name")
	templateCreateCmd.MarkFlagRequired("subject")

	// Flags for preview
	templatePreviewCmd.Flags().StringVar(&templateDataJSON, "data", "{}", "JSON data for preview")

	// Flags for export
	templateExportCmd.Flags().StringVar(&templateOutputDir, "output", "./", "Output directory")

	// Flags for import
	templateImportCmd.Flags().StringVar(&templateName, "name", "", "Template name (required)")
	templateImportCmd.Flags().StringVar(&templateDescription, "description", "", "Template description")
	templateImportCmd.Flags().StringVar(&templateSubject, "subject", "", "Subject template (required)")
	templateImportCmd.Flags().StringVar(&templateHTMLFile, "html", "", "HTML template file")
	templateImportCmd.Flags().StringVar(&templateTextFile, "text", "", "Text template file")
	templateImportCmd.MarkFlagRequired("name")
	templateImportCmd.MarkFlagRequired("subject")

	templateCmd.AddCommand(
		templateListCmd,
		templateCreateCmd,
		templateShowCmd,
		templatePreviewCmd,
		templateDeleteCmd,
		templateExportCmd,
		templateImportCmd,
	)
	rootCmd.AddCommand(templateCmd)
}

func getTemplateStorage() (*template.Storage, func(), error) {
	if cfgFile == "" {
		return nil, nil, fmt.Errorf("config file is required (use -c flag)")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	storage, err := queue.NewBoltStorage(cfg.Storage.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open storage: %w", err)
	}

	templateStorage, err := template.NewStorage(storage.DB())
	if err != nil {
		storage.Close()
		return nil, nil, fmt.Errorf("failed to create template storage: %w", err)
	}

	cleanup := func() {
		storage.Close()
	}

	return templateStorage, cleanup, nil
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	storage, cleanup, err := getTemplateStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	templates, err := storage.List(cmd.Context(), template.ListFilter{})
	if err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}

	if len(templates) == 0 {
		fmt.Println("No templates found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSUBJECT\tVERSION\tUPDATED")
	for _, tmpl := range templates {
		subject := tmpl.Subject
		if len(subject) > 40 {
			subject = subject[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			tmpl.ID[:8],
			tmpl.Name,
			subject,
			tmpl.Version,
			tmpl.UpdatedAt.Format("2006-01-02 15:04"),
		)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d templates\n", len(templates))
	return nil
}

func runTemplateCreate(cmd *cobra.Command, args []string) error {
	storage, cleanup, err := getTemplateStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	tmpl := &template.Template{
		Name:        templateName,
		Description: templateDescription,
		Subject:     templateSubject,
	}

	// Read HTML file
	if templateHTMLFile != "" {
		data, err := os.ReadFile(templateHTMLFile)
		if err != nil {
			return fmt.Errorf("failed to read HTML file: %w", err)
		}
		tmpl.HTML = string(data)
	}

	// Read text file
	if templateTextFile != "" {
		data, err := os.ReadFile(templateTextFile)
		if err != nil {
			return fmt.Errorf("failed to read text file: %w", err)
		}
		tmpl.Text = string(data)
	}

	if tmpl.HTML == "" && tmpl.Text == "" {
		return fmt.Errorf("at least one of --html or --text is required")
	}

	// Validate template
	engine := template.NewEngine()
	if err := engine.Validate(tmpl); err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	if err := storage.Create(cmd.Context(), tmpl); err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	fmt.Printf("Template created successfully\n")
	fmt.Printf("  ID:   %s\n", tmpl.ID)
	fmt.Printf("  Name: %s\n", tmpl.Name)
	return nil
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
	storage, cleanup, err := getTemplateStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	id := args[0]
	tmpl, err := storage.Get(cmd.Context(), id)
	if err != nil {
		return fmt.Errorf("failed to get template: %w", err)
	}

	if tmpl == nil {
		// Try by name
		tmpl, err = storage.GetByName(cmd.Context(), id)
		if err != nil {
			return fmt.Errorf("failed to get template: %w", err)
		}
	}

	if tmpl == nil {
		return fmt.Errorf("template not found: %s", id)
	}

	fmt.Printf("ID:          %s\n", tmpl.ID)
	fmt.Printf("Name:        %s\n", tmpl.Name)
	fmt.Printf("Description: %s\n", tmpl.Description)
	fmt.Printf("Version:     %d\n", tmpl.Version)
	fmt.Printf("Created:     %s\n", tmpl.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:     %s\n", tmpl.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\nSubject:\n  %s\n", tmpl.Subject)

	if tmpl.Text != "" {
		fmt.Printf("\nText Template:\n")
		for _, line := range strings.Split(tmpl.Text, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	if tmpl.HTML != "" {
		fmt.Printf("\nHTML Template:\n")
		lines := strings.Split(tmpl.HTML, "\n")
		if len(lines) > 20 {
			for _, line := range lines[:20] {
				fmt.Printf("  %s\n", line)
			}
			fmt.Printf("  ... (%d more lines)\n", len(lines)-20)
		} else {
			for _, line := range lines {
				fmt.Printf("  %s\n", line)
			}
		}
	}

	if len(tmpl.Variables) > 0 {
		fmt.Printf("\nVariables:\n")
		for _, v := range tmpl.Variables {
			req := ""
			if v.Required {
				req = " (required)"
			}
			fmt.Printf("  - %s: %s%s\n", v.Name, v.Description, req)
		}
	}

	return nil
}

func runTemplatePreview(cmd *cobra.Command, args []string) error {
	storage, cleanup, err := getTemplateStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	id := args[0]
	tmpl, err := storage.Get(cmd.Context(), id)
	if err != nil {
		return fmt.Errorf("failed to get template: %w", err)
	}

	if tmpl == nil {
		tmpl, err = storage.GetByName(cmd.Context(), id)
		if err != nil {
			return fmt.Errorf("failed to get template: %w", err)
		}
	}

	if tmpl == nil {
		return fmt.Errorf("template not found: %s", id)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(templateDataJSON), &data); err != nil {
		return fmt.Errorf("invalid JSON data: %w", err)
	}

	engine := template.NewEngine()
	result, err := engine.Render(tmpl, data)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	fmt.Printf("Subject:\n  %s\n\n", result.Subject)

	if result.Text != "" {
		fmt.Printf("Text:\n")
		for _, line := range strings.Split(result.Text, "\n") {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
	}

	if result.HTML != "" {
		fmt.Printf("HTML:\n")
		for _, line := range strings.Split(result.HTML, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}

func runTemplateDelete(cmd *cobra.Command, args []string) error {
	storage, cleanup, err := getTemplateStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	id := args[0]
	if err := storage.Delete(cmd.Context(), id); err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	fmt.Printf("Template deleted: %s\n", id)
	return nil
}

func runTemplateExport(cmd *cobra.Command, args []string) error {
	storage, cleanup, err := getTemplateStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	id := args[0]
	tmpl, err := storage.Get(cmd.Context(), id)
	if err != nil {
		return fmt.Errorf("failed to get template: %w", err)
	}

	if tmpl == nil {
		tmpl, err = storage.GetByName(cmd.Context(), id)
		if err != nil {
			return fmt.Errorf("failed to get template: %w", err)
		}
	}

	if tmpl == nil {
		return fmt.Errorf("template not found: %s", id)
	}

	// Create output directory
	dir := templateOutputDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Export metadata
	meta := map[string]interface{}{
		"name":        tmpl.Name,
		"description": tmpl.Description,
		"subject":     tmpl.Subject,
		"variables":   tmpl.Variables,
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	metaFile := fmt.Sprintf("%s/%s.json", dir, tmpl.Name)
	if err := os.WriteFile(metaFile, metaData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	fmt.Printf("Exported: %s\n", metaFile)

	// Export HTML
	if tmpl.HTML != "" {
		htmlFile := fmt.Sprintf("%s/%s.html", dir, tmpl.Name)
		if err := os.WriteFile(htmlFile, []byte(tmpl.HTML), 0644); err != nil {
			return fmt.Errorf("failed to write HTML: %w", err)
		}
		fmt.Printf("Exported: %s\n", htmlFile)
	}

	// Export text
	if tmpl.Text != "" {
		textFile := fmt.Sprintf("%s/%s.txt", dir, tmpl.Name)
		if err := os.WriteFile(textFile, []byte(tmpl.Text), 0644); err != nil {
			return fmt.Errorf("failed to write text: %w", err)
		}
		fmt.Printf("Exported: %s\n", textFile)
	}

	return nil
}

func runTemplateImport(cmd *cobra.Command, args []string) error {
	storage, cleanup, err := getTemplateStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	tmpl := &template.Template{
		Name:        templateName,
		Description: templateDescription,
		Subject:     templateSubject,
	}

	// Read HTML file
	if templateHTMLFile != "" {
		data, err := os.ReadFile(templateHTMLFile)
		if err != nil {
			return fmt.Errorf("failed to read HTML file: %w", err)
		}
		tmpl.HTML = string(data)
	}

	// Read text file
	if templateTextFile != "" {
		data, err := os.ReadFile(templateTextFile)
		if err != nil {
			return fmt.Errorf("failed to read text file: %w", err)
		}
		tmpl.Text = string(data)
	}

	if tmpl.HTML == "" && tmpl.Text == "" {
		return fmt.Errorf("at least one of --html or --text is required")
	}

	// Validate template
	engine := template.NewEngine()
	if err := engine.Validate(tmpl); err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	if err := storage.Create(cmd.Context(), tmpl); err != nil {
		return fmt.Errorf("failed to import template: %w", err)
	}

	fmt.Printf("Template imported successfully\n")
	fmt.Printf("  ID:   %s\n", tmpl.ID)
	fmt.Printf("  Name: %s\n", tmpl.Name)
	return nil
}
