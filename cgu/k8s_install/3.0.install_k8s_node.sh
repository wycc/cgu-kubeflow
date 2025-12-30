#!/bin/bash
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.26/deb/ /" | sudo tee /etc/apt/sources.list.d/kubernetes.list
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.26/deb/Release.key | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
apt update
VER=1.26
INSTALLED_VER=$(apt-cache policy kubelet | grep Installed | awk '{print $2}' | cut -d: -f2 | cut -d. -f1-2)
if [ "$INSTALLED_VER" == "$VER" ]; then
    echo "Kubelet version 26 is already installed, skipping."
else
    VER=$(apt-cache policy kubelet | grep -v 'Candidate' | grep $VER | awk '{print $1}' | head -n1)
    apt install -y kubelet=$VER kubeadm=$VER kubectl=$VER
    apt-mark hold kubelet kubeadm kubectl
fi
systemctl restart docker
swapoff -a
sed -i '/ swap / s/^/#/' /etc/fstab
sed -i '/swap.img/ s/^/#/' /etc/fstab
systemctl restart kubelet