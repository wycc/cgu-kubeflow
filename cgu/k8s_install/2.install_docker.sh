#!/bin/bash
REPO_KEY="/usr/share/keyrings/docker.gpg"
REPO_CONF="deb [arch=$(dpkg --print-architecture) signed-by=$REPO_KEY] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor --yes -o $REPO_KEY
echo "$REPO_CONF" | sudo tee /etc/apt/sources.list.d/docker.list
apt update
VER=26
INSTALLED_VER=$(apt-cache policy docker-ce | grep Installed | awk '{print $2}' | cut -d: -f2 | cut -d. -f1)
if [ "$INSTALLED_VER" == "$VER" ]; then
    echo "Docker version 26 is already installed, skipping."
else
    VER=$(apt-cache policy docker-ce | grep -v 'Candidate' | grep $VER | awk '{print $1}' | head -n1)
    apt install --no-install-recommends -y docker-ce=$VER docker-ce-cli=$VER containerd.io
    #maybe need --allow-change-held-packages
    apt-mark hold docker-ce docker-ce-cli
fi
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor --yes -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg \
  && curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
    sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
    sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
apt update
VER=1.15
INSTALLED_VER=$(apt-cache policy nvidia-container-toolkit | grep Installed | awk '{print $2}' | cut -d: -f2 | cut -d. -f1-2)
if [ "$INSTALLED_VER" == "$VER" ]; then
    echo "nvidia-container-toolkit version 1.15 is already installed, skipping."
else
    VER=$(apt-cache policy nvidia-container-toolkit | grep -v 'Candidate' | grep $VER | awk '{print $1}' | head -n1)
    apt install --no-install-recommends -y nvidia-container-toolkit-base=$VER
    apt install --no-install-recommends -y nvidia-container-toolkit=$VER
    apt-mark hold nvidia-container-toolkit
fi
wget -O cri-dockerd_0.3.15.3-0.ubuntu-jammy_amd64.deb https://github.com/Mirantis/cri-dockerd/releases/download/v0.3.15/cri-dockerd_0.3.15.3-0.ubuntu-jammy_amd64.deb
dpkg -i cri-dockerd_0.3.15.3-0.ubuntu-jammy_amd64.deb
nvidia-ctk runtime configure --runtime=docker --set-as-default
systemctl restart docker