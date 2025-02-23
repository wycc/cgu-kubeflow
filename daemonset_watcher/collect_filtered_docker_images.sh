#!/bin/bash
set -euo pipefail

# YAML 檔案名稱
YAML_FILE="docker-images-daemonset.yaml"

# 產生 DaemonSet 的 YAML 檔案
cat <<'EOF' > ${YAML_FILE}
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: docker-images
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: docker-images
  template:
    metadata:
      labels:
        app: docker-images
    spec:
      containers:
      - name: list-images
        image: docker:latest
        command:
          - /bin/sh
          - -c
          - |
            echo "Node $(hostname) - Listing images via docker images (filter: cguaicadmin):"
            docker images | grep '^cguaicadmin' || echo "No images starting with cguaicadmin found."
            # 保持 Pod 存活一段時間，方便日後收集 logs
            sleep 3600
        volumeMounts:
        - name: docker-socket
          mountPath: /var/run/docker.sock
        securityContext:
          privileged: true
      volumes:
      - name: docker-socket
        hostPath:
          path: /var/run/docker.sock
          type: Socket
EOF

echo "已產生 YAML 檔案：${YAML_FILE}"
echo "部署 DaemonSet 到 kube-system namespace..."
kubectl apply -f ${YAML_FILE}

echo "等待 DaemonSet 的 Pod 全部就緒..."
kubectl rollout status ds/docker-images -n kube-system

echo "收集每個節點上符合條件 (repository 以 cguaicadmin 開頭) 的 docker images 列表："
# 取得所有屬於 DaemonSet 的 Pod 名稱
PODS=$(kubectl get pods -n kube-system -l app=docker-images -o jsonpath='{.items[*].metadata.name}')

for pod in $PODS; do
  # 取得該 Pod 所在的 Node 名稱
  node=$(kubectl get pod ${pod} -n kube-system -o jsonpath='{.spec.nodeName}')
  echo "==================================="
  echo "Node: ${node}"
  echo "-----------------------------------"
  # 取得該 Pod 的 log（預設 container 名稱為 list-images）
  kubectl logs ${pod} -n kube-system -c list-images
  echo ""
done

echo "---------------------------------------------"
echo "所有節點上符合條件的 docker images 列表已收集完畢。"
echo "如果不再需要該 DaemonSet，可執行下列指令刪除："
echo "kubectl delete -f ${YAML_FILE}"
