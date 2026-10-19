#!/usr/bin/env bash
set -euo pipefail
curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION=v1.37.1+k3s1 INSTALL_K3S_EXEC="--write-kubeconfig-mode 600" sh -
command -v git >/dev/null || dnf -y install git
[ -d /opt/sms-platform ] || git clone -q https://github.com/martialanouman/sms-platform /opt/sms-platform
install -D -m 755 /opt/sms-platform/deploy/test-env/sync.sh /usr/local/libexec/sms-deploy-sync
install -m 644 /opt/sms-platform/deploy/test-env/sms-deploy.service /opt/sms-platform/deploy/test-env/sms-deploy.timer /etc/systemd/system/
systemctl daemon-reload
systemctl enable sms-deploy.timer && systemctl restart sms-deploy.timer
