#!/bin/bash

# Multi-Cloud LLM Router - AWS Deployment Script
# This script deploys the complete AWS infrastructure and LLM servers

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REGION="${AWS_REGION:-us-east-1}"
ENVIRONMENT="${ENVIRONMENT:-prod}"
DOMAIN_NAME="${DOMAIN_NAME:-}"
INSTANCE_TYPE="${INSTANCE_TYPE:-t3.small}"
USE_SPOT="${USE_SPOT:-true}"
ULTRA_CHEAP="${ULTRA_CHEAP:-false}"

echo -e "${BLUE}🚀 Multi-Cloud LLM Router - AWS Deployment${NC}"
echo "=================================="
echo "Region: $REGION"
echo "Environment: $ENVIRONMENT"
echo "Instance Type: $INSTANCE_TYPE"
echo "Use Spot Instances: $USE_SPOT"
echo "Ultra Cheap Mode: $ULTRA_CHEAP"
echo "Domain: ${DOMAIN_NAME:-'Not set - will skip ingress setup'}"
echo

if [ "$ULTRA_CHEAP" = "true" ]; then
    echo -e "${YELLOW}💸 ULTRA CHEAP MODE ENABLED${NC}"
    echo "• Starts scaled to zero (no compute costs)"
    echo "• Uses smallest model (OpenELM-270M)"
    echo "• No ingress (access via port-forward)"
    echo "• Cost when active: ~$0.005/hour"
    echo "• Cost when paused: ~$0.10/hour (EKS only)"
    echo
fi

# Function to print section headers
print_section() {
    echo -e "\n${BLUE}📋 $1${NC}"
    echo "-----------------------------------"
}

# Function to check if command exists
check_command() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}❌ $1 is not installed. Please install it first.${NC}"
        exit 1
    fi
}

# Check prerequisites
print_section "Checking Prerequisites"
check_command "pulumi"
check_command "kubectl"
check_command "helm"
check_command "aws"

# Check AWS credentials
if ! aws sts get-caller-identity &> /dev/null; then
    echo -e "${RED}❌ AWS credentials not configured. Run 'aws configure' first.${NC}"
    exit 1
fi

echo -e "${GREEN}✅ All prerequisites satisfied${NC}"

# Step 1: Deploy Infrastructure
print_section "Step 1: Deploying AWS Infrastructure"

cd infra/aws

# Initialize or select stack
if ! pulumi stack select $ENVIRONMENT 2>/dev/null; then
    echo "Creating new Pulumi stack: $ENVIRONMENT"
    pulumi stack init $ENVIRONMENT
fi

# Configure stack
echo "Configuring Pulumi stack..."
pulumi config set aws:region $REGION
pulumi config set multi-cloud-llm-aws:environment $ENVIRONMENT

if [ -n "$DOMAIN_NAME" ]; then
    pulumi config set multi-cloud-llm-aws:domainName $DOMAIN_NAME
else
    echo -e "${YELLOW}⚠️  No domain name provided. Ingress will need manual configuration.${NC}"
fi

# Deploy infrastructure
echo "Deploying infrastructure... This may take 10-15 minutes."
pulumi up --yes

# Get outputs
CLUSTER_NAME=$(pulumi stack output clusterName)
echo -e "${GREEN}✅ Infrastructure deployed successfully${NC}"
echo "Cluster name: $CLUSTER_NAME"

cd ../..

# Step 2: Configure kubectl
print_section "Step 2: Configuring kubectl"

aws eks update-kubeconfig --region $REGION --name $CLUSTER_NAME

# Wait for nodes to be ready
echo "Waiting for cluster nodes to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=300s

echo -e "${GREEN}✅ kubectl configured and cluster is ready${NC}"

# Step 3: Install Platform Components
print_section "Step 3: Installing Platform Components"

# Install ingress-nginx
echo "Installing ingress-nginx..."
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
    --namespace ingress-nginx \
    --create-namespace \
    --set controller.service.type=LoadBalancer \
    --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"="nlb" \
    --wait

