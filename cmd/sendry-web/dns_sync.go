package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/foxzi/sendry/internal/web/config"
	"github.com/foxzi/sendry/internal/web/db"
	"github.com/foxzi/sendry/internal/web/dnsprovider"
	"github.com/foxzi/sendry/internal/web/dnssync"
	"github.com/foxzi/sendry/internal/web/models"
	"github.com/foxzi/sendry/internal/web/repository"
	"github.com/spf13/cobra"
)

var (
	dnsSyncDomain   string
	dnsSyncAll      bool
	dnsSyncApply    bool
	dnsSyncProvider string
	dnsSyncToken    string
	dnsSyncEmail    string
	dnsSyncAuthMode string
)

var dnsSyncCmd = &cobra.Command{
	Use:   "dns-sync",
	Short: "Check and sync recommended DNS records (SPF, DKIM, DMARC)",
	Long: `Compare current DNS records against Sendry recommendations for a domain
(SPF, DKIM, DMARC) and, with --apply, create or update them via a DNS provider.

Currently supported provider: cloudflare.

Cloudflare authentication (one of):
  - API Token (recommended): --token or CLOUDFLARE_API_TOKEN.
    Required permissions: Zone:Read, DNS:Edit.
  - Global API Key (legacy): --email and --token (key) or
    CLOUDFLARE_API_EMAIL + CLOUDFLARE_API_KEY. Use --auth global to force this
    mode; otherwise it is selected automatically when email is provided.`,
	RunE: runDNSSync,
}

func init() {
	dnsSyncCmd.Flags().StringVarP(&configFile, "config", "c", "/etc/sendry/web.yaml", "Path to configuration file")
	dnsSyncCmd.Flags().StringVarP(&dnsSyncDomain, "domain", "d", "", "Domain to sync (by domain name)")
	dnsSyncCmd.Flags().BoolVar(&dnsSyncAll, "all", false, "Sync all domains")
	dnsSyncCmd.Flags().BoolVar(&dnsSyncApply, "apply", false, "Apply changes (default is plan only)")
	dnsSyncCmd.Flags().StringVar(&dnsSyncProvider, "provider", "cloudflare", "DNS provider (cloudflare)")
	dnsSyncCmd.Flags().StringVar(&dnsSyncToken, "token", "", "Provider API token or global key (overrides env)")
	dnsSyncCmd.Flags().StringVar(&dnsSyncEmail, "email", "", "Account email for legacy Cloudflare Global API Key (overrides env)")
	dnsSyncCmd.Flags().StringVar(&dnsSyncAuthMode, "auth", "auto", "Cloudflare auth mode: auto, token, global")
}

