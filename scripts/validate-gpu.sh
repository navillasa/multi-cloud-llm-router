#!/bin/bash

# GPU Configuration Validation Script
# Tests GPU support across all three cloud providers

set -e

echo "🚀 GPU Configuration Validation"
echo "================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    if [ $2 -eq 0 ]; then
        echo -e "${GREEN}✅ $1${NC}"
    else
        echo -e "${RED}❌ $1${NC}"
    fi
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

echo ""
echo "📋 Checking GPU configuration files..."

# Check base chart has GPU support
if grep -q "gpu:" charts/llm-server/values.yaml; then
    print_status "Base GPU configuration found" 0
else
    print_status "Base GPU configuration missing" 1
fi

# Check cloud-specific GPU configs
for cloud in aws azure gcp; do
    if grep -q "gpu:" charts/llm-server/values-${cloud}.yaml; then
        print_status "GPU config for $cloud found" 0
    else
        print_status "GPU config for $cloud missing" 1
    fi
done

echo ""
echo "🔧 Checking GPU overlays..."

# Check GPU overlays exist
for cloud in aws azure gcp; do
    if [ -d "clusters/${cloud}/overlays/gpu" ]; then
        print_status "GPU overlay for $cloud exists" 0
        
        # Check overlay has required files
        if [ -f "clusters/${cloud}/overlays/gpu/kustomization.yaml" ] && \
           [ -f "clusters/${cloud}/overlays/gpu/values-override.yaml" ]; then
            print_status "GPU overlay files for $cloud complete" 0
        else
            print_status "GPU overlay files for $cloud incomplete" 1
        fi
    else
        print_status "GPU overlay for $cloud missing" 1
    fi
done

echo ""
echo "⚙️  Validating Helm templates..."

# Test Helm template rendering with GPU enabled
for cloud in aws azure gcp; do
    echo "Testing $cloud GPU template..."
    if helm template test-gpu charts/llm-server \
        -f charts/llm-server/values-${cloud}.yaml \
        --set gpu.enabled=true \
        --dry-run > /dev/null 2>&1; then
        print_status "GPU template for $cloud renders correctly" 0
    else
        print_status "GPU template for $cloud has errors" 1
    fi
done

echo ""
echo "📊 GPU Cost Estimates:"
echo "======================"

echo "💰 Monthly costs (24/7 operation):"
echo ""
echo "CPU-Only Deployment:"
echo "  AWS (t3.small spot):           ~$21/month"
echo "  Azure (Standard_B2s spot):     ~$21/month" 
echo "  GCP (e2-standard-2 preempt):   ~$21/month"
echo "  Total CPU:                     ~$63/month"
echo ""
echo "GPU-Enabled Deployment:"
echo "  AWS (g4dn.xlarge spot):        ~$114/month"
echo "  Azure (NC4as_T4_v3 spot):      ~$151/month"
echo "  GCP (n1-std-4 + T4 preempt):   ~$201/month"
echo "  Total GPU:                     ~$466/month"
echo ""
print_warning "Scale-to-zero recommended: Base $63/month + GPU on-demand"

echo ""
echo "🧪 Testing Kustomize builds..."

# Test kustomize builds for GPU overlays
for cloud in aws azure gcp; do
    if kubectl kustomize clusters/${cloud}/overlays/gpu/ > /dev/null 2>&1; then
        print_status "Kustomize build for $cloud GPU overlay successful" 0
    else
        print_status "Kustomize build for $cloud GPU overlay failed" 1
    fi
done

echo ""
echo "📋 GPU Instance Type Summary:"
echo "============================="
echo "AWS:   g4dn.xlarge   (4 vCPU, 16GB RAM, 1x Tesla T4 16GB)"
echo "Azure: NC4as_T4_v3   (4 vCPU, 28GB RAM, 1x Tesla T4 16GB)" 
echo "GCP:   n1-std-4+T4   (4 vCPU, 15GB RAM, 1x Tesla T4 16GB)"

echo ""
echo "🚀 Quick Deployment Commands:"
echo "============================="
echo ""
echo "# Enable GPU for AWS:"
echo "kubectl apply -k clusters/aws/overlays/gpu/"
echo ""
echo "# Enable GPU for Azure:"  
echo "kubectl apply -k clusters/azure/overlays/gpu/"
echo ""
echo "# Enable GPU for GCP:"
echo "kubectl apply -k clusters/gcp/overlays/gpu/"
echo ""
echo "# Test GPU availability:"
echo "kubectl get nodes -l accelerator=nvidia-tesla-t4"
echo ""
echo "# Monitor GPU usage:"
echo "kubectl top node --use-protocol-buffers"

echo ""
echo "✅ GPU validation complete!"
echo ""
print_warning "Remember to enable GPU node pools in your cloud infrastructure first!"
print_warning "See docs/GPU_SUPPORT.md for detailed setup instructions."
