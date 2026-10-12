package github

import (
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS with .git",
			rawURL:    "https://github.com/myorg/gitops-repo.git",
			wantOwner: "myorg",
			wantRepo:  "gitops-repo",
		},
		{
			name:      "HTTPS without .git",
			rawURL:    "https://github.com/myorg/gitops-repo",
			wantOwner: "myorg",
			wantRepo:  "gitops-repo",
		},
		{
			name:      "SSH with .git",
			rawURL:    "git@github.com:myorg/gitops-repo.git",
			wantOwner: "myorg",
			wantRepo:  "gitops-repo",
		},
		{
			name:      "SSH without .git",
			rawURL:    "git@github.com:myorg/gitops-repo",
			wantOwner: "myorg",
			wantRepo:  "gitops-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseRepoURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRepoURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("ParseRepoURL() = (%s, %s), want (%s, %s)", owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}
