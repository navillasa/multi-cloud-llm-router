package common

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// KubeConfig represents a Kubernetes configuration
type KubeConfig struct {
	APIVersion     string                 `yaml:"apiVersion"`
	Kind           string                 `yaml:"kind"`
	CurrentContext string                 `yaml:"current-context"`
	Contexts       []KubeConfigContext    `yaml:"contexts"`
	Clusters       []KubeConfigCluster    `yaml:"clusters"`
	Users          []KubeConfigUser       `yaml:"users"`
}

type KubeConfigContext struct {
	Name    string                   `yaml:"name"`
	Context KubeConfigContextDetails `yaml:"context"`
}

type KubeConfigContextDetails struct {
	Cluster string `yaml:"cluster"`
	User    string `yaml:"user"`
}

type KubeConfigCluster struct {
	Name    string                   `yaml:"name"`
	Cluster KubeConfigClusterDetails `yaml:"cluster"`
}

type KubeConfigClusterDetails struct {
	CertificateAuthorityData string `yaml:"certificate-authority-data"`
	Server                   string `yaml:"server"`
}

type KubeConfigUser struct {
	Name string              `yaml:"name"`
	User KubeConfigUserToken `yaml:"user"`
}

type KubeConfigUserToken struct {
	Token string `yaml:"token"`
}

// GenerateEKSKubeConfig generates a kubeconfig for EKS
func GenerateEKSKubeConfig(clusterName, endpoint, certificateAuthority pulumi.StringOutput, region string) pulumi.StringOutput {
	return pulumi.Sprintf(`apiVersion: v1
kind: Config
current-context: %s
contexts:
- name: %s
  context:
    cluster: %s
    user: %s
clusters:
- name: %s
  cluster:
    certificate-authority-data: %s
    server: %s
users:
- name: %s
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws
      args:
        - eks
        - get-token
        - --cluster-name
        - %s
        - --region
        - %s
`, clusterName, clusterName, clusterName, clusterName, clusterName, certificateAuthority, endpoint, clusterName, clusterName, region)
}

// GenerateGKEKubeConfig generates a kubeconfig for GKE
func GenerateGKEKubeConfig(clusterName, endpoint, certificateAuthority, accessToken pulumi.StringOutput) pulumi.StringOutput {
	return pulumi.Sprintf(`apiVersion: v1
kind: Config
current-context: %s
contexts:
- name: %s
  context:
    cluster: %s
    user: %s
clusters:
- name: %s
  cluster:
    certificate-authority-data: %s
    server: https://%s
users:
- name: %s
  user:
    token: %s
`, clusterName, clusterName, clusterName, clusterName, clusterName, certificateAuthority, endpoint, clusterName, accessToken)
}

// GenerateAKSKubeConfig generates a kubeconfig for AKS
func GenerateAKSKubeConfig(clusterName, endpoint, certificateAuthority pulumi.StringOutput, resourceGroup, subscriptionId string) pulumi.StringOutput {
	return pulumi.Sprintf(`apiVersion: v1
kind: Config
current-context: %s
contexts:
- name: %s
  context:
    cluster: %s
    user: %s
clusters:
- name: %s
  cluster:
    certificate-authority-data: %s
    server: %s
users:
- name: %s
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: az
      args:
        - aks
        - get-credentials
        - --resource-group
        - %s
        - --name
        - %s
        - --subscription
        - %s
        - --file
        - "-"
        - --format
        - exec
`, clusterName, clusterName, clusterName, clusterName, clusterName, certificateAuthority, endpoint, clusterName, resourceGroup, clusterName, subscriptionId)
}
