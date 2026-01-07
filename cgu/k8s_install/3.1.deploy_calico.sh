#!/bin/bash
VER=3.27.3
curl -OL https://raw.githubusercontent.com/projectcalico/calico/v$VER/manifests/calico.yaml
kubectl apply -f calico.yaml