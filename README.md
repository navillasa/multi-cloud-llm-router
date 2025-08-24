# Multi-Cloud LLM Router

A cost-optimized, latency-aware router that automatically distributes LLM requests across multiple cloud providers (AWS, GCP, Azure) running CPU-only llama.cpp servers.

## Architecture

```
[Client] ──HTTPS──> [Global Router (Hetzner VM or Cloudflare Worker)]
                         │
          ┌──────────────┼────────────────┐
          │              │                │
   HTTPS/mTLS      HTTPS/mTLS       HTTPS/mTLS
          │              │                │
 [Ingress + Argo CD] [Ingress + Argo CD] [Ingress + Argo CD]
   AWS EKS (CPU)        GCP GKE (CPU)       Azure AKS (CPU)
      │                    │                  │
 [llama.cpp pods]    [llama.cpp pods]   [llama.cpp pods]
  (gguf models)        (gguf models)       (gguf models)
      │                    │                  │
 [Prom + Exporter]  [Prom + Exporter]  [Prom + Exporter]
      └───────────────metrics + costs─────────────┘
                         ↓
                 [Router cost engine]
```

## Features

- **Cost-aware routing**: Automatically routes to the cheapest healthy cluster based on real-time $/1K token calculations
- **Multi-cloud redundancy**: Deploys across AWS EKS, GCP GKE, and Azure AKS
- **GitOps deployment**: Uses Argo CD for automated deployments
- **Observability**: Prometheus metrics for cost, latency, and throughput tracking
- **CPU-optimized**: Uses quantized GGUF models for cost-effective CPU inference
- **Auto-scaling**: HPA based on CPU utilization and queue depth

## Quick Start

### Prerequisites

- Go 1.21+
- Pulumi CLI
- kubectl
- Helm 3.x
- Cloud provider CLIs (aws, gcloud, az)

### 1. Deploy Infrastructure

```bash
cd infra/aws
pulumi up

cd ../gcp  
pulumi up

cd ../azure
pulumi up
```

### 2. Verify Argo CD Deployment

```bash
kubectl --context aws-cluster get pods -n argocd
kubectl --context gcp-cluster get pods -n argocd  
kubectl --context azure-cluster get pods -n argocd
```

### 3. Deploy Router

```bash
cd router
go build -o router .
./router --config config.yaml
```

## Directory Structure

```
├─ infra/                       # Pulumi (Go) for clouds + bootstrap
│  ├─ aws/                      # EKS, VPC, nodegroup, ECR, IAM OIDC, DNS records
│  ├─ gcp/                      # GKE, VPC, Artifact Registry, IAM, DNS
│  ├─ azure/                    # AKS, VNet, ACR, Managed Identity, DNS
│  └─ common/                   # shared Go utilities (tags, naming, kubeconfig emit)
├─ clusters/                    # Argo CD "app-of-apps" per cluster
│  ├─ aws/overlays/prod/
│  ├─ gcp/overlays/prod/
│  └─ azure/overlays/prod/
├─ charts/                      # Helm charts
│  ├─ llm-server/               # llama.cpp + Service + Ingress + HPA + PVC
│  ├─ exporters/                # cost+throughput exporter (Go) + ServiceMonitor
│  └─ platform/                 # ingress-nginx, cert-manager, kube-prometheus-stack
├─ router/                      # global Router (Go)
│  ├─ internal/cost/            # $/1K calc, hysteresis
│  ├─ internal/health/          # per-cluster health + queue polling
│  ├─ internal/forward/         # HMAC/mTLS signed forwarding
│  └─ main.go
├─ argocd/                      # Argo CD bootstrap manifests (install via Pulumi)
└─ models/                      # model metadata (names, URIs, quant levels)
```

## Cost Calculation

The router computes effective cost per 1K output tokens:

```
$per1K = (node_hourly_cost / (tokens_per_sec * 3600)) * overhead_factor
```

- `node_hourly_cost`: Static value per node pool from cloud pricing
- `tokens_per_sec`: Live TPS from llama.cpp metrics
- `overhead_factor`: Accounts for idle headroom and safety margin (default 1.10)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details
