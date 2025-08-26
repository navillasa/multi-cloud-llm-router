#!/bin/bash

# SSL Setup Script for LLM Router Demo
set -e

# Load environment variables
source .env

echo "🔒 Setting up SSL for domain: $DEMO_DOMAIN"

# Stop existing containers to get certificates
echo "📋 Stopping existing services..."
docker compose -f docker-compose.smart-hybrid.yml down

# Create directories for SSL
mkdir -p deployment/ssl

# Start minimal nginx for certificate challenge
echo "🌐 Starting temporary nginx for certificate challenge..."
docker run -d --name temp-nginx \
  -p 80:80 \
  -v $(pwd)/deployment/ssl:/etc/letsencrypt \
  -v $(pwd)/certbot_data:/var/www/certbot \
  nginx:alpine \
  sh -c 'echo "server { listen 80; location /.well-known/acme-challenge/ { root /var/www/certbot; } location / { return 301 https://\$server_name\$request_uri; } }" > /etc/nginx/conf.d/default.conf && nginx -g "daemon off;"'

# Wait for nginx to start
sleep 5

# Get SSL certificate
echo "📜 Obtaining SSL certificate from Let's Encrypt..."
docker run --rm \
  -v $(pwd)/deployment/ssl:/etc/letsencrypt \
  -v $(pwd)/certbot_data:/var/www/certbot \
  certbot/certbot certonly \
  --webroot \
  --webroot-path=/var/www/certbot \
  --email admin@${DEMO_DOMAIN} \
  --agree-tos \
  --no-eff-email \
  -d ${DEMO_DOMAIN}

# Stop temporary nginx
docker stop temp-nginx
docker rm temp-nginx

# Start the full application with SSL
echo "🚀 Starting application with SSL enabled..."
docker compose -f docker-compose.smart-hybrid.yml up -d

echo "✅ SSL setup complete!"
echo "🌐 Your demo is now available at: https://$DEMO_DOMAIN"
echo "🔐 Password: $DEMO_PASSWORD"
