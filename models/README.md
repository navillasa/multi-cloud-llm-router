# Model Registry

This directory contains metadata about available models and their configurations.

## Available Models

### TinyLlama 1.1B Chat (Recommended for testing)
- **Size**: ~1.1GB (Q4_K_M quantization)
- **Context**: 2048 tokens
- **Performance**: ~15-20 tokens/sec on 2 vCPU
- **Use case**: Development, testing, light workloads

```yaml
model:
  name: tinyllama-1.1b-chat
  uri: "https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.q4_k_m.gguf"
  size: "1.1GB"
  quantization: "Q4_K_M"
  context_size: 2048
  estimated_tps: 15
```

### Phi-3 Mini (Good performance/size ratio)
- **Size**: ~2.3GB (Q4_K_M quantization)
- **Context**: 4096 tokens
- **Performance**: ~10-15 tokens/sec on 2 vCPU
- **Use case**: Production workloads, better quality

```yaml
model:
  name: phi-3-mini
  uri: "https://huggingface.co/microsoft/Phi-3-mini-4k-instruct-gguf/resolve/main/Phi-3-mini-4k-instruct-q4.gguf"
  size: "2.3GB"
  quantization: "Q4_K_M"
  context_size: 4096
  estimated_tps: 12
```

### Llama 2 7B Chat (Production quality)
- **Size**: ~4.1GB (Q4_K_M quantization)
- **Context**: 4096 tokens
- **Performance**: ~5-8 tokens/sec on 4 vCPU
- **Use case**: High-quality production workloads

```yaml
model:
  name: llama2-7b-chat
  uri: "https://huggingface.co/TheBloke/Llama-2-7B-Chat-GGUF/resolve/main/llama-2-7b-chat.q4_k_m.gguf"
  size: "4.1GB"
  quantization: "Q4_K_M"
  context_size: 4096
  estimated_tps: 6
```

## Quantization Levels

| Level | Size | Quality | Speed | Notes |
|-------|------|---------|--------|-------|
| Q4_K_M | Smallest | Good | Fastest | Recommended for CPU |
| Q5_K_M | Medium | Better | Medium | Good balance |
| Q8_0 | Large | Best | Slowest | GPU recommended |

## Cost Estimates (per 1K tokens)

Based on t3.large (2 vCPU, 8GB RAM) at $0.0928/hour:

| Model | TPS | Cost/1K tokens | Notes |
|-------|-----|----------------|-------|
| TinyLlama 1.1B | 15 | $0.0017 | Very economical |
| Phi-3 Mini | 12 | $0.0021 | Good balance |
| Llama 2 7B | 6 | $0.0043 | Higher quality |

## Storage Requirements

- Models are downloaded once per node and cached in PVC
- Use `ReadWriteOnce` PVC with sufficient storage
- Consider using faster storage classes (gp3, SSD) for better loading times

## Recommended Instance Types

### AWS
- **t3.large** (2 vCPU, 8GB): TinyLlama, Phi-3 Mini
- **t3.xlarge** (4 vCPU, 16GB): Llama 2 7B
- **c5.xlarge** (4 vCPU, 8GB): CPU-optimized alternative

### GCP
- **n1-standard-2** (2 vCPU, 7.5GB): TinyLlama, Phi-3 Mini
- **n1-standard-4** (4 vCPU, 15GB): Llama 2 7B
- **c2-standard-4** (4 vCPU, 16GB): CPU-optimized

### Azure
- **Standard_D2s_v3** (2 vCPU, 8GB): TinyLlama, Phi-3 Mini
- **Standard_D4s_v3** (4 vCPU, 16GB): Llama 2 7B
- **Standard_F4s_v2** (4 vCPU, 8GB): CPU-optimized
