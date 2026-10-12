package kustomize

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// AddPatchToKustomization adds a patch file path to an existing kustomization.yaml content.
// It is idempotent: if the patch is already present, it returns the content unchanged.
func AddPatchToKustomization(existingContent, patchRelativePath string) (string, error) {
	if strings.TrimSpace(existingContent) == "" {
		// New kustomization.yaml
		return fmt.Sprintf("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n  - path: %s\n", patchRelativePath), nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(existingContent), &root); err != nil {
		return "", fmt.Errorf("parsing kustomization.yaml: %w", err)
	}

	if len(root.Content) == 0 {
		return fmt.Sprintf("patches:\n  - path: %s\n", patchRelativePath), nil
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return "", fmt.Errorf("kustomization.yaml root is not a mapping")
	}

	// Look for "patches" or "patchesStrategicMerge"
	var patchesValueNode *yaml.Node
	var patchesStrategicValNode *yaml.Node

	for i := 0; i < len(doc.Content); i += 2 {
		key := doc.Content[i].Value
		if key == "patches" {
			patchesValueNode = doc.Content[i+1]
		} else if key == "patchesStrategicMerge" {
			patchesStrategicValNode = doc.Content[i+1]
		}
	}

	// 1. If "patches" exists
	if patchesValueNode != nil {
		if patchAlreadyInPatches(patchesValueNode, patchRelativePath) {
			return existingContent, nil
		}
		// Append to patches list: - path: patchRelativePath
		entryMap := &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "path"},
				{Kind: yaml.ScalarNode, Value: patchRelativePath},
			},
		}
		patchesValueNode.Content = append(patchesValueNode.Content, entryMap)
	} else if patchesStrategicValNode != nil {
		// 2. If legacy "patchesStrategicMerge" exists
		if patchAlreadyInStrategic(patchesStrategicValNode, patchRelativePath) {
			return existingContent, nil
		}
		entryScalar := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: patchRelativePath,
		}
		patchesStrategicValNode.Content = append(patchesStrategicValNode.Content, entryScalar)
	} else {
		// 3. Create new "patches" node
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "patches",
		}
		seqNode := &yaml.Node{
			Kind: yaml.SequenceNode,
			Content: []*yaml.Node{
				{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "path"},
						{Kind: yaml.ScalarNode, Value: patchRelativePath},
					},
				},
			},
		}
		doc.Content = append(doc.Content, keyNode, seqNode)
	}

	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return "", fmt.Errorf("encoding updated kustomization.yaml: %w", err)
	}

	return buf.String(), nil
}

func patchAlreadyInPatches(seqNode *yaml.Node, path string) bool {
	if seqNode.Kind != yaml.SequenceNode {
		return false
	}
	for _, item := range seqNode.Content {
		if item.Kind == yaml.MappingNode {
			for i := 0; i < len(item.Content); i += 2 {
				if item.Content[i].Value == "path" && item.Content[i+1].Value == path {
					return true
				}
			}
		} else if item.Kind == yaml.ScalarNode && item.Value == path {
			return true
		}
	}
	return false
}

func patchAlreadyInStrategic(seqNode *yaml.Node, path string) bool {
	if seqNode.Kind != yaml.SequenceNode {
		return false
	}
	for _, item := range seqNode.Content {
		if item.Kind == yaml.ScalarNode && item.Value == path {
			return true
		}
	}
	return false
}
