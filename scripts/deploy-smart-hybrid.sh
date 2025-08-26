#!/bin/bash

# Multi-Cloud LLM Router - Smart Hybrid Demo Deployment
# Real router + Smart mocking + External APIs with budget

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
DEMO_DOMAIN="${DEMO_DOMAIN:-demo.yourdomain.com}"
API_BUDGET="${API_BUDGET:-5.00}"
VPS_PROVIDER="${VPS_PROVIDER:-hetzner}"  # hetzner, digitalocean, or linode

echo -e "${BLUE}🌐 Smart Hybrid LLM Router Demo${NC}"
echo "=============================================="
echo "Demo Domain: $DEMO_DOMAIN"
echo "Monthly API Budget: \$$API_BUDGET"
echo "VPS Provider: $VPS_PROVIDER"
echo

# Generate secure demo password
if [ -z "$DEMO_PASSWORD" ]; then
    DEMO_PASSWORD=$(openssl rand -base64 12 | tr -d "=+/" | cut -c1-10)
    echo -e "${GREEN}Generated Demo Password: $DEMO_PASSWORD${NC}"
    echo -e "${YELLOW}💾 Save this password! You'll need it to access the demo.${NC}"
else
    echo "Using provided demo password"
fi
echo

print_section() {
    echo -e "\n${BLUE}📋 $1${NC}"
    echo "-----------------------------------"
}

print_section "Smart Hybrid Strategy"

echo "This deployment creates:"
echo "• 🖥️  Real Go router with intelligent demo mode"
echo "• 🤖 Mock 'self-hosted clusters' with realistic responses"
echo "• 🌐 Limited external API calls within budget"
echo "• 📊 Real Prometheus metrics and monitoring"
echo "• 🎨 Interactive frontend dashboard"
echo "• 🔐 Password-protected access"
echo "• 💰 Cost: ~\$8-12/month total"

echo
echo -e "${BLUE}💡 How it works:${NC}"
echo "1. Simple requests → Mock 'cluster' responses (free)"
echo "2. Complex requests → Real external APIs (within budget)"
echo "3. Budget exceeded → Smart mock responses with explanation"
echo "4. All routing decisions → Real metrics and logging"

echo
read -p "Continue with Smart Hybrid deployment? (y/n): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Deployment cancelled."
    exit 1
fi

print_section "Creating VPS and Deployment Files"

# Create docker-compose for smart hybrid
cat > docker-compose.smart-hybrid.yml << EOF
version: '3.8'

services:
  llm-router:
    build: 
      context: ./router
      dockerfile: Dockerfile.smart-hybrid
    ports:
      - "8080:8080"
    environment:
      - DEMO_MODE=smart_hybrid
      - DEMO_PASSWORD=${DEMO_PASSWORD}
      - API_BUDGET_MONTHLY=${API_BUDGET}
      - OPENAI_API_KEY=\${OPENAI_API_KEY}
      - ANTHROPIC_API_KEY=\${ANTHROPIC_API_KEY}
      - GEMINI_API_KEY=\${GEMINI_API_KEY}
      - LOG_LEVEL=info
    volumes:
      - ./data:/app/data
    restart: unless-stopped
    depends_on:
      - prometheus
    
  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile
    ports:
      - "3000:80"
    environment:
      - ROUTER_URL=http://llm-router:8080
    restart: unless-stopped
    depends_on:
      - llm-router
    
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus-demo.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'
      - '--storage.tsdb.retention.time=200h'
      - '--web.enable-lifecycle'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3001:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${DEMO_PASSWORD}
      - GF_USERS_ALLOW_SIGN_UP=false
      - GF_USERS_ALLOW_ORG_CREATE=false
    volumes:
      - grafana_data:/var/lib/grafana
      - ./monitoring/grafana-dashboards:/etc/grafana/provisioning/dashboards:ro
      - ./monitoring/grafana-datasources:/etc/grafana/provisioning/datasources:ro
    restart: unless-stopped
    depends_on:
      - prometheus

  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./deployment/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./deployment/ssl:/etc/nginx/ssl:ro
      - certbot_data:/var/www/certbot
    restart: unless-stopped
    depends_on:
      - frontend
      - llm-router

  certbot:
    image: certbot/certbot
    volumes:
      - certbot_data:/var/www/certbot
      - ./deployment/ssl:/etc/letsencrypt
    entrypoint: "/bin/sh -c 'trap exit TERM; while :; do certbot renew; sleep 12h & wait; done;'"

