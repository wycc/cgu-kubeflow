#!/bin/bash
bash 2.install_docker.sh
bash 3.0.install_k8s_mater.sh
bash 3.1.deploy_calico.sh
bash 4.install_helm.sh
bash 5.install_kustomize.sh
bash 6.deploy_kubeflow.sh