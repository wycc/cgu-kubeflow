#!/usr/bin/env bash

set -euo pipefail

TAG="${TAG:-v$(date +%y%m%d%H%M)}"
IMG="${IMG:-cguaicadmin/jupyter}"
NAMESPACE="${NAMESPACE:-kubeflow}"
DEPLOYMENT="${DEPLOYMENT:-jupyter-web-app-deployment}"
CONTAINER="${CONTAINER:-jupyter-web-app}"

echo "[1/4] Build image: ${IMG}:${TAG}"
docker build -t "${IMG}:${TAG}" . --no-cache

echo "[2/4] Push image to Docker Hub: ${IMG}:${TAG}"
docker push "${IMG}:${TAG}"

echo "[3/4] Update deployment image: ${DEPLOYMENT}/${CONTAINER}"
kubectl -n "${NAMESPACE}" set image deployment/"${DEPLOYMENT}" \
	"${CONTAINER}=${IMG}:${TAG}"

echo "[4/4] Rollout restart deployment: ${DEPLOYMENT}"
kubectl -n "${NAMESPACE}" rollout restart deployment/"${DEPLOYMENT}"
kubectl -n "${NAMESPACE}" rollout status deployment/"${DEPLOYMENT}"

echo "Done."
echo "Image: ${IMG}:${TAG}"

