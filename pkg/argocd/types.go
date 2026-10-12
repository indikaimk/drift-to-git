package argocd

// ApplicationList represents a list of Argo CD Application resources
type ApplicationList struct {
	Items []Application `json:"items"`
}

// Application represents an Argo CD Application
type Application struct {
	Metadata ObjectMeta        `json:"metadata"`
	Spec     ApplicationSpec   `json:"spec"`
	Status   ApplicationStatus `json:"status"`
}

// ObjectMeta contains metadata about an object
type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
	Source      ApplicationSource      `json:"source"`
	Destination ApplicationDestination `json:"destination"`
}

// ApplicationSource contains information about the source of an application
type ApplicationSource struct {
	RepoURL        string `json:"repoURL"`
	Path           string `json:"path"`
	TargetRevision string `json:"targetRevision"`
}

// ApplicationDestination contains target cluster/namespace
type ApplicationDestination struct {
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
}

// ApplicationStatus contains status information for an Application
type ApplicationStatus struct {
	Sync      SyncStatus       `json:"sync"`
	Health    HealthStatus     `json:"health"`
	Resources []ResourceStatus `json:"resources,omitempty"`
}

// SyncStatus contains information about the sync state of the application
type SyncStatus struct {
	Status   string `json:"status"` // "Synced", "OutOfSync", "Unknown"
	Revision string `json:"revision"`
}

// HealthStatus contains health assessment
type HealthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ResourceStatus provides sync and health status of a specific managed resource
type ResourceStatus struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"` // "Synced", "OutOfSync"
}

// ManagedResourcesResponse represents the response from /api/v1/applications/{name}/managed-resources
type ManagedResourcesResponse struct {
	Items []ManagedResourceItem `json:"items"`
}

// ManagedResourceItem represents a single managed resource with desired and live states
type ManagedResourceItem struct {
	Group               string `json:"group"`
	Version             string `json:"version"`
	Kind                string `json:"kind"`
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	TargetState         string `json:"targetState"`         // Serialized JSON of desired state
	LiveState           string `json:"liveState"`           // Serialized JSON of actual live cluster state
	NormalizedLiveState string `json:"normalizedLiveState"` // Normalized live state (if available)
	Hook                bool   `json:"hook"`
	RequiresPruning     bool   `json:"requiresPruning"`
}
