#!/bin/bash
OS_VER=$(source /etc/os-release; echo $VERSION_ID | tr -d '.')
curl -OL https://developer.download.nvidia.com/compute/cuda/repos/ubuntu$OS_VER/x86_64/cuda-keyring_1.1-1_all.deb
dpkg -i cuda-keyring*.deb
apt update
VER=550
apt install linux-headers-$(uname -r)
apt install --no-install-recommends -y nvidia-headless-$VER-server nvidia-utils-$VER-server
apt list --installed 2>/dev/null | grep nvidia-utils | awk -F'/' '{print $1}' | xargs sudo apt-mark hold
apt list --installed 2>/dev/null | grep nvidia-headless | awk -F'/' '{print $1}' | xargs sudo apt-mark hold