func runDNSSync(cmd *cobra.Command, args []string) error {
	if dnsSyncDomain == "" && !dnsSyncAll {
		return fmt.Errorf("either --domain or --all is required")
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	provider, err := buildProvider()
	if err != nil {
		return err
	}

	domainsRepo := repository.NewDomainRepository(database.DB)
	dkimRepo := repository.NewDKIMRepository(database.DB)
	settingsRepo := repository.NewSettingsRepository(database.DB)

	globalVars, err := settingsRepo.GetGlobalVariablesMap()
	if err != nil {
		return fmt.Errorf("load global variables: %w", err)
	}
	spfInclude := strings.TrimSpace(globalVars["spf_include"])

	domains, err := loadDomains(domainsRepo, dkimRepo, dnsSyncDomain, dnsSyncAll)
	if err != nil {
		return err
	}
	if len(domains) == 0 {
		return fmt.Errorf("no domains found")
	}

	syncer := &dnssync.Syncer{Provider: provider}
	ctx := context.Background()

	mode := "plan"
	if dnsSyncApply {
		mode = "apply"
	}
	fmt.Printf("DNS sync [%s] provider=%s domains=%d\n", mode, provider.Name(), len(domains))

	hadErrors := false
	for _, d := range domains {
		fmt.Printf("\n=== %s ===\n", d.Domain)
		entries := dnssync.BuildRecommended(d, spfInclude)

		var results []dnssync.SyncResult
		if dnsSyncApply {
			results, err = syncer.Apply(ctx, entries)
		} else {
			results, err = syncer.Plan(ctx, entries)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
			hadErrors = true
			continue
		}
		printResults(results)
		for _, r := range results {
			if r.Error != "" {
				hadErrors = true
			}
		}
	}

	if hadErrors {
		return fmt.Errorf("dns-sync finished with errors")
	}
	return nil
}

func buildProvider() (dnsprovider.Provider, error) {
	switch strings.ToLower(dnsSyncProvider) {
	case "cloudflare", "cf":
		return buildCloudflareProvider()
	default:
		return nil, fmt.Errorf("unsupported provider %q", dnsSyncProvider)
	}
}

func buildCloudflareProvider() (dnsprovider.Provider, error) {
	token := strings.TrimSpace(dnsSyncToken)
	email := strings.TrimSpace(dnsSyncEmail)

	if token == "" {
		if v := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")); v != "" {
			token = v
		} else if v := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_KEY")); v != "" {
			token = v
		}
	}
	if email == "" {
		email = strings.TrimSpace(os.Getenv("CLOUDFLARE_API_EMAIL"))
	}

	mode := strings.ToLower(strings.TrimSpace(dnsSyncAuthMode))
	if mode == "" {
		mode = "auto"
	}
	if mode == "auto" {
		if email != "" {
			mode = "global"
		} else {
			mode = "token"
		}
	}

	switch mode {
	case "token":
		if token == "" {
			return nil, fmt.Errorf("cloudflare API token is required: use --token or CLOUDFLARE_API_TOKEN")
		}
		return dnsprovider.NewCloudflare(token), nil
	case "global", "global-key", "globalkey":
		if email == "" {
			return nil, fmt.Errorf("cloudflare global key requires --email or CLOUDFLARE_API_EMAIL")
		}
		if token == "" {
			return nil, fmt.Errorf("cloudflare global key requires --token/CLOUDFLARE_API_KEY (the global key value)")
		}
		return dnsprovider.NewCloudflareGlobalKey(email, token), nil
	default:
		return nil, fmt.Errorf("unsupported cloudflare auth mode %q (use auto|token|global)", mode)
	}
}

func loadDomains(domainsRepo *repository.DomainRepository, dkimRepo *repository.DKIMRepository, domainName string, all bool) ([]*models.Domain, error) {
	var result []*models.Domain
	if all {
		list, err := domainsRepo.List(models.DomainFilter{})
		if err != nil {
			return nil, err
		}
		for _, item := range list {
			d, err := domainsRepo.GetByID(item.ID)
			if err != nil {
				return nil, err
			}
			if d == nil {
				continue
			}
			attachDKIM(d, dkimRepo)
			result = append(result, d)
		}
		return result, nil
	}

	d, err := domainsRepo.GetByDomain(domainName)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, fmt.Errorf("domain %q not found", domainName)
	}
	attachDKIM(d, dkimRepo)
	return []*models.Domain{d}, nil
}

func attachDKIM(d *models.Domain, dkimRepo *repository.DKIMRepository) {
	if d.DKIMKeyID == "" {
		return
	}
	key, err := dkimRepo.GetByID(d.DKIMKeyID)
	if err == nil && key != nil {
		d.DKIMKey = key
	}
}

func printResults(results []dnssync.SyncResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tNAME\tACTION\tSTATUS\tDETAILS")
	for _, r := range results {
		status := "planned"
		if r.Applied {
			status = "applied"
		}
		if r.Error != "" {
			status = "error"
		}
		details := r.Reason
		if r.Error != "" {
			details = r.Error
		}
		name := r.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Kind, name, r.Action, status, details)
	}
	w.Flush()
}
