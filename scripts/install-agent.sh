#!/bin/sh
set -eu
if [ "$(uname -s)" != Linux ] || [ "$(id -u)" != 0 ]; then
  echo 'Run this installer as root on a Linux systemd host.' >&2
  exit 1
fi
if [ "$#" != 1 ] || [ ! -f "$1" ]; then
  echo 'Usage: sudo sh scripts/install-agent.sh /path/to/built/certree' >&2
  exit 1
fi
command -v systemctl >/dev/null
install -m 0755 "$1" /usr/local/bin/certree
/usr/local/bin/certree service install
echo 'Next: sudo certree settings'
echo 'Then: sudo systemctl enable --now certree-agent'
