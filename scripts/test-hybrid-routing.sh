#!/bin/bash

# Multi-Cloud LLM Router - Hybrid Routing Test Script
# This script demonstrates routing to both self-hosted clusters and external providers

set -e

ROUTER_URL="${ROUTER_URL:-http://localhost:8080}"
SLEEP_TIME="${SLEEP_TIME:-2}"

echo "🚀 Testing Multi-Cloud LLM Router with Hybrid Routing"
echo "Router URL: $ROUTER_URL"
echo "=================================="

# Function to make a request and show routing decision
test_request() {
    local description="$1"
    local model="$2" 
    local message="$3"
    local max_tokens="$4"
    
    echo
    echo "📝 Test: $description"
    echo "Model: $model, Max Tokens: $max_tokens"
    echo "Message: $message"
    echo "---"
    
    # Make the request
    response=$(curl -s -X POST "$ROUTER_URL/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model\",
            \"messages\": [{\"role\": \"user\", \"content\": \"$message\"}],
            \"max_tokens\": $max_tokens,
            \"temperature\": 0.7
        }")
    
    # Check if request was successful
    if echo "$response" | grep -q "error"; then
        echo "❌ Request failed:"
        echo "$response" | jq .
    else
        echo "✅ Request successful"
        # Extract and show the response content
        content=$(echo "$response" | jq -r '.choices[0].message.content // "No content"')
        echo "Response: $content"
    fi
    
    sleep $SLEEP_TIME
}

# Function to check router health
check_health() {
    echo "🏥 Checking router health..."
    health=$(curl -s "$ROUTER_URL/health")
    
    if echo "$health" | grep -q "healthy"; then
        echo "✅ Router is healthy"
        echo "$health" | jq .
    else
        echo "❌ Router health check failed"
        echo "$health"
        exit 1
    fi
    echo
}

# Function to show current metrics
show_metrics() {
    echo "📊 Current routing metrics:"
    curl -s "$ROUTER_URL/metrics" | grep -E "(routing_decisions|provider_health|cluster_health)" | head -10
    echo "..."
    echo
}

# Wait for router to be ready
echo "⏳ Waiting for router to be ready..."
while ! curl -s "$ROUTER_URL/health" > /dev/null 2>&1; do
    echo "  Waiting for router..."
    sleep 2
done

# Check initial health
check_health

# Test 1: Simple request (should prefer clusters if healthy and cost-effective)
test_request \
    "Simple completion (should prefer cluster)" \
    "gpt-3.5-turbo" \
    "Hello, how are you?" \
    20

# Test 2: Short coding task (might prefer cluster)
test_request \
    "Short coding task (cluster preferred)" \
    "gpt-3.5-turbo" \
    "Write a Python function to add two numbers" \
    100

# Test 3: Complex analysis (might prefer external provider)
test_request \
    "Complex analysis (may prefer external)" \
    "gpt-4" \
    "Analyze the economic implications of renewable energy adoption in developing countries. Consider infrastructure costs, job creation, and environmental benefits." \
    500

# Test 4: Large context request (might prefer external for capability)
test_request \
    "Large context request (external for capability)" \
    "claude-3-haiku" \
    "Summarize the key themes and arguments in this research paper. Focus on methodology, findings, and implications for future research." \
    300

# Test 5: Specific model request (test external provider)
test_request \
    "Specific external model request" \
    "gemini-pro" \
    "Explain quantum computing in simple terms" \
    200

# Show final metrics
show_metrics

# Show routing decisions summary
echo "📈 Routing Decisions Summary:"
curl -s "$ROUTER_URL/metrics" | grep "llm_router_routing_decisions_total" | \
    awk -F'{' '{print $2}' | awk -F'}' '{print $1}' | sort | uniq -c | \
    while read count labels; do
        echo "  $count requests: $labels"
    done

echo
echo "🎉 Hybrid routing test completed!"
echo "Check the metrics above to see how requests were routed between clusters and external providers."
echo
echo "💡 Tips:"
echo "  - Simple requests should route to clusters (cost-effective)"
echo "  - Complex requests may route to external providers (capability)"
echo "  - Routing depends on your configuration strategy and cluster health"
echo "  - Monitor costs using the Prometheus metrics"
