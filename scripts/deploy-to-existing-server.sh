#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Deploying Multi-Cloud LLM Router to Existing Server${NC}"
echo "========================================================="

# Get configuration
DEMO_DOMAIN=${DEMO_DOMAIN:-"mini.multicloud.navillasa.dev"}
API_BUDGET=${API_BUDGET:-"5.00"}
DEMO_PASSWORD=${DEMO_PASSWORD:-$(openssl rand -base64 12 | tr -d "=+/" | cut -c1-8)}

echo -e "Demo Domain: ${GREEN}${DEMO_DOMAIN}${NC}"
echo -e "API Budget: ${GREEN}\$${API_BUDGET}${NC}"
echo -e "Demo Password: ${GREEN}${DEMO_PASSWORD}${NC}"
echo
echo -e "${YELLOW}💾 Save this password! You'll need it to access the demo.${NC}"
echo

# Create deployment directory
mkdir -p deployment/simple

# Create simple docker-compose
cat > deployment/simple/docker-compose.yml << 'EOF'
version: '3.8'

services:
  router:
    build: 
      context: ../../
      dockerfile: deployment/simple/Dockerfile.router
    ports:
      - "8080:8080"
    environment:
      - DEMO_PASSWORD=${DEMO_PASSWORD}
      - API_BUDGET=${API_BUDGET}
    volumes:
      - ./config-demo.yaml:/app/config.yaml
      - ../../frontend:/app/frontend
    restart: unless-stopped

  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
      - ../../frontend:/usr/share/nginx/html
    depends_on:
      - router
    restart: unless-stopped
EOF

echo -e "${GREEN}📦 Deployment files will be created when you run this script!${NC}"
echo
echo -e "${BLUE}🚀 To deploy to your Hetzner server:${NC}"
echo
echo "1. Copy project to server:"
echo "   ${YELLOW}scp -r . root@[YOUR_SERVER_IP]:/opt/llm-router/${NC}"
echo
echo "2. SSH and deploy:"
echo "   ${YELLOW}ssh root@[YOUR_SERVER_IP]${NC}"
echo "   ${YELLOW}cd /opt/llm-router/deployment/simple${NC}"
echo "   ${YELLOW}export DEMO_PASSWORD=${DEMO_PASSWORD}${NC}"
echo "   ${YELLOW}docker-compose up -d --build${NC}"
echo
echo -e "${YELLOW}🔐 Demo Password: ${DEMO_PASSWORD}${NC}"

