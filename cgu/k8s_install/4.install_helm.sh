#!/bin/bash
REPO_KEY="/usr/share/keyrings/helm.gpg"
REPO_CONF="deb [arch=$(dpkg --print-architecture) signed-by=$REPO_KEY] https://baltocdn.com/helm/stable/debian/ all main"
curl https://baltocdn.com/helm/signing.asc | sudo gpg --dearmor --yes -o /usr/share/keyrings/helm.gpg
echo "$REPO_CONF" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
apt update
apt install helm
kubectl taint nodes $(hostname -s) node-role.kubernetes.io/control-plane-
helm repo add nfs-subdir-external-provisioner https://kubernetes-sigs.github.io/nfs-subdir-external-provisioner/
NASIP=120.126.23.7
NASPATH=/kflow_dev
helm install nfs-subdir-external-provisioner nfs-subdir-external-provisioner/nfs-subdir-external-provisioner \
    --namespace kf-storage \
    --create-namespace \
    --set nfs.server=$NASIP \
    --set nfs.path=$NASPATH \
    --version 4.0.13
kubectl annotate sc nfs-client storageclass.kubernetes.io/is-default-class=true
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update prometheus-community
helm install kube-prom-stack prometheus-community/kube-prometheus-stack \
    --namespace prometheus \
    --create-namespace
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
helm repo update metrics-server
cat <<EOF | tee metrics-server.yaml
args:
- --kubelet-insecure-tls
- --kubelet-preferred-address-types=InternalIP
EOF
helm install metrics-server metrics-server/metrics-server --namespace kube-system -f metrics-server.yaml
sleep 5