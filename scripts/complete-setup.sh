#!/bin/bash

# Multi-Cloud LLM Router - Complete Setup and Test Script
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}"
cat << "EOF"
╭─────────────────────────────────────────────────╮
│                                                 │
│         Multi-Cloud LLM Router Setup           │
│                                                 │
│    Cost-optimized, latency-aware LLM routing   │
│         across AWS, GCP, and Azure             │
│                                                 │
╰─────────────────────────────────────────────────╯
EOF
echo -e "${NC}"

# Functions
log_info() {
    echo -e "${GREEN}ℹ️  $1${NC}"
}

log_warn() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

check_command() {
    if command -v $1 &> /dev/null; then
        log_success "$1 is installed"
        return 0
    else
        log_error "$1 is not installed"
        return 1
    fi
}

# Check prerequisites
log_info "Checking prerequisites..."
prerequisites_ok=true

check_command "go" || prerequisites_ok=false
check_command "kubectl" || prerequisites_ok=false
check_command "helm" || prerequisites_ok=false
check_command "pulumi" || prerequisites_ok=false

# Optional but recommended
if ! check_command "docker"; then
    log_warn "Docker not found - container builds will not work"
fi

if ! check_command "aws"; then
    log_warn "AWS CLI not found - AWS deployment will not work"
fi

if ! $prerequisites_ok; then
    log_error "Please install missing prerequisites before continuing"
    echo ""
    echo "Installation commands:"
    echo "  Go: https://golang.org/doc/install"
    echo "  kubectl: https://kubernetes.io/docs/tasks/tools/"
    echo "  Helm: https://helm.sh/docs/intro/install/"
    echo "  Pulumi: https://www.pulumi.com/docs/get-started/install/"
    exit 1
fi

echo ""
log_info "All prerequisites satisfied!"

# Menu
echo ""
echo -e "${YELLOW}Choose your deployment path:${NC}"
echo ""
echo "1) 🏃 Quick local demo (router only)"
echo "2) ☁️  Deploy AWS infrastructure + router"
echo "3) 🔬 Full development setup"
echo "4) 🧪 Run tests"
echo "5) 📊 Show project overview"
echo ""
read -p "Enter choice [1-5]: " choice

case $choice in
    1)
        echo ""
        log_info "Setting up local demo..."
        
        # Build router
        cd router
        log_info "Building router..."
        go mod tidy
        go build -o router .
        
        # Create demo config
        cat > config-demo.yaml << 'EOF'
server:
  port: 8080

router:
  stickinessWindow: 30s
  healthCheckInterval: 15s
  maxLatencyMs: 10000
  maxQueueDepth: 20
  overheadFactor: 1.1
  metricsUpdateInterval: 15s

clusters:
  - name: mock-aws
    endpoint: http://httpbin.org/status/200
    region: us-west-2
    provider: aws
    costPerHour: 0.0928
    authType: none
    
  - name: mock-gcp
    endpoint: http://httpbin.org/status/200
    region: us-central1
    provider: gcp
    costPerHour: 0.0950
    authType: none
    
  - name: mock-azure
    endpoint: http://httpbin.org/status/200
    region: eastus
    provider: azure
    costPerHour: 0.0968
    authType: none
