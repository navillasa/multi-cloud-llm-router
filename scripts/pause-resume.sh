#!/bin/bash

# Multi-Cloud LLM Router - Pause/Resume Script
# Scale everything to zero to minimize costs, resume for demos

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

ACTION=${1:-status}
CLUSTER_NAME=${2:-$(kubectl config current-context | cut -d'/' -f2 2>/dev/null || echo "")}

case $ACTION in
    "pause"|"stop"|"sleep")
        echo -e "${YELLOW}😴 Pausing cluster to save costs...${NC}"
        
        # Scale LLM server to zero
        kubectl scale deployment -n llm-server llm-server --replicas=0 2>/dev/null || echo "LLM server not found"
        
        # Scale Argo CD to zero
        kubectl scale deployment -n argocd argocd-server --replicas=0 2>/dev/null || echo "Argo CD not found"
        kubectl scale deployment -n argocd argocd-repo-server --replicas=0 2>/dev/null || echo "Argo CD repo server not found"
        kubectl scale deployment -n argocd argocd-dex-server --replicas=0 2>/dev/null || echo "Argo CD dex not found"
        
        # Scale Grafana to zero (keep Prometheus for when we resume)
        kubectl scale deployment -n monitoring prometheus-grafana --replicas=0 2>/dev/null || echo "Grafana not found"
        
        # Scale cert-manager to zero
        kubectl scale deployment -n cert-manager cert-manager --replicas=0 2>/dev/null || echo "cert-manager not found"
        kubectl scale deployment -n cert-manager cert-manager-webhook --replicas=0 2>/dev/null || echo "cert-manager webhook not found"
        kubectl scale deployment -n cert-manager cert-manager-cainjector --replicas=0 2>/dev/null || echo "cert-manager cainjector not found"
        
        echo -e "${GREEN}✅ Cluster paused! Costs minimized to just EKS control plane (~$73/month)${NC}"
        echo -e "${BLUE}💡 To resume: ./scripts/pause-resume.sh resume${NC}"
        ;;
        
    "resume"|"wake"|"start")
        echo -e "${GREEN}🚀 Resuming cluster for demo/development...${NC}"
        
        # Resume cert-manager first (needed for SSL)
        kubectl scale deployment -n cert-manager cert-manager --replicas=1 2>/dev/null || echo "cert-manager not found"
        kubectl scale deployment -n cert-manager cert-manager-webhook --replicas=1 2>/dev/null || echo "cert-manager webhook not found"
        kubectl scale deployment -n cert-manager cert-manager-cainjector --replicas=1 2>/dev/null || echo "cert-manager cainjector not found"
        
        # Resume Grafana
        kubectl scale deployment -n monitoring prometheus-grafana --replicas=1 2>/dev/null || echo "Grafana not found"
        
        # Resume Argo CD
        kubectl scale deployment -n argocd argocd-server --replicas=1 2>/dev/null || echo "Argo CD not found"
        kubectl scale deployment -n argocd argocd-repo-server --replicas=1 2>/dev/null || echo "Argo CD repo server not found"
        kubectl scale deployment -n argocd argocd-dex-server --replicas=1 2>/dev/null || echo "Argo CD dex not found"
        
        # Resume LLM server
        kubectl scale deployment -n llm-server llm-server --replicas=1 2>/dev/null || echo "LLM server not found"
        
        echo -e "${YELLOW}⏳ Waiting for services to be ready...${NC}"
        
        # Wait for LLM server
        echo "Waiting for LLM server..."
        kubectl wait --for=condition=Available deployment/llm-server -n llm-server --timeout=300s 2>/dev/null || echo "LLM server not ready yet"
        
        echo -e "${GREEN}✅ Cluster resumed! Ready for demos.${NC}"
        echo -e "${BLUE}💡 To pause again: ./scripts/pause-resume.sh pause${NC}"
        ;;
        
    "demo")
        echo -e "${BLUE}🎭 Quick demo setup...${NC}"
        
        # Resume everything
        $0 resume
        
        # Get endpoint info
        echo -e "\n${GREEN}🌐 Demo endpoints:${NC}"
        
        if kubectl get ingress -n llm-server llm-server 2>/dev/null; then
            ENDPOINT=$(kubectl get ingress -n llm-server llm-server -o jsonpath='{.spec.rules[0].host}' 2>/dev/null)
            echo "LLM API: https://$ENDPOINT"
            echo "Health: https://$ENDPOINT/health"
        else
            echo "LLM API: kubectl port-forward -n llm-server svc/llm-server 8080:8080"
            echo "Then: http://localhost:8080"
        fi
        
        echo "Grafana: kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80"
        echo "Prometheus: kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090"
        
        echo -e "\n${YELLOW}💡 Remember to pause after demo: ./scripts/pause-resume.sh pause${NC}"
        ;;
        
    "status")
        echo -e "${BLUE}📊 Cluster Status:${NC}"
        echo "==================="
        
        echo -e "\n${BLUE}🖥️  LLM Server:${NC}"
        kubectl get deployment -n llm-server llm-server -o jsonpath='{.status.replicas}/{.status.readyReplicas}' 2>/dev/null && echo " replicas" || echo "Not deployed"
        
        echo -e "\n${BLUE}📊 Monitoring:${NC}"
        kubectl get deployment -n monitoring prometheus-grafana -o jsonpath='{.status.replicas}/{.status.readyReplicas}' 2>/dev/null && echo " Grafana replicas" || echo "Grafana not found"
        
        echo -e "\n${BLUE}🔄 Argo CD:${NC}"
        kubectl get deployment -n argocd argocd-server -o jsonpath='{.status.replicas}/{.status.readyReplicas}' 2>/dev/null && echo " ArgoCD replicas" || echo "ArgoCD not found"
        
        echo -e "\n${BLUE}🔒 Cert Manager:${NC}"
        kubectl get deployment -n cert-manager cert-manager -o jsonpath='{.status.replicas}/{.status.readyReplicas}' 2>/dev/null && echo " cert-manager replicas" || echo "cert-manager not found"
        
        echo -e "\n${BLUE}💰 Current State:${NC}"
        LLMSERVER_REPLICAS=$(kubectl get deployment -n llm-server llm-server -o jsonpath='{.status.replicas}' 2>/dev/null || echo "0")
        if [ "$LLMSERVER_REPLICAS" = "0" ]; then
            echo -e "${GREEN}🟢 PAUSED - Minimal cost mode (~$73/month EKS only)${NC}"
        else
            echo -e "${YELLOW}🟡 ACTIVE - Full cost mode (~$100/month)${NC}"
        fi
        ;;
        
    "nuke"|"destroy")
        echo -e "${RED}💥 DESTROYING EVERYTHING (This will delete all resources!)${NC}"
        read -p "Are you sure? Type 'yes' to continue: " confirm
        if [ "$confirm" = "yes" ]; then
            echo "Destroying infrastructure..."
            cd infra/aws
            pulumi destroy --yes
            cd ../..
            echo -e "${RED}💥 Everything destroyed. Costs: $0/month${NC}"
        else
            echo "Cancelled."
        fi
        ;;
        
    *)
        echo -e "${BLUE}🛠️  Multi-Cloud LLM Router - Pause/Resume Script${NC}"
        echo "=================================================="
        echo
        echo "Usage: $0 <command>"
        echo
        echo "Commands:"
        echo "  pause   - Scale everything to zero (save costs)"
        echo "  resume  - Scale everything back up (ready for demos)"
        echo "  demo    - Quick resume + show endpoints"
        echo "  status  - Show current state"
        echo "  nuke    - Destroy everything (costs: $0)"
        echo
        echo "Examples:"
        echo "  $0 pause    # Before going to bed"
        echo "  $0 demo     # Before an interview"
        echo "  $0 status   # Check current state"
        echo
        echo "💰 Cost Modes:"
        echo "  PAUSED:  ~$73/month (EKS control plane only)"
        echo "  ACTIVE:  ~$100/month (full deployment)"
        echo "  NUKED:   $0/month (everything deleted)"
        ;;
esac
