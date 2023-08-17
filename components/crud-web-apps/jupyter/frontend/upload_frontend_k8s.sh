POD=jupyter-web-app-deployment-58c9859645-s97gp
cp -a dist dist2
rm -rf dist/frontend/assets/logos
tar czvf ff.tgz dist/frontend

kubectl cp ff.tgz -n kubeflow ${POD}:/
kubectl cp ../backend/apps/common/yaml/notebook_template.yaml -n kubeflow ${POD}:/
kubectl cp ../backend/apps/default/routes/post.py -n kubeflow ${POD}:/
kubectl cp ../backend/apps/common/utils.py -n kubeflow ${POD}:/
kubectl exec -n kubeflow ${POD} -- sh -c "cd /;tar xzvf ff.tgz; cp -a dist/frontend/* /src/apps/default/static"
kubectl exec -n kubeflow ${POD} -- sh -c "cd /;cp -a notebook_template.yaml /src/apps/common/yaml/notebook_template.yaml"
kubectl exec -n kubeflow ${POD} -- sh -c "cd /;cp -a post.py /src/apps/default/routes/post.py"
kubectl exec -n kubeflow ${POD} -- sh -c "cd /;cp -a utils.py /src/apps/common/utils.py"
rm -rf dist
mv dist2 dist
exit 0