volumes:
  prometheus_data:
  grafana_data:
  certbot_data:
EOF

# Create smart hybrid Dockerfile
cat > router/Dockerfile.smart-hybrid << 'EOF'
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o router .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/router .
COPY --from=builder /app/config-smart-hybrid.yaml ./config.yaml

# Create data directory for persistence
RUN mkdir -p /app/data

EXPOSE 8080
CMD ["./router", "--config", "config.yaml"]
EOF

# Create smart hybrid config
cat > router/config-smart-hybrid.yaml << EOF
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 120s
  idleTimeout: 60s

router:
  routingStrategy: smart_hybrid
  healthCheckInterval: 30s
  maxLatencyMs: 5000
  maxQueueDepth: 10
  overheadFactor: 1.1
  metricsUpdateInterval: 30s
  
  # Smart hybrid specific settings
  enableSmartMocking: true
  monthlyAPIBudget: ${API_BUDGET}
  mockClusterLatency: 200
  mockClusterCost: 0.0002

# Mock self-hosted clusters for demo
clusters:
  - name: aws-us-east-1
    endpoint: mock://aws-cluster
    region: us-east-1
    provider: aws
    costPerHour: 0.0464
    authType: mock
    
  - name: gcp-us-central1
    endpoint: mock://gcp-cluster
    region: us-central1
    provider: gcp
    costPerHour: 0.0520
    authType: mock
    
  - name: azure-eastus
    endpoint: mock://azure-cluster
    region: eastus
    provider: azure
    costPerHour: 0.0580
    authType: mock

# Real external providers (with budget controls)
externalProviders:
  - name: openai
    type: openai
    enabled: true
    apiKey: "\${OPENAI_API_KEY}"
    defaultModel: gpt-3.5-turbo
    dailyBudgetLimit: $(echo "scale=2; $API_BUDGET / 30" | bc)
    
  - name: claude
    type: claude
    enabled: true
    apiKey: "\${ANTHROPIC_API_KEY}"
    defaultModel: claude-3-haiku-20240307
    dailyBudgetLimit: $(echo "scale=2; $API_BUDGET / 30" | bc)
    
  - name: gemini
    type: gemini
    enabled: true
    apiKey: "\${GEMINI_API_KEY}"
    defaultModel: gemini-1.5-flash
    dailyBudgetLimit: $(echo "scale=2; $API_BUDGET / 30" | bc)

# Demo authentication
demo:
  enabled: true
  password: "${DEMO_PASSWORD}"
  sessionTimeout: 24h
  rateLimitPerIP: 100
EOF

# Create nginx config
mkdir -p deployment
cat > deployment/nginx.conf << EOF
events {
    worker_connections 1024;
}

