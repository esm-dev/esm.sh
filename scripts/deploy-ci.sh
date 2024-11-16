#!/bin/bash

echo "--- building..."
go build -o esmd $(dirname $0)/../main.go
if [ "$?" != "0" ]; then
  exit 1
fi

mkdir -p ~/.ssh
echo "${SSH_PRIVATE_KEY}" >> ~/.ssh/id_ed25519
echo "Host next.esm.sh" >> ~/.ssh/config
echo "  HostName ${SSH_HOST_NAME}" >> ~/.ssh/config
echo "  User ${SSH_USER}" >> ~/.ssh/config
echo "  IdentityFile ~/.ssh/id_ed25519" >> ~/.ssh/config
echo "  IdentitiesOnly yes" >> ~/.ssh/config

cat ~/.ssh/id_ed25519
cat ~/.ssh/config

echo "--- uploading..."
tar -czf esmd.tar.gz esmd
scp esmd.tar.gz next.esm.sh:/tmp/esmd.tar.gz
if [ "$?" != "0" ]; then
  exit 1
fi

echo "--- installing..."
ssh next.esm.sh << EOF
  cd /tmp
  tar -xzf esmd.tar.gz
  if [ "\$?" != "0" ]; then
    exit \$?
  fi
  rm -rf esmd.tar.gz

  supervisorctl version
  if [ "\$?" != "0" ]; then
    apt update
    apt install -y supervisor git git-lfs
  fi

  svcf=/etc/supervisor/conf.d/esmd.conf
  reload=no
  if [ ! -f \$svcf ]; then
    echo "[program:esmd]" >> \$svcf
    echo "command=/usr/local/bin/esmd" >> \$svcf
    echo "directory=/tmp" >> \$svcf
    echo "user=${SSH_USER}" >> \$svcf
    echo "autostart=true" >> \$svcf
    echo "autorestart=true" >> \$svcf
    reload=yes
  if

  supervisorctl stop esmd
  rm -f /usr/local/bin/esmd
  mv -f esmd /usr/local/bin/esmd
  chmod +x /usr/local/bin/esmd

  if [ "\$reload" == "yes" ]; then
    supervisorctl reload
  else
    supervisorctl start esmd
  fi
EOF
if [ "$?" != "0" ]; then
  exit 1
fi
