MAX_PODS=254
if grep -q "maxPods:" /var/lib/kubelet/config.yaml; then
  sed -i "s/maxPods: [0-9]*/maxPods: $MAX_PODS/" /var/lib/kubelet/config.yaml
else
  echo "maxPods: $MAX_PODS" >> /var/lib/kubelet/config.yaml
fi
systemctl restart kubelet