http {
    upstream frontend {
        server frontend:80;
    }
    
    upstream api {
        server llm-router:8080;
    }
    
    upstream monitoring {
        server grafana:3000;
    }

    server {
        listen 80;
        server_name ${DEMO_DOMAIN};
        
        location /.well-known/acme-challenge/ {
            root /var/www/certbot;
        }
        
        location / {
            return 301 https://\$server_name\$request_uri;
        }
    }

    server {
        listen 443 ssl;
        server_name ${DEMO_DOMAIN};
        
        ssl_certificate /etc/nginx/ssl/live/${DEMO_DOMAIN}/fullchain.pem;
        ssl_certificate_key /etc/nginx/ssl/live/${DEMO_DOMAIN}/privkey.pem;
        
        # Frontend
        location / {
            proxy_pass http://frontend;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
        }
        
        # API
        location /api/ {
            proxy_pass http://api/;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_read_timeout 120s;
        }
        
        # LLM endpoints
        location /v1/ {
            proxy_pass http://api/v1/;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_read_timeout 120s;
        }
        
        # Monitoring
        location /monitoring/ {
            proxy_pass http://monitoring/;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            auth_basic "Monitoring";
            auth_basic_user_file /etc/nginx/.htpasswd;
        }
    }
}
EOF

# Create deployment script for VPS
cat > deployment/setup-vps.sh << 'EOF'
#!/bin/bash

set -e

echo "🚀 Setting up Smart Hybrid LLM Router Demo..."

# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh
usermod -aG docker $USER

# Install Docker Compose
curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Clone repository (replace with your actual repo)
git clone https://github.com/yourname/multi-cloud-llm-router.git /opt/llm-router
cd /opt/llm-router

# Set up environment
cat > .env << ENVEOF
OPENAI_API_KEY=${OPENAI_API_KEY}
ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
GEMINI_API_KEY=${GEMINI_API_KEY}
DEMO_PASSWORD=${DEMO_PASSWORD}
API_BUDGET=${API_BUDGET}
ENVEOF

# Build and start services
docker-compose -f docker-compose.smart-hybrid.yml up -d --build

# Wait for services to start
sleep 30

# Get SSL certificate
docker run --rm -v $(pwd)/deployment/ssl:/etc/letsencrypt \
  -v $(pwd)/deployment/certbot:/var/www/certbot \
  certbot/certbot certonly --webroot \
  --webroot-path=/var/www/certbot \
  --email admin@${DEMO_DOMAIN} \
  --agree-tos --no-eff-email \
  -d ${DEMO_DOMAIN}

# Restart nginx with SSL
docker-compose -f docker-compose.smart-hybrid.yml restart nginx

echo "✅ Deployment complete!"
echo "Demo URL: https://${DEMO_DOMAIN}"
echo "Password: ${DEMO_PASSWORD}"
echo "Monitoring: https://${DEMO_DOMAIN}/monitoring/"
EOF

chmod +x deployment/setup-vps.sh

print_section "VPS Provider Selection"

case $VPS_PROVIDER in
    "hetzner")
        echo "🇩🇪 Deploying to Hetzner Cloud (€2.99/month)"
        deploy_hetzner
        ;;
    "digitalocean")
        echo "🌊 Deploying to DigitalOcean (\$6/month)"
        deploy_digitalocean
        ;;
    "linode")
        echo "🔵 Deploying to Linode (\$5/month)"
        deploy_linode
        ;;
    *)
        echo "📋 Manual deployment files created"
        deploy_manual
        ;;
esac

deploy_hetzner() {
    echo -e "${BLUE}Deploying to existing Hetzner box:${NC}"
    echo "✅ Using existing server (ssh llm-box)"
    echo
    echo -e "${GREEN}📋 Commands to run on your existing VPS:${NC}"
    echo "# Copy files to server"
    echo "scp -r deployment/ llm-box:/tmp/"
    echo "scp scripts/setup-vps.sh llm-box:/tmp/"
    echo
    echo "# SSH and deploy"
    echo "ssh llm-box"
    echo "cd /tmp"
    echo "export DEMO_DOMAIN=${DEMO_DOMAIN}"
    echo "export DEMO_PASSWORD=${DEMO_PASSWORD}"
    echo "export API_BUDGET=${API_BUDGET}"
    echo "chmod +x setup-vps.sh"
    echo "./setup-vps.sh"
    echo
    echo -e "${YELLOW}💡 This will run alongside your existing self-hosted LLM${NC}"
    echo -e "${YELLOW}   - Self-hosted LLM: Uses ports 80,443,8080,9090,3000${NC}"
    echo -e "${YELLOW}   - Multi-cloud router: Uses ports 8081,8082${NC}"
    echo -e "${YELLOW}   - Traefik will route: ${DEMO_DOMAIN} → router${NC}"
}

