package argocd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/indika/drift-to-git/pkg/config"
)

// Client interacts with the Argo CD REST API
type Client struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewClient creates a new Argo CD API client
func NewClient(cfg config.ArgoCDConfig) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
		},
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL:   strings.TrimRight(cfg.ServerURL, "/"),
		authToken: cfg.AuthToken,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   timeout,
		},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
	}
	return req, nil
}

// ListApplications retrieves all applications from Argo CD
func (c *Client) ListApplications(ctx context.Context) ([]Application, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/applications")
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request to Argo CD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d from Argo CD: %s", resp.StatusCode, string(body))
	}

	var appList ApplicationList
	if err := json.NewDecoder(resp.Body).Decode(&appList); err != nil {
		return nil, fmt.Errorf("decoding Argo CD applications response: %w", err)
	}

	return appList.Items, nil
}

// GetApplication retrieves a single application by name
func (c *Client) GetApplication(ctx context.Context, name string) (*Application, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/applications/%s", name))
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request to Argo CD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d from Argo CD: %s", resp.StatusCode, string(body))
	}

	var app Application
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("decoding Argo CD application response: %w", err)
	}

	return &app, nil
}

// GetManagedResources retrieves the managed resources and live/target states for an application
func (c *Client) GetManagedResources(ctx context.Context, name string) ([]ManagedResourceItem, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/applications/%s/managed-resources", name))
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request for managed resources: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d from Argo CD managed-resources: %s", resp.StatusCode, string(body))
	}

	var managedResp ManagedResourcesResponse
	if err := json.NewDecoder(resp.Body).Decode(&managedResp); err != nil {
		return nil, fmt.Errorf("decoding managed resources: %w", err)
	}

	return managedResp.Items, nil
}

// GetOutOfSyncApplications returns all applications currently marked as OutOfSync, filtered by appFilter if specified
func (c *Client) GetOutOfSyncApplications(ctx context.Context, appFilter []string) ([]Application, error) {
	apps, err := c.ListApplications(ctx)
	if err != nil {
		return nil, err
	}

	filterMap := make(map[string]bool)
	for _, f := range appFilter {
		filterMap[f] = true
	}

	var outOfSync []Application
	for _, app := range apps {
		if len(filterMap) > 0 && !filterMap[app.Metadata.Name] {
			continue
		}

		if app.Status.Sync.Status == "OutOfSync" {
			outOfSync = append(outOfSync, app)
		}
	}

	return outOfSync, nil
}
