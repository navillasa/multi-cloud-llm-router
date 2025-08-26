# LLM Deployment Guide

This guide will help you deploy actual LLMs to your cloud clusters and get them running.

## Prerequisites

- AWS CLI configured with appropriate permissions
- Pulumi CLI installed and logged in
- kubectl installed
- helm installed
- A domain name for the cluster endpoint

## Step 1: Deploy AWS Infrastructure

### 1.1 Configure Pulumi

```bash
cd infra/aws

# Create new stack for your deployment
pulumi stack init prod  # or dev

# Set required configuration
pulumi config set aws:region us-east-1
pulumi config set multi-cloud-llm-aws:environment prod
pulumi config set multi-cloud-llm-aws:domainName "llm.yourdomain.com"  # Replace with your domain

# Optional: Set cost optimization settings
pulumi config set multi-cloud-llm-aws:instanceType t3.small
pulumi config set multi-cloud-llm-aws:useSpotInstances true
```

### 1.2 Deploy Infrastructure

```bash
# Deploy the infrastructure
pulumi up

# This will create:
# - VPC with public subnets
# - EKS cluster with cost-optimized node group (t3.small spot instances)
# - IAM roles and policies
# - Argo CD installation
```

### 1.3 Configure kubectl

```bash
# Get the kubeconfig
aws eks update-kubeconfig --region us-east-1 --name $(pulumi stack output clusterName)

# Verify connection
kubectl get nodes
kubectl get pods -n argocd
```

## Step 2: Prepare Model Files

We need to use proper tiny LLM models optimized for CPU inference.

### 2.1 Choose Your Model

For cost-effective CPU inference, these models work well:

```bash
# TinyLlama 1.1B (very small, good for testing)
MODEL_URI="https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.q4_k_m.gguf"

# Phi-2 2.7B (better quality, still efficient)
MODEL_URI="https://huggingface.co/microsoft/phi-2/resolve/main/model.safetensors"

# OpenELM 270M (ultra-small Apple model)
MODEL_URI="https://huggingface.co/apple/OpenELM-270M/resolve/main/model.safetensors"
```

### 2.2 Update Model Configuration

Edit the values file for your deployment:

```bash
cp charts/llm-server/values.yaml charts/llm-server/values-prod.yaml
```

Edit `values-prod.yaml`:

```yaml
model:
  # Use TinyLlama for initial deployment
  uri: "https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.q4_k_m.gguf"
  path: "/models/tinyllama.gguf"
  storageSize: "5Gi"  # Smaller for tiny models

# Resource limits for t3.small nodes
resources:
  limits:
    cpu: 1800m      # Leave some for system
    memory: 1500Mi  # Conservative for 2GB node
  requests:
    cpu: 1000m
    memory: 1000Mi

# Update ingress for your domain
ingress:
  hosts:
    - host: aws.llm.yourdomain.com  # Replace with your domain
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: llm-server-tls
      hosts:
        - aws.llm.yourdomain.com
```

## Step 3: Deploy Platform Components

### 3.1 Install ingress-nginx

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace \
  --set controller.service.type=LoadBalancer \
  --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"="nlb"
```

### 3.2 Install cert-manager

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update

helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set installCRDs=true

# Create Let's Encrypt issuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com  # Replace with your email
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
EOF
```

### 3.3 Install Prometheus

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
```

## Step 4: Deploy LLM Server

### 4.1 Create Namespace

```bash
kubectl create namespace llm-server
```

### 4.2 Deploy via Helm

```bash
cd charts/llm-server

helm install llm-server . \
  --namespace llm-server \
  --values values-prod.yaml \
  --wait --timeout=10m
```

### 4.3 Monitor Deployment

```bash
# Watch pods start up
kubectl get pods -n llm-server -w

# Check model download progress
kubectl logs -n llm-server -l app.kubernetes.io/name=llm-server -c model-downloader -f

