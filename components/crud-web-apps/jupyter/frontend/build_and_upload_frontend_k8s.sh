#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$SCRIPT_DIR"

echo "[1/2] Building frontend"
npm run build

echo "[2/2] Uploading frontend to Kubernetes"
sudo ./upload_frontend_k8s.sh
