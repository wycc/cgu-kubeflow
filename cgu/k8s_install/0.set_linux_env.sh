#!/bin/bash
apt update
apt dist-upgrade -y
apt install net-tools curl git nfs-common vim ssh -y
timedatectl set-timezone Asia/Taipei
timedatectl set-ntp true
echo 'GRUB_CMDLINE_LINUX_DEFAULT="${GRUB_CMDLINE_LINUX_DEFAULT} ipv6.disable=1"' \
  | sudo tee /etc/default/grub.d/90-disable-ipv6.cfg
update-grub
CONF="/etc/sysctl.d/99-disable-ipv6.conf"
echo 'net.ipv6.conf.all.disable_ipv6=1' | sudo tee "$CONF" > /dev/null
echo 'net.ipv6.conf.default.disable_ipv6=1' | sudo tee -a "$CONF" > /dev/null
interfaces=$(ip -o link show | awk -F': ' '{print $2}' | grep -v 'lo')
# Loop through each interface and print the command
for iface in $interfaces; do
    echo "net.ipv6.conf.$iface.disable_ipv6=1" | sudo tee -a "$CONF" > /dev/null
done
sysctl -p $CONF
echo 'GRUB_CMDLINE_LINUX_DEFAULT="${GRUB_CMDLINE_LINUX_DEFAULT} systemd.unified_cgroup_hierarchy=1"' \
  | sudo tee /etc/default/grub.d/90-use-cgroupv2.cfg
update-grub
test -f /sys/fs/cgroup/cgroup.controllers && echo OK
sed -i -e $'$a\\\nnfs.inotify.max_user_watches = 524288\\nfs.inotify.max_user_instances = 512' /etc/sysctl.conf
sysctl --system