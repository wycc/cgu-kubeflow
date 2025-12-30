#!/bin/bash
wget -O kustomize_v5.4.0_linux_amd64.tar.gz https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv5.4.0/kustomize_v5.4.0_linux_amd64.tar.gz
tar zxvf kustomize_v5.4.0_linux_amd64.tar.gz
cp kustomize /usr/local/bin