deploy_digitalocean() {
    if ! command -v doctl &> /dev/null; then
        echo -e "${YELLOW}Install DigitalOcean CLI: https://docs.digitalocean.com/reference/doctl/how-to/install/${NC}"
        return
    fi
    
    echo "Creating DigitalOcean droplet..."
    doctl compute droplet create llm-router-demo \
        --image ubuntu-22-04-x64 \
        --size s-1vcpu-1gb \
        --region nyc1 \
        --ssh-keys $(doctl compute ssh-key list --format ID --no-header | head -n1) \
        --user-data-file deployment/setup-vps.sh \
        --wait

    IP=$(doctl compute droplet list llm-router-demo --format PublicIPv4 --no-header)
    show_completion_info $IP
}

deploy_linode() {
    echo "Manual Linode deployment:"
    echo "1. Create a 1GB Nanode (\$5/month)"
    echo "2. Upload and run deployment/setup-vps.sh"
    echo "3. Point $DEMO_DOMAIN to the server IP"
}

deploy_manual() {
    echo -e "${GREEN}✅ Smart Hybrid deployment files created!${NC}"
    echo
    echo "📁 Files created:"
    echo "• docker-compose.smart-hybrid.yml - Main deployment"
    echo "• router/Dockerfile.smart-hybrid - Router container"
    echo "• router/config-smart-hybrid.yaml - Smart hybrid config"
    echo "• deployment/nginx.conf - Web server config"
    echo "• deployment/setup-vps.sh - VPS setup script"
    echo
    echo "🚀 Manual deployment steps:"
    echo "1. Rent a \$5-10/month VPS (1GB RAM, 1 vCPU)"
    echo "2. Upload these files to the server"
    echo "3. Run: ./deployment/setup-vps.sh"
    echo "4. Point $DEMO_DOMAIN to the server IP"
    echo "5. Access: https://$DEMO_DOMAIN (password: $DEMO_PASSWORD)"
}

show_completion_info() {
    local ip=$1
    echo -e "${GREEN}✅ Server created successfully!${NC}"
    echo
    echo -e "${BLUE}📋 Next Steps:${NC}"
    echo "1. Point $DEMO_DOMAIN to IP: $ip"
    echo "2. Wait 5-10 minutes for setup to complete"
    echo "3. Access: https://$DEMO_DOMAIN"
    echo "4. Password: $DEMO_PASSWORD"
    echo
    echo -e "${BLUE}🔧 API Keys Setup:${NC}"
    echo "SSH into server and set environment variables:"
    echo "export OPENAI_API_KEY='sk-...'"
    echo "export ANTHROPIC_API_KEY='sk-ant-...'"
    echo "export GEMINI_API_KEY='...'"
    echo "docker-compose -f docker-compose.smart-hybrid.yml restart llm-router"
    echo
    echo -e "${BLUE}💰 Cost Breakdown:${NC}"
    echo "• VPS: \$5-10/month"
    echo "• Domain: \$12/year (optional)"
    echo "• External API budget: \$$API_BUDGET/month"
    echo "• Total: ~\$$(echo "$API_BUDGET + 8" | bc)/month"
    echo
    echo -e "${GREEN}🎯 Demo Features:${NC}"
    echo "• Interactive LLM playground"
    echo "• Real-time routing metrics"
    echo "• Smart cost-optimized responses"
    echo "• Professional monitoring dashboard"
    echo "• Mobile-responsive design"
}

echo
echo -e "${GREEN}Smart Hybrid deployment prepared! 🚀${NC}"
