#!/bin/bash
set -e

echo "==> Building Docker image..."
docker build -t watchgpt-backend:latest .

echo "==> Creating namespace..."
kubectl apply -f k8s/namespace.yaml

echo "==> Checking ingress dependencies..."
if ! kubectl get ingressclass nginx >/dev/null 2>&1; then
  echo "Missing nginx ingress class. Install nginx ingress controller before applying k8s/ingress.yaml."
  exit 1
fi

if ! kubectl api-resources | grep -q '^clusterissuers[[:space:]]'; then
  echo "Missing cert-manager CRDs. Install cert-manager before applying k8s/cert-manager-issuer.yaml."
  exit 1
fi

echo "==> Applying secrets and config..."
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/configmap.yaml

echo "==> Deploying backend..."
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/hpa.yaml

echo "==> Applying ingress and TLS resources..."
kubectl apply -f k8s/cert-manager-issuer.yaml
kubectl apply -f k8s/ingress.yaml

echo "==> Waiting for pods to be ready..."
kubectl -n watchgpt rollout status deployment/watchgpt-backend --timeout=60s

echo ""
echo "==> Deployment complete!"
echo ""
kubectl -n watchgpt get pods
echo ""
kubectl -n watchgpt get svc
echo ""
kubectl -n watchgpt get ingress
echo ""
echo "Backend ingress host: https://api.watchgpt.example.com"
echo "Replace api.watchgpt.example.com and admin@example.com before production use."
