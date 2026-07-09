#!/bin/bash
set -e

echo "==> Tearing down WatchGPT..."
kubectl delete namespace watchgpt --ignore-not-found
echo "==> Done. All resources removed."
