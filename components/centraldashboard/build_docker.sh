make docker-build IMG=cguaicadmin/centraldashboard TAG=v260516-ann-v8
make docker-push  IMG=cguaicadmin/centraldashboard TAG=v260516-ann-v8

kubectl -n kubeflow set image deployment/centraldashboard \
  centraldashboard=cguaicadmin/centraldashboard:v260516-ann-v8

kubectl -n kubeflow rollout status deployment/centraldashboard
