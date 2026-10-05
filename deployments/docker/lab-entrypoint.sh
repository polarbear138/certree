#!/bin/sh
set -eu
cp /fixtures/ssh_host_ed25519_key /etc/ssh/ssh_host_ed25519_key
chmod 600 /etc/ssh/ssh_host_ed25519_key
cp /fixtures/authorized_keys /home/certree/.ssh/authorized_keys
chmod 700 /home/certree/.ssh
chmod 600 /home/certree/.ssh/authorized_keys
chown -R certree:certree /home/certree/.ssh
sed -n '/BEGIN CERTIFICATE/,/END CERTIFICATE/p' /fixtures/v1.pem > /srv/nginx/cert.pem
# Extract just the private key; tests never use fixture keys outside this lab.
sed -n '/BEGIN PRIVATE KEY/,/END PRIVATE KEY/p' /fixtures/v1.pem > /srv/nginx/key.pem
chmod 600 /srv/nginx/key.pem
cat > /srv/nginx/nginx.conf <<'EOF'
pid /srv/nginx/nginx.pid;
error_log /srv/nginx/error.log;
events { worker_connections 64; }
http {
  access_log /srv/nginx/access.log;
  client_body_temp_path /srv/nginx/tmp/client;
  proxy_temp_path /srv/nginx/tmp/proxy;
  fastcgi_temp_path /srv/nginx/tmp/fastcgi;
  uwsgi_temp_path /srv/nginx/tmp/uwsgi;
  scgi_temp_path /srv/nginx/tmp/scgi;
  server {
    listen 8443 ssl;
    server_name lab.test;
    ssl_certificate /srv/nginx/cert.pem;
    ssl_certificate_key /srv/nginx/key.pem;
    location / { return 200 'Certree TLS lab\n'; }
  }
}
EOF
mkdir -p /srv/nginx/backups /srv/nginx/tmp
chmod 700 /srv/nginx/backups
chown -R certree:certree /srv/nginx
printf '\nPasswordAuthentication no\nPermitRootLogin no\nAllowUsers certree\nHostKey /etc/ssh/ssh_host_ed25519_key\n' >> /etc/ssh/sshd_config
su -s /bin/sh certree -c 'nginx -c /srv/nginx/nginx.conf'
exec /usr/sbin/sshd -D -e
