package kustomize

import (
	"strings"
	"testing"
)

func TestAddPatchToExistingKustomization(t *testing.T) {
	initialYAML := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - deployment.yaml
  - service.yaml
`

	patchPath := "patches/drift-deployment-payment.yaml"
	updated, err := AddPatchToKustomization(initialYAML, patchPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(updated, "patches:") {
		t.Errorf("expected 'patches:' block in output, got:\n%s", updated)
	}
	if !strings.Contains(updated, patchPath) {
		t.Errorf("expected patch path %s in output, got:\n%s", patchPath, updated)
	}

	// Test idempotency: adding the same patch again should not duplicate
	updatedSecondTime, err := AddPatchToKustomization(updated, patchPath)
	if err != nil {
		t.Fatalf("unexpected error on second add: %v", err)
	}

	occurrences := strings.Count(updatedSecondTime, patchPath)
	if occurrences != 1 {
		t.Errorf("expected exactly 1 occurrence of %s, got %d", patchPath, occurrences)
	}
}

func TestAddPatchToEmptyContent(t *testing.T) {
	patchPath := "patches/drift-patch.yaml"
	updated, err := AddPatchToKustomization("", patchPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(updated, patchPath) {
		t.Errorf("expected patch path %s, got:\n%s", patchPath, updated)
	}
}
