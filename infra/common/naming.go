package common

import (
	"fmt"
	"strings"
)

// ResourceNaming provides consistent naming conventions across clouds
type ResourceNaming struct {
	Environment string
	Project     string
	Component   string
}

// NewResourceNaming creates a new resource naming helper
func NewResourceNaming(environment, project, component string) *ResourceNaming {
	return &ResourceNaming{
		Environment: environment,
		Project:     project,
		Component:   component,
	}
}

// GetName returns a standardized resource name
func (rn *ResourceNaming) GetName(resourceType string) string {
	parts := []string{rn.Project, rn.Environment, rn.Component, resourceType}
	return strings.Join(parts, "-")
}

// GetTags returns standardized tags for cloud resources
func (rn *ResourceNaming) GetTags() map[string]string {
	return map[string]string{
		"Environment": rn.Environment,
		"Project":     rn.Project,
		"Component":   rn.Component,
		"ManagedBy":   "pulumi",
		"Purpose":     "multi-cloud-llm-router",
	}
}

// GetLabels returns standardized Kubernetes labels
func (rn *ResourceNaming) GetLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       rn.Component,
		"app.kubernetes.io/instance":   fmt.Sprintf("%s-%s", rn.Project, rn.Environment),
		"app.kubernetes.io/version":    "1.0.0",
		"app.kubernetes.io/component":  rn.Component,
		"app.kubernetes.io/part-of":    rn.Project,
		"app.kubernetes.io/managed-by": "pulumi",
		"environment":                  rn.Environment,
	}
}
