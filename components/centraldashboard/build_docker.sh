TAG="v$(date +%y%m%d%H%M)"
IMG=cguaicadmin/centraldashboard

make docker-build IMG=$IMG TAG=$TAG
make docker-push  IMG=$IMG TAG=$TAG

kubectl -n kubeflow set image deployment/centraldashboard \
  centraldashboard=$IMG:$TAG

kubectl -n kubeflow rollout status deployment/centraldashboard
