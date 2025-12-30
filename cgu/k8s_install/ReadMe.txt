sudo -i
bash 0.set_linux_env.sh
reboot

修改NAS的IP跟路徑
在4.install_helm.sh的10、11行

在mater的時候:
bash 7.0.install_master.sh

在node的時候:
bash 7.1.install_node.sh
到master執行kubeadm token create --print-join-command
會獲得新增node指令
kubeadm join 192.168.1.235:6443 --token iqxccl.ccxwwsw5j9vn5g7l --discovery-token-ca-cert-hash sha256:6ca6b79d7b3b94982e666ee82cd8072c62e81a0d89321012dcc90c9cd68e9df9
在node中貼上以上指令，並新增--cri-socket unix:///var/run/cri-dockerd.sock

執行bash 3.2.apply_kubelet_setting.sh

回到master
kubectl get po -A 查看是否所有pod都處於running