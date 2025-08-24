# Development Guide

This guide helps you get started developing and testing the multi-cloud LLM router.

## Quick Start

1. **Run the setup script:**
   ```bash
   ./scripts/setup.sh
   ```

2. **Choose option 2** for local development

3. **Start the router:**
   ```bash
   cd router
   ./router --config config-dev.yaml
   ```

## Development Workflow

### 1. Local Router Development

The router can be developed and tested locally without deploying to Kubernetes:

```bash
cd router
go run main.go --config config-dev.yaml
```

**Testing the router:**
```bash
# Health check
curl http://localhost:8080/health

# Test completion endpoint (will fail without backend)
curl -X POST http://localhost:8080/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "test", "prompt": "Hello", "max_tokens": 10}'

# Check metrics
curl http://localhost:8080/metrics
```

### 2. Deploy Single Cloud (AWS)

For testing with real infrastructure:

```bash
cd infra/aws
pulumi stack init dev
pulumi config set multi-cloud-llm-aws:domainName "dev.llm.yourdomain.com"
pulumi config set multi-cloud-llm-aws:environment "dev"
pulumi up
```

### 3. Test with a Real LLM Backend

You can test with llama.cpp locally:

```bash
# Download a small model
mkdir -p models
cd models
wget https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.q4_k_m.gguf

# Run llama.cpp server (if you have it installed)
llama-cpp-server --model tinyllama-1.1b-chat-v1.0.q4_k_m.gguf --port 8081 --host 0.0.0.0
```

Update your `config-dev.yaml` to point to this local server.

## Project Structure

```
├── infra/                  # Infrastructure as Code
│   ├── aws/               # AWS EKS deployment
│   ├── gcp/               # GCP GKE deployment (TODO)
│   ├── azure/             # Azure AKS deployment (TODO)
│   └── common/            # Shared utilities
├── router/                # Router application
│   ├── internal/cost/     # Cost calculation engine
│   ├── internal/health/   # Health checking
│   ├── internal/forward/  # Request forwarding
│   └── main.go           # Main application
├── charts/               # Helm charts
│   ├── llm-server/       # LLM server deployment
│   ├── platform/         # Core platform (ingress, prometheus)
│   └── exporters/        # Metrics exporters
└── clusters/            # GitOps configurations
    └── aws/overlays/prod/
```

## Configuration

### Router Configuration

The router uses YAML configuration:

```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 120s

router:
  stickinessWindow: 60s        # How long to stick to a cluster
  healthCheckInterval: 30s     # How often to check health
  maxLatencyMs: 5000          # Max acceptable latency
  maxQueueDepth: 10           # Max acceptable queue depth
  overheadFactor: 1.1         # Cost calculation overhead

clusters:
  - name: aws-us-east-1
    endpoint: https://aws.llm.yourdomain.com
    provider: aws
    costPerHour: 0.0928
    authType: hmac              # or mtls
    sharedSecret: secret
```

### Authentication

Two auth types are supported:

1. **HMAC** - Shared secret with timestamp
2. **mTLS** - Mutual TLS certificates

## Testing

### Unit Tests

```bash
cd router
go test ./...
```

### Integration Tests

```bash
# Start router in background
cd router
./router --config config-dev.yaml &
ROUTER_PID=$!

# Run tests
go test -tags=integration ./tests/

# Cleanup
kill $ROUTER_PID
```

### Load Testing

Use k6 for load testing:

```bash
# Install k6
brew install k6

# Run load test
k6 run tests/load-test.js
```

## Monitoring

### Metrics

The router exposes Prometheus metrics at `/metrics`:

- `llm_router_requests_total` - Total requests by cluster and status
- `llm_router_request_duration_seconds` - Request duration
- `llm_router_cluster_health` - Cluster health status
- `llm_router_cluster_cost_per_1k_tokens` - Cost per 1K tokens
- `llm_router_routing_decisions_total` - Routing decisions

### Logs

The router uses structured JSON logging. Set log level:

```bash
export LOG_LEVEL=debug
./router --config config.yaml
```

## Deployment

### Local Development
```bash
cd router
go run main.go --config config-dev.yaml
```

### Docker Build
```bash
docker build -t llm-router .
docker run -p 8080:8080 -v $(pwd)/config.yaml:/config.yaml llm-router
```

### Kubernetes
The router is designed to run on Kubernetes but can also run on:
- Hetzner VMs
- Cloudflare Workers (with some modifications)
- Any container platform

## Troubleshooting

### Common Issues

1. **Router can't connect to clusters**
   - Check cluster endpoints are accessible
   - Verify authentication configuration
   - Check firewall rules

2. **High latency routing decisions**
   - Increase health check interval
   - Check cluster response times
   - Review cost calculation overhead

3. **No healthy clusters**
   - Check cluster health endpoints
   - Verify health check configuration
   - Review cluster logs

### Debug Mode

Enable debug logging:
```bash
export LOG_LEVEL=debug
./router --config config.yaml
```

### Health Check

Check router status:
```bash
curl http://localhost:8080/health
```

Example response:
```json
{
  "status": "healthy",
  "healthy_clusters": 2,
  "total_clusters": 3,
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run `go fmt` and `go vet`
6. Submit a pull request

## Next Steps

- [ ] Implement GCP and Azure infrastructure
- [ ] Add cost estimation refinements
- [ ] Implement circuit breaker pattern
- [ ] Add request queueing
- [ ] Implement A/B testing capabilities
- [ ] Add OpenTelemetry tracing
