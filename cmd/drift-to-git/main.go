package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/indika/drift-to-git/pkg/config"
	"github.com/indika/drift-to-git/pkg/reconciler"
)

var (
	version = "v0.1.0"

	// CLI Flags
	argocdServer string
	argocdToken  string
	githubToken  string
	dryRun       bool
	pollInterval time.Duration
	appFilter    string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "drift-to-git",
		Short: "drift-to-git reconciles Kubernetes cluster drift back to Git via Kustomize Pull Requests",
		Long: `drift-to-git monitors Argo CD for OutOfSync applications, extracts runtime cluster mutations, 
generates clean Kustomize strategic merge patches, and opens GitHub Pull Requests to reconcile Git with live state.`,
	}

	rootCmd.PersistentFlags().StringVar(&argocdServer, "argocd-server", "", "Argo CD server URL (env: ARGOCD_SERVER)")
	rootCmd.PersistentFlags().StringVar(&argocdToken, "argocd-token", "", "Argo CD auth token (env: ARGOCD_AUTH_TOKEN)")
	rootCmd.PersistentFlags().StringVar(&githubToken, "github-token", "", "GitHub Personal Access Token (env: GITHUB_TOKEN)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Detect drift and show patches without creating GitHub PRs")
	rootCmd.PersistentFlags().StringVar(&appFilter, "app", "", "Filter by specific application name")

	// scan command
	scanCmd := &cobra.Command{
		Use:   "scan",
		Short: "Run a single drift detection and PR reconciliation scan",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if err := cfg.Validate(!cfg.Reconciler.DryRun); err != nil {
				return err
			}

			engine := reconciler.NewEngine(cfg)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			res, err := engine.RunOnce(ctx)
			if err != nil {
				return err
			}

			log.Printf("Scan completed. OutOfSync apps: %d, PRs created: %d\n", res.AppsDrifted, len(res.PRsCreated))
			return nil
		},
	}

	// run / operator command
	runCmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"operator", "controller"},
		Short:   "Run continuous reconciliation loop in-cluster or as a service",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if err := cfg.Validate(!cfg.Reconciler.DryRun); err != nil {
				return err
			}

			engine := reconciler.NewEngine(cfg)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			log.Printf("Starting drift-to-git controller (interval: %v, dry-run: %v)...\n", cfg.Reconciler.PollInterval, cfg.Reconciler.DryRun)

			ticker := time.NewTicker(cfg.Reconciler.PollInterval)
			defer ticker.Stop()

			// Run immediate pass on startup
			if _, err := engine.RunOnce(ctx); err != nil {
				log.Printf("Error during reconciliation pass: %v\n", err)
			}

			for {
				select {
				case <-ctx.Done():
					log.Println("Shutting down drift-to-git gracefully...")
					return nil
				case <-ticker.C:
					if _, err := engine.RunOnce(ctx); err != nil {
						log.Printf("Error during reconciliation pass: %v\n", err)
					}
				}
			}
		},
	}
	runCmd.Flags().DurationVar(&pollInterval, "poll-interval", 0, "Polling interval between drift scans (e.g. 30s, 1m)")

	// version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("drift-to-git %s\n", version)
		},
	}

	rootCmd.AddCommand(scanCmd, runCmd, versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, err
	}

	if argocdServer != "" {
		cfg.ArgoCD.ServerURL = argocdServer
	}
	if argocdToken != "" {
		cfg.ArgoCD.AuthToken = argocdToken
	}
	if githubToken != "" {
		cfg.GitHub.Token = githubToken
	}
	if dryRun {
		cfg.Reconciler.DryRun = true
	}
	if pollInterval > 0 {
		cfg.Reconciler.PollInterval = pollInterval
	}
	if appFilter != "" {
		cfg.ArgoCD.AppFilter = []string{appFilter}
	}

	return cfg, nil
}