# Check LLM server logs
kubectl logs -n llm-server -l app.kubernetes.io/name=llm-server -c llama-cpp-server -f
```

## Step 5: Configure DNS

### 5.1 Get Load Balancer IP

```bash
kubectl get svc -n ingress-nginx ingress-nginx-controller
```

### 5.2 Update DNS

Point your domain (`aws.llm.yourdomain.com`) to the LoadBalancer's external IP.

## Step 6: Test the Deployment

### 6.1 Health Check

```bash
# Wait for DNS to propagate, then test
curl https://aws.llm.yourdomain.com/health

# Should return: {"status":"ok"}
```

### 6.2 Test LLM Inference

```bash
# Test completion endpoint
curl -X POST https://aws.llm.yourdomain.com/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Hello, I am",
    "max_tokens": 50,
    "temperature": 0.7
  }'

# Test chat completions endpoint
curl -X POST https://aws.llm.yourdomain.com/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tinyllama",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 50
  }'
```

## Step 7: Update Router Configuration

### 7.1 Update Router Config

Edit `router/config.yaml`:

```yaml
clusters:
  - name: aws-us-east-1
    endpoint: https://aws.llm.yourdomain.com  # Your actual endpoint
    region: us-east-1
    provider: aws
    costPerHour: 0.0464  # t3.small spot instance cost
    authType: hmac
    sharedSecret: your-shared-secret-here
```

### 7.2 Test Router to Cluster

```bash
cd router
go run main.go --config config.yaml

# In another terminal, test routing
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tinyllama",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 50
  }'
```

## Step 8: Monitoring and Metrics

### 8.1 Access Prometheus

```bash
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
```

Open http://localhost:9090 and check these metrics:
- `container_cpu_usage_seconds_total`
- `container_memory_usage_bytes`
- Custom LLM metrics from the service monitor

### 8.2 Access Grafana

```bash
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80

# Get admin password
kubectl get secret -n monitoring prometheus-grafana -o jsonpath="{.data.admin-password}" | base64 --decode
```

Open http://localhost:3000 (admin/password from above)

## Troubleshooting

### Common Issues

1. **Pod stuck in Pending**
   ```bash
   kubectl describe pod -n llm-server <pod-name>
   # Check for resource constraints or node issues
   ```

2. **Model download fails**
   ```bash
   kubectl logs -n llm-server <pod-name> -c model-downloader
   # Check model URI and network connectivity
   ```

3. **LLM server won't start**
   ```bash
   kubectl logs -n llm-server <pod-name> -c llama-cpp-server
   # Check model file format and server arguments
   ```

4. **Ingress not working**
   ```bash
   kubectl get ingress -n llm-server
   kubectl describe ingress -n llm-server llm-server
   # Check DNS configuration and certificate issues
   ```

### Cost Optimization

1. **Use spot instances** (already configured)
2. **Scale to zero** when not in use:
   ```bash
   kubectl scale deployment -n llm-server llm-server --replicas=0
   ```
3. **Monitor costs** via AWS Cost Explorer
4. **Use smaller models** for development

## Next Steps

Once AWS is working:
1. Deploy to GCP using similar pattern
2. Deploy to Azure
3. Test multi-cloud routing
4. Set up CI/CD with Argo CD
5. Implement proper secrets management
6. Add alerting and monitoring

## Quick Commands Reference

```bash
# Deploy infrastructure
cd infra/aws && pulumi up

# Deploy LLM server
cd charts/llm-server && helm install llm-server . -n llm-server --values values-prod.yaml

# Scale down to save costs
kubectl scale deployment -n llm-server llm-server --replicas=0

# Scale back up
kubectl scale deployment -n llm-server llm-server --replicas=1

# Check logs
kubectl logs -n llm-server -l app.kubernetes.io/name=llm-server -c llama-cpp-server

# Test endpoint
curl https://aws.llm.yourdomain.com/health
```
