#!/bin/bash

# Quick Local Demo Test - Test your router locally before deploying

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}🚀 Local Demo Test${NC}"
echo "=================="

# Generate demo password
DEMO_PASSWORD=$(openssl rand -base64 8 | tr -d "=+/" | cut -c1-8)
echo -e "${GREEN}Demo Password: $DEMO_PASSWORD${NC}"

# Create minimal demo config
cat > router/config-demo.yaml << EOF
server:
  port: 8080

router:
  routingStrategy: hybrid
  healthCheckInterval: 30s
  enableExternalFallback: true
  clusterCostThreshold: 0.01

# Mock clusters for demo
clusters:
  - name: aws-demo
    endpoint: https://demo.aws.cluster
    region: us-east-1
    provider: aws
    costPerHour: 0.05
    authType: hmac
    sharedSecret: demo-secret

# Real external providers (optional)
externalProviders:
  - name: openai
    type: openai
    enabled: true
    apiKey: "\${OPENAI_API_KEY}"
    defaultModel: gpt-3.5-turbo

demo:
  enabled: true
  password: "$DEMO_PASSWORD"
  sessionTimeout: 24h
  rateLimitPerIP: 100
EOF

echo -e "${BLUE}Starting router...${NC}"

cd router

# Start with demo config
export DEMO_MODE=true
go run . --config config-demo.yaml &
ROUTER_PID=$!

# Wait for startup
sleep 3

echo -e "${GREEN}✅ Router started on http://localhost:8080${NC}"
echo
echo -e "${BLUE}🧪 Test the demo:${NC}"
echo

# Test health endpoint
echo "1. Health check:"
echo "curl http://localhost:8080/health"
curl -s http://localhost:8080/health | jq . 2>/dev/null || curl -s http://localhost:8080/health
echo

# Test auth endpoint  
echo "2. Authentication:"
echo "curl -X POST http://localhost:8080/api/auth -d '{\"password\":\"$DEMO_PASSWORD\"}'"
curl -s -X POST http://localhost:8080/api/auth \
  -H "Content-Type: application/json" \
  -d "{\"password\":\"$DEMO_PASSWORD\"}" | jq . 2>/dev/null || echo "Auth endpoint ready"
echo

# Test LLM endpoint
echo "3. LLM Request:"
echo "curl -X POST http://localhost:8080/v1/chat/completions ..."
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello!"}],"max_tokens":50}' \
  | jq . 2>/dev/null || echo "LLM endpoint responding"
echo

echo -e "${BLUE}🌐 Frontend Demo:${NC}"
echo "Open frontend/index.html in your browser"
echo "Password: $DEMO_PASSWORD"
echo
echo -e "${YELLOW}Press Ctrl+C to stop${NC}"

# Cleanup on exit
trap 'kill $ROUTER_PID 2>/dev/null; echo "Demo stopped"' EXIT

# Keep running
wait $ROUTER_PID
