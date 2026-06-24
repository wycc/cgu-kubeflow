#!/usr/bin/env bash
set -e

POD=$(kubectl get -n kubeflow pods -l app=jupyter-web-app --no-headers -o custom-columns=":metadata.name")

cp -a dist dist2
rm -rf dist/frontend/assets/logos
tar czvf ff.tgz dist/frontend

kubectl cp ff.tgz -n kubeflow "${POD}":/
kubectl exec -n kubeflow "${POD}" -- sh -c "cd /; tar xzvf ff.tgz; cp -a dist/frontend/* /src/apps/default/static"
kubectl exec -n kubeflow "${POD}" -- sh -c "pkill -HUP gunicorn || true"

rm -rf dist
mv dist2 dist
