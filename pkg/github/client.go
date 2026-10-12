package github

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"

	"github.com/indika/drift-to-git/pkg/config"
	"github.com/indika/drift-to-git/pkg/diff"
	"github.com/indika/drift-to-git/pkg/kustomize"
)

// Client handles GitHub interactions using a Personal Access Token
type Client struct {
	ghClient *github.Client
	cfg      config.GitHubConfig
}

// NewClient creates a new GitHub client
func NewClient(cfg config.GitHubConfig) *Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.Token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	return &Client{
		ghClient: github.NewClient(tc),
		cfg:      cfg,
	}
}

// ParseRepoURL extracts owner and repo from HTTPS or SSH GitHub URLs
func ParseRepoURL(rawURL string) (owner, repo string, err error) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimSuffix(rawURL, ".git")

	if strings.HasPrefix(rawURL, "git@github.com:") {
		trimmed := strings.TrimPrefix(rawURL, "git@github.com:")
		parts := strings.Split(trimmed, "/")
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
		return "", "", fmt.Errorf("invalid SSH GitHub URL format: %s", rawURL)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parsing URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("cannot parse owner/repo from URL: %s", rawURL)
}

// HasActivePR checks if there's already an open PR for this application's drift to ensure idempotency
func (c *Client) HasActivePR(ctx context.Context, owner, repo, appName string) (bool, string, error) {
	opts := &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
	}

	prs, _, err := c.ghClient.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return false, "", fmt.Errorf("listing open PRs: %w", err)
	}

	expectedPrefix := fmt.Sprintf("%s/%s", c.cfg.BranchPrefix, appName)
	for _, pr := range prs {
		headRef := pr.GetHead().GetRef()
		if strings.HasPrefix(headRef, expectedPrefix) {
			return true, pr.GetHTMLURL(), nil
		}
		for _, l := range pr.Labels {
			if l.GetName() == fmt.Sprintf("drift-%s", appName) {
				return true, pr.GetHTMLURL(), nil
			}
		}
	}

	return false, "", nil
}

// CreateDriftPRParams contains data needed to open a reconciliation PR
type CreateDriftPRParams struct {
	AppName     string
	RepoURL     string
	AppPath     string
	BaseBranch  string
	Patches     []*diff.DriftPatch
	DiffSummary string
}

