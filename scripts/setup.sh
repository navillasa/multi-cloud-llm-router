#!/bin/bash

# Multi-Cloud LLM Router - Quick Start Script
# This script helps you get started with the multi-cloud LLM router

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Multi-Cloud LLM Router Setup${NC}"
echo "==========================================="

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

check_command() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}❌ $1 is not installed${NC}"
        exit 1
    else
        echo -e "${GREEN}✅ $1 is installed${NC}"
    fi
}

check_command "go"
check_command "pulumi"
check_command "kubectl"
check_command "helm"

echo ""
echo -e "${YELLOW}Choose deployment option:${NC}"
echo "1) Deploy AWS infrastructure only"
echo "2) Deploy router locally for development"
echo "3) Full multi-cloud deployment (requires all cloud CLIs)"
echo ""
read -p "Enter choice [1-3]: " choice

case $choice in
    1)
        echo -e "${GREEN}Deploying AWS infrastructure...${NC}"
        cd infra/aws
        
        # Check if config exists
        if [ ! -f "Pulumi.prod.yaml" ]; then
            echo -e "${YELLOW}Creating Pulumi configuration...${NC}"
            pulumi stack init prod
            
            read -p "Enter your domain name (e.g., llm.yourdomain.com): " domain
            pulumi config set multi-cloud-llm-aws:domainName "$domain"
            pulumi config set aws:region us-west-2
        fi
        
        echo -e "${YELLOW}Running pulumi up...${NC}"
        pulumi up
        
        echo -e "${GREEN}✅ AWS infrastructure deployed!${NC}"
        echo ""
        echo "Next steps:"
        echo "1. Update your DNS to point aws.yourdomain.com to the LoadBalancer IP"
        echo "2. Wait for Argo CD to sync applications"
        echo "3. Check cluster status: kubectl get pods -n llm-system"
        ;;
        
    2)
        echo -e "${GREEN}Setting up local router development...${NC}"
        cd router
        
        # Build router
        echo -e "${YELLOW}Building router...${NC}"
        go mod tidy
        go build -o router .
        
        # Create development config
        if [ ! -f "config-dev.yaml" ]; then
            echo -e "${YELLOW}Creating development configuration...${NC}"
            cat > config-dev.yaml << EOF
server:
  port: 8080

router:
  stickinessWindow: 30s
  healthCheckInterval: 15s
  maxLatencyMs: 10000
  maxQueueDepth: 20
  overheadFactor: 1.1

clusters:
  - name: local-test
    endpoint: http://localhost:8081
    region: local
    provider: local
    costPerHour: 0.01
    authType: none
EOF
        fi
        
        echo -e "${GREEN}✅ Router built successfully!${NC}"
        echo ""
        echo "To start the router:"
        echo "  ./router --config config-dev.yaml"
        echo ""
        echo "API will be available at: http://localhost:8080"
        echo "Metrics at: http://localhost:8080/metrics"
        echo "Health check: http://localhost:8080/health"
        ;;
        
    3)
        echo -e "${RED}Full multi-cloud deployment not implemented in this script yet.${NC}"
        echo "Please deploy each cloud manually:"
        echo "1. cd infra/aws && pulumi up"
        echo "2. cd infra/gcp && pulumi up"  
        echo "3. cd infra/azure && pulumi up"
        echo "4. Deploy router to your preferred platform"
        ;;
        
    *)
        echo -e "${RED}Invalid choice${NC}"
        exit 1
        ;;
esac

echo ""
echo -e "${GREEN}🎉 Setup complete!${NC}"