EOF

        log_success "Demo configuration created!"
        echo ""
        echo -e "${YELLOW}Starting router demo...${NC}"
        echo "The router will start on http://localhost:8080"
        echo ""
        echo "In another terminal, try:"
        echo "  curl http://localhost:8080/health"
        echo "  curl http://localhost:8080/metrics"
        echo ""
        echo "Press Ctrl+C to stop the router"
        echo ""
        
        ./router --config config-demo.yaml
        ;;
        
    2)
        echo ""
        log_info "Setting up AWS infrastructure deployment..."
        
        # Check AWS credentials
        if ! aws sts get-caller-identity &> /dev/null; then
            log_error "AWS credentials not configured"
            echo "Please run: aws configure"
            exit 1
        fi
        
        log_success "AWS credentials found"
        
        # Get domain name
        read -p "Enter your domain name (e.g., llm.yourdomain.com): " domain
        if [ -z "$domain" ]; then
            log_error "Domain name is required"
            exit 1
        fi
        
        # Deploy infrastructure
        cd infra/aws
        log_info "Initializing Pulumi stack..."
        
        if [ ! -f "Pulumi.prod.yaml" ]; then
            pulumi stack init prod
        fi
        
        pulumi config set multi-cloud-llm-aws:domainName "$domain"
        pulumi config set aws:region us-west-2
        
        log_info "Deploying AWS infrastructure..."
        log_warn "This will take 10-15 minutes..."
        
        pulumi up --yes
        
        log_success "AWS infrastructure deployed!"
        
        # Get cluster info
        CLUSTER_NAME=$(pulumi stack output clusterName)
        REGION=$(pulumi stack output region)
        
        log_info "Updating kubeconfig..."
        aws eks update-kubeconfig --region $REGION --name $CLUSTER_NAME
        
        log_info "Waiting for Argo CD to be ready..."
        kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd
        
        log_success "Deployment complete!"
        echo ""
        echo "Next steps:"
        echo "1. Update DNS: Point aws.$domain to the LoadBalancer IP"
        echo "2. Check applications: kubectl get applications -n argocd"
        echo "3. Access Argo CD: kubectl port-forward svc/argocd-server -n argocd 8080:443"
        echo "4. Get initial password: kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath='{.data.password}' | base64 -d"
        ;;
        
    3)
        echo ""
        log_info "Setting up full development environment..."
        
        # Initialize Go modules
        log_info "Initializing Go modules..."
        go mod tidy
        
        cd router
        go mod tidy
        cd ..
        
        cd infra/aws
        go mod tidy
        cd ../..
        
        # Build router
        log_info "Building router..."
        cd router
        go build -o router .
        cd ..
        
        # Lint Helm charts
        log_info "Validating Helm charts..."
        helm lint charts/llm-server
        helm lint charts/platform
        
        # Create development config
        log_info "Creating development configuration..."
        cd router
        if [ ! -f "config-dev.yaml" ]; then
            cp config.yaml config-dev.yaml
            sed -i '' 's/8080/8081/g' config-dev.yaml  # Change port for dev
        fi
        cd ..
        
        log_success "Development environment ready!"
        echo ""
        echo "Development commands:"
        echo "  Start router:     cd router && ./router --config config-dev.yaml"
        echo "  Run tests:        cd router && go test ./..."
        echo "  Build charts:     helm template test charts/llm-server"
        echo "  Deploy to AWS:    cd infra/aws && pulumi up"
        ;;
        
    4)
        echo ""
        log_info "Running tests..."
        
        # Router tests
        log_info "Testing router..."
        cd router
        go test -v ./...
        go vet ./...
        cd ..
        
        # Infrastructure tests
        log_info "Testing infrastructure code..."
        cd infra/aws
        go vet ./...
        cd ../..
        
        # Helm tests
        log_info "Testing Helm charts..."
        helm lint charts/llm-server
        helm template test-release charts/llm-server --values charts/llm-server/values-aws.yaml > /dev/null
        
        log_success "All tests passed!"
        ;;
        
    5)
        echo ""
        log_info "Project Overview"
        echo ""
        echo -e "${YELLOW}Architecture:${NC}"
        echo "  Global Router (Go) → AWS EKS | GCP GKE | Azure AKS"
        echo "  Each cluster runs: llama.cpp + Prometheus + Argo CD"
        echo ""
        echo -e "${YELLOW}Cost Optimization:${NC}"
        echo "  - Real-time $/1K token calculation"
        echo "  - Routes to cheapest healthy cluster"
        echo "  - Scale-to-zero support"
        echo ""
        echo -e "${YELLOW}Tech Stack:${NC}"
        echo "  Infrastructure: Pulumi (Go)"
        echo "  Router: Go + Prometheus + Gorilla Mux"
        echo "  Deployment: Argo CD + Helm"
        echo "  LLM Server: llama.cpp + GGUF models"
        echo ""
        echo -e "${YELLOW}Directory Structure:${NC}"
        tree -L 2 -I '__pycache__|*.pyc|.git'
        echo ""
        echo -e "${YELLOW}Next Steps:${NC}"
        echo "  1. Run ./scripts/setup.sh and choose option 1 for demo"
        echo "  2. Read DEVELOPMENT.md for detailed guide"
        echo "  3. Deploy to AWS with option 2"
        echo "  4. Add GCP/Azure following the same pattern"
        ;;
        
    *)
        log_error "Invalid choice"
        exit 1
        ;;
esac

echo ""
log_success "Setup complete! 🎉"

if [ "$choice" != "1" ]; then
    echo ""
    echo -e "${BLUE}Useful commands:${NC}"
    echo "  Health check:     curl http://localhost:8080/health"
    echo "  Metrics:          curl http://localhost:8080/metrics"
    echo "  LLM request:      curl -X POST http://localhost:8080/v1/completions -H 'Content-Type: application/json' -d '{\"prompt\":\"Hello\",\"max_tokens\":10}'"
    echo "  View logs:        docker logs <container> (if using Docker)"
    echo ""
    echo "📚 Read DEVELOPMENT.md for more details"
    echo "🐛 Report issues at: https://github.com/navillasa/multi-cloud-llm-router/issues"
fi
