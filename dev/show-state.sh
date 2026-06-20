#!/usr/bin/env bash
set -euo pipefail

echo "==> Nodes"
kubectl get nodes

echo
echo "==> argocd namespace"
kubectl get all,imageupdaters,applications,appprojects -n argocd

echo
echo "==> Application kustomize image override"
kubectl -n argocd get application demo-app -o jsonpath='{.spec.source.kustomize.images}{"\n"}' 2>/dev/null || echo "(no application)"

echo
echo "==> demo workload"
kubectl -n default get deploy,pods -l app=demo-app 2>/dev/null || echo "(no workload)"

echo
echo "==> image-updater controller (last 15 lines)"
kubectl -n argocd logs deploy/argocd-image-updater-controller --tail=15 2>/dev/null || true
