package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration options for drift-to-git
type Config struct {
	ArgoCD     ArgoCDConfig
	GitHub     GitHubConfig
	Reconciler ReconcilerConfig
}

// ArgoCDConfig holds Argo CD connection settings
type ArgoCDConfig struct {
	ServerURL string        // e.g. "https://argocd-server.argocd.svc:443" or "http://localhost:8080"
	AuthToken string        // Argo CD JWT token or API token
	Insecure  bool          // Skip TLS cert verification
	Timeout   time.Duration // API request timeout
	AppFilter []string      // Optional list of specific application names to monitor (empty means all)
}

// GitHubConfig holds GitHub credentials and repository settings
type GitHubConfig struct {
	Token          string   // GitHub Personal Access Token (PAT)
	PRTitlePrefix  string   // Default: "[drift-to-git]"
	PRLabels       []string // Labels added to created PRs
	DefaultBranch  string   // Fallback branch if not specified in Argo Application
	BranchPrefix   string   // Default: "drift-to-git"
	PatchDirectory string   // Subdirectory within app path for patch files, e.g. "patches"
}

// ReconcilerConfig controls the execution loop
type ReconcilerConfig struct {
	PollInterval time.Duration // Interval between scan iterations
	DryRun       bool          // If true, detect drift and show patches without opening PRs
	MaxPRsPerRun int           // Limit PR creation per loop to prevent runaway bursts
}

// LoadFromEnv loads configuration from environment variables with fallback defaults
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		ArgoCD: ArgoCDConfig{
			ServerURL: getEnv("ARGOCD_SERVER", "http://localhost:8080"),
			AuthToken: getEnv("ARGOCD_AUTH_TOKEN", ""),
			Insecure:  getEnvBool("ARGOCD_INSECURE", true),
			Timeout:   getEnvDuration("ARGOCD_TIMEOUT", 30*time.Second),
			AppFilter: splitCsv(getEnv("ARGOCD_APP_FILTER", "")),
		},
		GitHub: GitHubConfig{
			Token:          getEnv("GITHUB_TOKEN", ""),
			PRTitlePrefix:  getEnv("GITHUB_PR_PREFIX", "[drift-to-git]"),
			PRLabels:       splitCsv(getEnv("GITHUB_PR_LABELS", "drift-to-git,automated-pr")),
			DefaultBranch:  getEnv("GITHUB_DEFAULT_BRANCH", "main"),
			BranchPrefix:   getEnv("GITHUB_BRANCH_PREFIX", "drift-to-git"),
			PatchDirectory: getEnv("PATCH_DIR", "patches"),
		},
		Reconciler: ReconcilerConfig{
			PollInterval: getEnvDuration("POLL_INTERVAL", 60*time.Second),
			DryRun:       getEnvBool("DRY_RUN", false),
			MaxPRsPerRun: getEnvInt("MAX_PRS_PER_RUN", 5),
		},
	}

	return cfg, nil
}

// Validate ensures required fields are populated
func (c *Config) Validate(requireGitHub bool) error {
	if c.ArgoCD.ServerURL == "" {
		return fmt.Errorf("ARGOCD_SERVER must not be empty")
	}
	if requireGitHub && c.GitHub.Token == "" {
		return fmt.Errorf("GITHUB_TOKEN must not be empty when running in live PR mode")
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(val)
		if err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		i, err := strconv.Atoi(val)
		if err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		d, err := time.ParseDuration(val)
		if err == nil {
			return d
		}
	}
	return defaultVal
}

func splitCsv(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
