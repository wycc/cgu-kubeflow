#!/bin/bash
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.26/deb/ /" | sudo tee /etc/apt/sources.list.d/kubernetes.list
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.26/deb/Release.key | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
apt update
VER=1.26
INSTALLED_VER=$(apt-cache policy kubelet | grep Installed | awk '{print $2}' | cut -d: -f2 | cut -d. -f1-2)
if [ "$INSTALLED_VER" == "$VER" ]; then
    echo "Kubelet version 1.26 is already installed, skipping."
else
    VER=$(apt-cache policy kubelet | grep -v 'Candidate' | grep $VER | awk '{print $1}' | head -n1)
    apt install -y kubelet=$VER kubeadm=$VER kubectl=$VER
    apt-mark hold kubelet kubeadm kubectl
fi
systemctl restart docker
swapoff -a
sed -i '/ swap / s/^/#/' /etc/fstab
sed -i '/swap.img/ s/^/#/' /etc/fstab
K8S_API_IP=$(hostname -I | awk '{print $1}')
K8S_POD_CIDR="192.168.0.0/16"
K8S_VER="$(kubeadm version -o yaml | grep gitVersion | awk '{printf $2}')"
kubeadm init --v=2  \
  --apiserver-advertise-address "$K8S_API_IP" \
  --pod-network-cidr="$K8S_POD_CIDR" \
  --kubernetes-version "$K8S_VER" \
  --cri-socket unix:///var/run/cri-dockerd.sock
mkdir -p $HOME/.kube
cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
chown $(id -u):$(id -g) $HOME/.kube/config
MAX_PODS=254
if grep -q "maxPods:" /var/lib/kubelet/config.yaml; then
  sed -i "s/maxPods: [0-9]*/maxPods: $MAX_PODS/" /var/lib/kubelet/config.yaml
else
  echo "maxPods: $MAX_PODS" >> /var/lib/kubelet/config.yaml
fi
systemctl restart kubelet