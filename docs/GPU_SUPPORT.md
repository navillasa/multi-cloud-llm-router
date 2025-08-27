# GPU Support for Multi-Cloud LLM Router

This document describes how to enable GPU acceleration across AWS, Azure, and GCP clusters for running larger, more powerful LLM models.

## Overview

The multi-cloud LLM router includes GPU support that can be enabled on-demand for models that require GPU acceleration. The system is designed with cost optimization in mind, using spot/preemptible instances and scale-to-zero capabilities.

## Architecture

### GPU Node Pools
Each cloud provider has dedicated GPU node pools that are separate from the standard CPU nodes:

- **AWS**: g4dn.xlarge instances with NVIDIA Tesla T4 GPUs
- **Azure**: Standard_NC4as_T4_v3 instances with NVIDIA Tesla T4 GPUs  
- **GCP**: n1-standard-4 + nvidia-tesla-t4 preemptible instances

### Scaling Strategy
- Nodes start at 0 replicas (scale-to-zero for cost optimization)
- Auto-scaling based on GPU utilization and queue depth
- Maximum 2-3 replicas per cloud for cost control
- Spot/preemptible instances for 60-80% cost savings

## Cost Comparison

### Monthly Estimates (24/7 operation)

#### CPU-Only Deployment
- AWS: ~$21/month (t3.small spot)
- Azure: ~$21/month (Standard_B2s spot)  
- GCP: ~$21/month (e2-standard-2 preemptible)
- **Total: ~$63/month**

#### GPU-Enabled Deployment  
- AWS: ~$114/month (g4dn.xlarge spot)
- Azure: ~$151/month (Standard_NC4as_T4_v3 spot)
- GCP: ~$201/month (n1-standard-4 + T4 preemptible)
- **Total: ~$466/month**

#### Scale-to-Zero GPU (Recommended)
- Base cost: ~$63/month (CPU nodes always running)
- GPU cost: Only when processing GPU-requiring models
- Typical usage: ~$100-200/month depending on workload

## Enabling GPU Support

### Method 1: Environment Variables (Recommended)
```bash
# Enable GPU for specific deployments
export GPU_ENABLED=true
export GPU_INSTANCE_TYPE=g4dn.xlarge  # AWS
export MAX_GPU_REPLICAS=2

# Deploy with GPU overlay
kubectl apply -k clusters/aws/overlays/gpu/
```

### Method 2: Helm Values Override
```yaml
# values-gpu.yaml
gpu:
  enabled: true
  resourceName: "nvidia.com/gpu"
  count: 1
  
llamacpp:
  ngl: -1  # Use all GPU layers
  flash_attn: true
  ctx_size: 4096
```

### Method 3: Kustomize Overlays
Pre-configured GPU overlays are available for each cloud:
```bash
# AWS GPU deployment
kubectl apply -k clusters/aws/overlays/gpu/

# Azure GPU deployment  
kubectl apply -k clusters/azure/overlays/gpu/

# GCP GPU deployment
kubectl apply -k clusters/gcp/overlays/gpu/
```

## Model Configuration

### GPU-Optimized Models
When GPU is enabled, the system automatically switches to larger, more capable models:

**Default (CPU)**: TinyLlama 1.1B (~1GB VRAM)
**GPU-Enabled**: Llama-2 7B Chat (~4GB VRAM)

### Custom Model Configuration
```yaml
model:
  uri: "https://huggingface.co/TheBloke/Llama-2-13B-Chat-GGUF/resolve/main/llama-2-13b-chat.q4_k_m.gguf"
  path: "/models/llama-2-13b-chat.gguf"
  storageSize: "30Gi"  # Larger models need more storage

llamacpp:
  ngl: -1  # Offload all layers to GPU
  ctx_size: 4096  # Larger context window
  parallel: 8     # More parallel requests
```

## Infrastructure Setup

### 1. Create GPU Node Pools

#### AWS (using Pulumi)
```go
// Enable GPU node group creation
gpuNodeGroup, err := createGPUNodeGroup(ctx, cluster, vpcConfig)
if err != nil {
    return err
}
```

