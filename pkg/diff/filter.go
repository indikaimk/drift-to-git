package diff

import (
	"encoding/json"
	"strings"
)

var ignoredMetadataKeys = map[string]bool{
	"managedFields":     true,
	"resourceVersion":   true,
	"uid":               true,
	"generation":        true,
	"creationTimestamp": true,
	"selfLink":          true,
}

var ignoredAnnotationPrefixes = []string{
	"kubectl.kubernetes.io/",
	"deployment.kubernetes.io/",
	"control-plane.alpha.kubernetes.io/",
	"argocd.argoproj.io/",
}

// CleanLiveObject cleans runtime noise from a Kubernetes object map
func CleanLiveObject(obj map[string]any) map[string]any {
	cleaned := deepCopyMap(obj)

	// Remove status block completely
	delete(cleaned, "status")

	// Clean metadata
	if metaVal, ok := cleaned["metadata"]; ok {
		if metaMap, ok := metaVal.(map[string]any); ok {
			cleanMetadata(metaMap)
			if len(metaMap) == 0 {
				delete(cleaned, "metadata")
			}
		}
	}

	return cleaned
}

func cleanMetadata(meta map[string]any) {
	for k := range ignoredMetadataKeys {
		delete(meta, k)
	}

	// Filter annotations
	if annVal, ok := meta["annotations"]; ok {
		if annMap, ok := annVal.(map[string]any); ok {
			for annKey := range annMap {
				for _, prefix := range ignoredAnnotationPrefixes {
					if strings.HasPrefix(annKey, prefix) {
						delete(annMap, annKey)
						break
					}
				}
			}
			if len(annMap) == 0 {
				delete(meta, "annotations")
			}
		}
	}
}

func deepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	bytes, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst map[string]any
	_ = json.Unmarshal(bytes, &dst)
	return dst
}
