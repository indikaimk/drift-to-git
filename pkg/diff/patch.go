package diff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// DriftPatch represents a computed patch and metadata
type DriftPatch struct {
	Group        string
	Version      string
	Kind         string
	Namespace    string
	Name         string
	PatchYAML    string
	PatchMap     map[string]any
	DiffSummary  string
	HasDrift     bool
	ChangedPaths []string
}

// ComputeStrategicMergePatch compares desired (target) and live states, returning a clean Kustomize patch
func ComputeStrategicMergePatch(group, version, kind, namespace, name, targetJSON, liveJSON string) (*DriftPatch, error) {
	patch := &DriftPatch{
		Group:     group,
		Version:   version,
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
	}

	var targetMap map[string]any
	if targetJSON != "" && targetJSON != "null" {
		if err := json.Unmarshal([]byte(targetJSON), &targetMap); err != nil {
			return nil, fmt.Errorf("parsing target JSON: %w", err)
		}
	}

	var liveMap map[string]any
	if liveJSON != "" && liveJSON != "null" {
		if err := json.Unmarshal([]byte(liveJSON), &liveMap); err != nil {
			return nil, fmt.Errorf("parsing live JSON: %w", err)
		}
	}

	// Filter noise out of live and target
	cleanedLive := CleanLiveObject(liveMap)
	cleanedTarget := CleanLiveObject(targetMap)

	// If no live map exists, resource was deleted
	if len(cleanedLive) == 0 {
		patch.HasDrift = false
		return patch, nil
	}

	// Determine apiVersion
	apiVersion := ""
	if v, ok := cleanedLive["apiVersion"].(string); ok && v != "" {
		apiVersion = v
	} else if v, ok := cleanedTarget["apiVersion"].(string); ok && v != "" {
		apiVersion = v
	} else {
		if group != "" {
			apiVersion = fmt.Sprintf("%s/%s", group, version)
		} else {
			apiVersion = version
		}
	}

	// Extract deltas between target and live
	deltaMap, changedPaths := computeDelta(cleanedTarget, cleanedLive, "")
	if len(deltaMap) == 0 {
		patch.HasDrift = false
		return patch, nil
	}

	patch.HasDrift = true
	patch.ChangedPaths = changedPaths

	// Build the minimal strategic merge patch structure
	patchObj := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name": name,
		},
	}
	if namespace != "" {
		patchObj["metadata"].(map[string]any)["namespace"] = namespace
	}

	// Merge the deltas into patchObj
	for k, v := range deltaMap {
		if k == "apiVersion" || k == "kind" {
			continue
		}
		if k == "metadata" {
			if metaDelta, ok := v.(map[string]any); ok {
				for mk, mv := range metaDelta {
					patchObj["metadata"].(map[string]any)[mk] = mv
				}
			}
			continue
		}
		patchObj[k] = v
	}

	patch.PatchMap = patchObj

	// Convert patchObj to YAML
	yamlBytes, err := yaml.Marshal(patchObj)
	if err != nil {
		return nil, fmt.Errorf("marshaling patch to YAML: %w", err)
	}
	patch.PatchYAML = string(yamlBytes)

	// Generate a human-readable diff summary
	patch.DiffSummary = formatDiffSummary(kind, name, changedPaths, cleanedTarget, cleanedLive)

	return patch, nil
}

// computeDelta recursively finds values in `live` that differ from `target`
func computeDelta(target, live map[string]any, prefix string) (map[string]any, []string) {
	delta := make(map[string]any)
	var paths []string

	for k, liveVal := range live {
		currPath := k
		if prefix != "" {
			currPath = fmt.Sprintf("%s.%s", prefix, k)
		}

		targetVal, existsInTarget := target[k]
		if !existsInTarget {
			// Field was added in live cluster
			delta[k] = liveVal
			paths = append(paths, fmt.Sprintf("+ %s", currPath))
			continue
		}

		// Both exist: check if deeply equal
		if reflect.DeepEqual(targetVal, liveVal) {
			continue
		}

		// If both are maps, recurse
		targetSubMap, targetIsMap := targetVal.(map[string]any)
		liveSubMap, liveIsMap := liveVal.(map[string]any)
		if targetIsMap && liveIsMap {
			subDelta, subPaths := computeDelta(targetSubMap, liveSubMap, currPath)
			if len(subDelta) > 0 {
				delta[k] = subDelta
				paths = append(paths, subPaths...)
			}
			continue
		}

		// Otherwise, scalar or slice changed
		delta[k] = liveVal
		paths = append(paths, fmt.Sprintf("~ %s", currPath))
	}

	return delta, paths
}

func formatDiffSummary(kind, name string, changedPaths []string, target, live map[string]any) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Resource: `%s/%s`\n\n", kind, name))
	sb.WriteString("**Changed Fields:**\n")
	for _, p := range changedPaths {
		sb.WriteString(fmt.Sprintf("- `%s`\n", p))
	}
	sb.WriteString("\n")
	return sb.String()
}