#### Manual kubectl setup
```bash
# Apply NVIDIA device plugin
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.14.1/nvidia-device-plugin.yml

# Verify GPU nodes
kubectl get nodes -l accelerator=nvidia-tesla-t4
```

### 2. Install NVIDIA Drivers
GPU nodes automatically install NVIDIA drivers via:
- AWS: EKS-optimized AMI with GPU support
- Azure: AKS GPU-enabled node pools  
- GCP: COS with GPU support

### 3. Configure Runtime Classes
```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: nvidia
handler: nvidia
```

## Monitoring and Observability

### GPU Metrics
The platform automatically exposes GPU metrics via:
- NVIDIA DCGM Exporter
- Prometheus GPU metrics
- Grafana GPU dashboards

### Key Metrics
- GPU utilization percentage
- GPU memory usage
- Model inference latency  
- Queue depth and wait times
- Cost per inference

### Alerting
```yaml
# GPU High Utilization Alert
- alert: GPUHighUtilization
  expr: nvidia_gpu_utilization > 90
  for: 5m
  annotations:
    summary: "GPU utilization high on {{ $labels.instance }}"
    
# GPU Cost Alert  
- alert: GPUCostAlert
  expr: increase(gpu_cost_usd[24h]) > 50
  annotations:
    summary: "GPU costs exceeded $50 in 24h"
```

## Routing Logic

The LLM router automatically determines when to use GPU nodes:

```go
// Model routing decision
func shouldUseGPU(modelSize int, complexity string) bool {
    // Use GPU for models > 3B parameters
    if modelSize > 3_000_000_000 {
        return true
    }
    
    // Use GPU for complex tasks
    if complexity == "reasoning" || complexity == "code-generation" {
        return true
    }
    
    return false
}
```

## Deployment Commands

### Quick Start (GPU Enabled)
```bash
# 1. Enable GPU node pools
pulumi config set gpu:enabled true
pulumi up

# 2. Deploy GPU-capable LLM servers
kubectl apply -k clusters/aws/overlays/gpu/
kubectl apply -k clusters/azure/overlays/gpu/  
kubectl apply -k clusters/gcp/overlays/gpu/

# 3. Verify GPU availability
kubectl get nodes -l compute=gpu-enabled
kubectl describe node <gpu-node-name>
```

### Testing GPU Workloads
```bash
# Test GPU pod
kubectl run gpu-test --image=nvidia/cuda:11.0-runtime-ubuntu20.04 --rm -it --restart=Never -- nvidia-smi

# Test LLM inference on GPU
curl -X POST http://aws-gpu.llm.yourdomain.com/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-2-7b-chat", 
    "messages": [{"role": "user", "content": "Explain quantum computing"}],
    "max_tokens": 500
  }'
```

## Troubleshooting

### Common Issues

1. **GPU Not Detected**
   ```bash
   kubectl logs -n kube-system -l name=nvidia-device-plugin-daemonset
   ```

2. **CUDA Out of Memory**
   - Reduce model size or context length
   - Lower parallel request count
   - Use quantized models (Q4_K_M instead of F16)

3. **High GPU Costs**
   - Verify scale-to-zero is working
   - Check autoscaling metrics
   - Consider smaller GPU instances

### Debugging Commands
```bash
# Check GPU resource allocation
kubectl describe node <gpu-node> | grep nvidia.com/gpu

# Monitor GPU utilization
kubectl top node <gpu-node> --use-protocol-buffers

# View GPU pod logs
kubectl logs -n llm-server deployment/gpu-llm-server
```

## Best Practices

1. **Cost Optimization**
   - Use spot/preemptible instances
   - Enable scale-to-zero
   - Set appropriate resource limits
   - Monitor usage patterns

2. **Performance**
   - Use quantized models when possible
   - Optimize batch sizes
   - Enable tensor parallelism for large models
   - Cache popular models

3. **Reliability**
   - Configure node affinity rules
   - Use multiple availability zones
   - Implement graceful degradation to CPU
   - Monitor GPU health metrics

## Future Enhancements

- [ ] Multi-GPU support for larger models
- [ ] Model parallelism across GPUs
- [ ] Dynamic model loading based on request
- [ ] Advanced cost optimization with predictive scaling
- [ ] Integration with model serving frameworks (vLLM, TensorRT-LLM)
