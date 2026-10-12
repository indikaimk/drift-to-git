package reconciler

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/indika/drift-to-git/pkg/argocd"
	"github.com/indika/drift-to-git/pkg/config"
	"github.com/indika/drift-to-git/pkg/diff"
	"github.com/indika/drift-to-git/pkg/github"
)

// Engine coordinates the drift detection and Git PR reconciliation workflow
type Engine struct {
	cfg        *config.Config
	argoClient *argocd.Client
	ghClient   *github.Client
}

// NewEngine creates a new reconciliation engine
func NewEngine(cfg *config.Config) *Engine {
	var ghClient *github.Client
	if cfg.GitHub.Token != "" {
		ghClient = github.NewClient(cfg.GitHub)
	}

	return &Engine{
		cfg:        cfg,
		argoClient: argocd.NewClient(cfg.ArgoCD),
		ghClient:   ghClient,
	}
}

// ReconcileResult summarizes the outcome of a reconciliation cycle
type ReconcileResult struct {
	AppsChecked  int
	AppsDrifted  int
	PRsCreated   []string
	SkippedDueTo []string
}

// RunOnce performs a single pass over all target Argo CD applications
func (e *Engine) RunOnce(ctx context.Context) (*ReconcileResult, error) {
	result := &ReconcileResult{}

	log.Println("🔍 Checking Argo CD for OutOfSync applications...")
	apps, err := e.argoClient.GetOutOfSyncApplications(ctx, e.cfg.ArgoCD.AppFilter)
	if err != nil {
		return nil, fmt.Errorf("retrieving OutOfSync apps: %w", err)
	}

	result.AppsChecked = len(apps)
	if len(apps) == 0 {
		log.Println("✅ No OutOfSync applications found. All applications are synchronized.")
		return result, nil
	}

	log.Printf("⚠️ Found %d OutOfSync application(s). Analyzing drift...\n", len(apps))

	prsCreatedCount := 0

	for _, app := range apps {
		appName := app.Metadata.Name

		// Check if explicitly disabled via annotation
		if app.Metadata.Annotations != nil && app.Metadata.Annotations["drift-to-git.io/enabled"] == "false" {
			log.Printf("⏩ Skipping app %s (drift-to-git disabled via annotation)\n", appName)
			result.SkippedDueTo = append(result.SkippedDueTo, fmt.Sprintf("%s (disabled by annotation)", appName))
			continue
		}

		// Ensure we don't exceed max PR limit per run
		if prsCreatedCount >= e.cfg.Reconciler.MaxPRsPerRun {
			log.Printf("⏸️ Reached maximum PR limit (%d) for this cycle. Remaining apps will be processed in next run.\n", e.cfg.Reconciler.MaxPRsPerRun)
			break
		}

		managedResources, err := e.argoClient.GetManagedResources(ctx, appName)
		if err != nil {
			log.Printf("❌ Error fetching managed resources for app %s: %v\n", appName, err)
			continue
		}

		var appPatches []*diff.DriftPatch
		var diffSummaries strings.Builder

		for _, res := range managedResources {
			if res.LiveState == "" || res.TargetState == "" {
				continue
			}

			patch, err := diff.ComputeStrategicMergePatch(
				res.Group, res.Version, res.Kind, res.Namespace, res.Name,
				res.TargetState, res.LiveState,
			)
			if err != nil {
				log.Printf("⚠️ Could not compute diff for %s/%s in app %s: %v\n", res.Kind, res.Name, appName, err)
				continue
			}

			if patch.HasDrift {
				appPatches = append(appPatches, patch)
				diffSummaries.WriteString(patch.DiffSummary)
				log.Printf("⚡ Drift detected in %s [%s/%s]: %v\n", appName, patch.Kind, patch.Name, patch.ChangedPaths)
			}
		}

		if len(appPatches) == 0 {
			log.Printf("ℹ️ App %s is marked OutOfSync, but changes are runtime noise (filtered).\n", appName)
			continue
		}

		result.AppsDrifted++

		if e.cfg.Reconciler.DryRun {
			log.Printf("🧪 [DRY RUN] Would create PR for app %s with %d patch(es):\n", appName, len(appPatches))
			for _, p := range appPatches {
				log.Printf("--- Patch for %s/%s ---\n%s\n", p.Kind, p.Name, p.PatchYAML)
			}
			continue
		}

		if e.ghClient == nil {
			log.Printf("❌ Cannot create PR for app %s: GITHUB_TOKEN is not configured.\n", appName)
			result.SkippedDueTo = append(result.SkippedDueTo, fmt.Sprintf("%s (missing GITHUB_TOKEN)", appName))
			continue
		}

		// Open Pull Request via GitHub
		log.Printf("🚀 Generating Kustomize patch and opening PR for %s...\n", appName)
		pr, err := e.ghClient.CreateDriftPR(ctx, github.CreateDriftPRParams{
			AppName:     appName,
			RepoURL:     app.Spec.Source.RepoURL,
			AppPath:     app.Spec.Source.Path,
			BaseBranch:  app.Spec.Source.TargetRevision,
			Patches:     appPatches,
			DiffSummary: diffSummaries.String(),
		})
		if err != nil {
			log.Printf("⚠️ Could not create PR for %s: %v\n", appName, err)
			result.SkippedDueTo = append(result.SkippedDueTo, fmt.Sprintf("%s (%v)", appName, err))
			continue
		}

		log.Printf("🎉 Successfully opened PR for %s: %s\n", appName, pr.GetHTMLURL())
		result.PRsCreated = append(result.PRsCreated, pr.GetHTMLURL())
		prsCreatedCount++
	}

	return result, nil
}
