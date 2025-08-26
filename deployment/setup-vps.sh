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