// CreateDriftPR creates a new branch, commits the patches + updated kustomization.yaml, and opens a PR
func (c *Client) CreateDriftPR(ctx context.Context, params CreateDriftPRParams) (*github.PullRequest, error) {
	owner, repo, err := ParseRepoURL(params.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("parsing repo URL: %w", err)
	}

	baseBranch := params.BaseBranch
	if baseBranch == "" || baseBranch == "HEAD" {
		baseBranch = c.cfg.DefaultBranch
	}

	// 1. Check idempotency: is there already an active PR?
	active, prURL, err := c.HasActivePR(ctx, owner, repo, params.AppName)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("active PR already exists for application %s: %s", params.AppName, prURL)
	}

	// 2. Get base branch commit SHA
	baseRef, _, err := c.ghClient.Git.GetRef(ctx, owner, repo, "heads/"+baseBranch)
	if err != nil {
		return nil, fmt.Errorf("getting ref heads/%s: %w", baseBranch, err)
	}
	baseSHA := baseRef.Object.GetSHA()

	baseCommit, _, err := c.ghClient.Git.GetCommit(ctx, owner, repo, baseSHA)
	if err != nil {
		return nil, fmt.Errorf("getting base commit %s: %w", baseSHA, err)
	}

	// 3. Prepare entries for the new Git tree
	var treeEntries []*github.TreeEntry

	// Read existing kustomization.yaml if it exists
	kustomizationPath := path.Join(params.AppPath, "kustomization.yaml")
	existingKustomizeContent := ""

	fileContent, _, resp, err := c.ghClient.Repositories.GetContents(ctx, owner, repo, kustomizationPath, &github.RepositoryContentGetOptions{
		Ref: baseSHA,
	})
	if err == nil && fileContent != nil {
		contentStr, decErr := fileContent.GetContent()
		if decErr == nil {
			existingKustomizeContent = contentStr
		}
	} else if resp != nil && resp.StatusCode == 404 {
		// Also try kustomization.yml
		kustomizationPathYml := path.Join(params.AppPath, "kustomization.yml")
		fileContentYml, _, _, errYml := c.ghClient.Repositories.GetContents(ctx, owner, repo, kustomizationPathYml, &github.RepositoryContentGetOptions{
			Ref: baseSHA,
		})
		if errYml == nil && fileContentYml != nil {
			contentStr, decErr := fileContentYml.GetContent()
			if decErr == nil {
				existingKustomizeContent = contentStr
				kustomizationPath = kustomizationPathYml
			}
		}
	}

	// For each patch, add it as a file and register it in kustomization.yaml
	updatedKustomizeContent := existingKustomizeContent
	patchDir := c.cfg.PatchDirectory
	if patchDir == "" {
		patchDir = "patches"
	}

	for _, p := range params.Patches {
		if !p.HasDrift {
			continue
		}

		patchFileName := fmt.Sprintf("drift-%s-%s.yaml", strings.ToLower(p.Kind), strings.ToLower(p.Name))
		patchRelativePath := path.Join(patchDir, patchFileName)
		patchRepoPath := path.Join(params.AppPath, patchRelativePath)

		// Tree entry for patch file
		pContent := p.PatchYAML
		treeEntries = append(treeEntries, &github.TreeEntry{
			Path:    github.String(patchRepoPath),
			Mode:    github.String("100644"),
			Type:    github.String("blob"),
			Content: github.String(pContent),
		})

		// Update kustomization.yaml
		updated, kErr := kustomize.AddPatchToKustomization(updatedKustomizeContent, patchRelativePath)
		if kErr != nil {
			return nil, fmt.Errorf("updating kustomization.yaml with patch %s: %w", patchRelativePath, kErr)
		}
		updatedKustomizeContent = updated
	}

	// Tree entry for updated kustomization.yaml
	treeEntries = append(treeEntries, &github.TreeEntry{
		Path:    github.String(kustomizationPath),
		Mode:    github.String("100644"),
		Type:    github.String("blob"),
		Content: github.String(updatedKustomizeContent),
	})

	// 4. Create new Git tree
	newTree, _, err := c.ghClient.Git.CreateTree(ctx, owner, repo, baseCommit.GetTree().GetSHA(), treeEntries)
	if err != nil {
		return nil, fmt.Errorf("creating Git tree: %w", err)
	}

	// 5. Create new commit
	commitMsg := fmt.Sprintf("chore(drift): reconcile cluster changes for %s", params.AppName)
	newCommit, _, err := c.ghClient.Git.CreateCommit(ctx, owner, repo, &github.Commit{
		Message: github.String(commitMsg),
		Tree:    newTree,
		Parents: []*github.Commit{baseCommit},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Git commit: %w", err)
	}

	// 6. Create new branch
	timestamp := time.Now().UTC().Format("20060102-150405")
	branchName := fmt.Sprintf("%s/%s-%s", c.cfg.BranchPrefix, params.AppName, timestamp)
	refName := "refs/heads/" + branchName

	_, _, err = c.ghClient.Git.CreateRef(ctx, owner, repo, &github.Reference{
		Ref: github.String(refName),
		Object: &github.GitObject{
			SHA: newCommit.SHA,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating branch %s: %w", branchName, err)
	}

	// 7. Open Pull Request
	prTitle := fmt.Sprintf("%s Reconcile cluster drift in %s", c.cfg.PRTitlePrefix, params.AppName)
	prBody := fmt.Sprintf("### 🔄 Automated Cluster Drift Reconciliation\n\n"+
		"Argo CD detected live changes in the Kubernetes cluster that diverged from Git for application **`%s`**.\n\n"+
		"This PR reconciles those changes as clean Kustomize patches under `%s/%s/`.\n\n"+
		"%s\n\n"+
		"---\n*Generated automatically by [drift-to-git](https://github.com/indika/drift-to-git).* Merging this PR brings Git and cluster back into sync without wiping out live hotfixes.",
		params.AppName, params.AppPath, patchDir, params.DiffSummary,
	)

	newPR, _, err := c.ghClient.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.String(prTitle),
		Head:  github.String(branchName),
		Base:  github.String(baseBranch),
		Body:  github.String(prBody),
	})
	if err != nil {
		return nil, fmt.Errorf("creating pull request: %w", err)
	}

	// 8. Add labels
	labels := append(c.cfg.PRLabels, fmt.Sprintf("drift-%s", params.AppName))
	_, _, _ = c.ghClient.Issues.AddLabelsToIssue(ctx, owner, repo, newPR.GetNumber(), labels)

	return newPR, nil
}