# Install cert-manager if domain is provided
if [ -n "$DOMAIN_NAME" ]; then
    echo "Installing cert-manager..."
    helm repo add jetstack https://charts.jetstack.io
    helm repo update
    
    helm upgrade --install cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --create-namespace \
        --set installCRDs=true \
        --wait
        
    # Create Let's Encrypt issuer
    echo "Creating Let's Encrypt cluster issuer..."
    kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@${DOMAIN_NAME#*.}
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
EOF
fi

# Install Prometheus
echo "Installing Prometheus monitoring..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
    --namespace monitoring \
    --create-namespace \
    --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
    --set grafana.adminPassword=admin123 \
    --wait

echo -e "${GREEN}✅ Platform components installed${NC}"

# Step 4: Deploy LLM Server
print_section "Step 4: Deploying LLM Server"

kubectl create namespace llm-server --dry-run=client -o yaml | kubectl apply -f -

# Prepare values file
VALUES_FILE="charts/llm-server/values-deployment.yaml"
if [ "$ULTRA_CHEAP" = "true" ]; then
    cp charts/llm-server/values-ultra-cheap.yaml $VALUES_FILE
    echo "Using ultra-cheap configuration"
else
    cp charts/llm-server/values-cost-optimized.yaml $VALUES_FILE
    echo "Using cost-optimized configuration"
fi

# Update domain in values file if provided
if [ -n "$DOMAIN_NAME" ]; then
    sed -i.bak "s/aws\.llm\.yourdomain\.com/aws.${DOMAIN_NAME}/g" $VALUES_FILE
    rm ${VALUES_FILE}.bak
    echo "Updated domain to: aws.${DOMAIN_NAME}"
else
    # Disable ingress if no domain
    sed -i.bak 's/enabled: true/enabled: false/g' $VALUES_FILE
    rm ${VALUES_FILE}.bak
    echo "Disabled ingress (no domain provided)"
fi

# Deploy LLM server
echo "Deploying LLM server... This may take 5-10 minutes for model download."
cd charts/llm-server

helm upgrade --install llm-server . \
    --namespace llm-server \
    --values values-deployment.yaml \
    --wait --timeout=15m

cd ../..

echo -e "${GREEN}✅ LLM server deployed${NC}"

# Step 5: Verify Deployment
print_section "Step 5: Verifying Deployment"

# Check pod status
echo "Checking pod status..."
kubectl get pods -n llm-server

# Wait for pods to be ready
echo "Waiting for LLM server to be ready..."
kubectl wait --for=condition=Ready pods -l app.kubernetes.io/name=llm-server -n llm-server --timeout=300s

# Get service information
if [ -n "$DOMAIN_NAME" ]; then
    # Get ingress info
    echo -e "\n${GREEN}🌐 Ingress Information:${NC}"
    kubectl get ingress -n llm-server
    
    echo -e "\n${YELLOW}📝 DNS Configuration Needed:${NC}"
    echo "Point aws.${DOMAIN_NAME} to the LoadBalancer IP:"
    kubectl get svc -n ingress-nginx ingress-nginx-controller -o jsonpath='{.status.loadBalancer.ingress[0].hostname}{.status.loadBalancer.ingress[0].ip}'
    echo
    
    ENDPOINT="https://aws.${DOMAIN_NAME}"
else
    # Show port-forward instructions for no domain setup
    echo -e "\n${YELLOW}🔗 Access via Port Forward:${NC}"
    echo "Run this command to access the LLM server:"
    echo "kubectl port-forward -n llm-server svc/llm-server 8080:8080"
    echo "Then access: http://localhost:8080"
    
    ENDPOINT="http://localhost:8080"
fi

# Step 6: Test the deployment
print_section "Step 6: Testing Deployment"

if [ -n "$DOMAIN_NAME" ]; then
    echo "Waiting for DNS propagation and SSL certificate..."
    echo "This may take a few minutes..."
    
    for i in {1..30}; do
        if curl -s --max-time 10 "${ENDPOINT}/health" > /dev/null 2>&1; then
            echo -e "${GREEN}✅ Health check passed!${NC}"
            break
        else
            echo "Attempt $i/30: Waiting for service to be ready..."
            sleep 30
        fi
    done
else
    echo "To test the deployment, run:"
    echo "kubectl port-forward -n llm-server svc/llm-server 8080:8080"
    echo "Then test: curl http://localhost:8080/health"
fi

# Final information
print_section "Deployment Complete! 🎉"

echo -e "${GREEN}✅ AWS deployment completed successfully!${NC}"
echo
echo -e "${BLUE}📊 Cluster Information:${NC}"
echo "Cluster Name: $CLUSTER_NAME"
echo "Region: $REGION"
echo "Endpoint: $ENDPOINT"
echo
echo -e "${BLUE}📈 Monitoring Access:${NC}"
echo "Prometheus: kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090"
echo "Grafana: kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80 (admin/admin123)"
echo
echo -e "${BLUE}🧪 Test Commands:${NC}"
if [ -n "$DOMAIN_NAME" ]; then
    echo "Health: curl ${ENDPOINT}/health"
    echo "Chat: curl -X POST ${ENDPOINT}/v1/chat/completions -H 'Content-Type: application/json' -d '{\"model\":\"tinyllama\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}],\"max_tokens\":50}'"
else
    echo "First run: kubectl port-forward -n llm-server svc/llm-server 8080:8080"
    echo "Health: curl http://localhost:8080/health"
    echo "Chat: curl -X POST http://localhost:8080/v1/chat/completions -H 'Content-Type: application/json' -d '{\"model\":\"tinyllama\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}],\"max_tokens\":50}'"
fi
echo
echo -e "${BLUE}💰 Cost Optimization:${NC}"
echo "Scale down: kubectl scale deployment -n llm-server llm-server --replicas=0"
echo "Scale up: kubectl scale deployment -n llm-server llm-server --replicas=1"
echo
echo -e "${BLUE}🔧 Next Steps:${NC}"
echo "1. Update router/config.yaml with cluster endpoint: $ENDPOINT"
echo "2. Test hybrid routing: ./scripts/test-hybrid-routing.sh"
echo "3. Deploy to other clouds (GCP, Azure) for multi-cloud setup"
echo "4. Set up CI/CD with Argo CD"

# Cleanup temporary files
rm -f $VALUES_FILE

echo -e "\n${GREEN}Deployment script completed! 🚀${NC}"
