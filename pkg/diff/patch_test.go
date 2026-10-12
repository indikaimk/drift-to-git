package diff

import (
	"strings"
	"testing"
)

func TestComputeStrategicMergePatch(t *testing.T) {
	targetJSON := `{
		"apiVersion": "apps/v1",
		"kind": "Deployment",
		"metadata": {
			"name": "payment-api",
			"namespace": "production"
		},
		"spec": {
			"replicas": 2,
			"template": {
				"spec": {
					"containers": [
						{
							"name": "payment",
							"image": "myorg/payment:1.0.0"
						}
					]
				}
			}
		}
	}`

	liveJSON := `{
		"apiVersion": "apps/v1",
		"kind": "Deployment",
		"metadata": {
			"name": "payment-api",
			"namespace": "production",
			"resourceVersion": "123456",
			"uid": "abc-123",
			"generation": 3,
			"creationTimestamp": "2026-10-12T10:00:00Z",
			"managedFields": [{"manager": "kubectl-edit"}],
			"annotations": {
				"kubectl.kubernetes.io/last-applied-configuration": "{...}"
			}
		},
		"spec": {
			"replicas": 5,
			"template": {
				"spec": {
					"containers": [
						{
							"name": "payment",
							"image": "myorg/payment:1.0.0",
							"resources": {
								"limits": {
									"memory": "4Gi"
								}
							}
						}
					]
				}
			}
		},
		"status": {
			"readyReplicas": 5,
			"replicas": 5
		}
	}`

	patch, err := ComputeStrategicMergePatch(
		"apps", "v1", "Deployment", "production", "payment-api",
		targetJSON, liveJSON,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !patch.HasDrift {
		t.Fatalf("expected drift to be detected, but got false")
	}

	// Verify runtime noise was stripped
	if strings.Contains(patch.PatchYAML, "resourceVersion") {
		t.Errorf("patch should not contain resourceVersion")
	}
	if strings.Contains(patch.PatchYAML, "managedFields") {
		t.Errorf("patch should not contain managedFields")
	}
	if strings.Contains(patch.PatchYAML, "readyReplicas") || strings.Contains(patch.PatchYAML, "status:") {
		t.Errorf("patch should not contain status fields")
	}

	// Verify drifted fields are present in patch
	if !strings.Contains(patch.PatchYAML, "replicas: 5") {
		t.Errorf("patch should contain 'replicas: 5', got:\n%s", patch.PatchYAML)
	}
	if !strings.Contains(patch.PatchYAML, "4Gi") {
		t.Errorf("patch should contain memory limit 4Gi, got:\n%s", patch.PatchYAML)
	}

	t.Logf("Generated Patch YAML:\n%s", patch.PatchYAML)
}

func TestNoDriftWhenIdentical(t *testing.T) {
	manifest := `{
		"apiVersion": "v1",
		"kind": "ConfigMap",
		"metadata": {"name": "app-config", "namespace": "default"},
		"data": {"KEY": "value"}
	}`

	patch, err := ComputeStrategicMergePatch("", "v1", "ConfigMap", "default", "app-config", manifest, manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patch.HasDrift {
		t.Errorf("expected no drift, but got true")
	}
}
