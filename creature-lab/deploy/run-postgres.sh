#!/usr/bin/env bash
# Foreground postgres launcher for the fly.io sprite deploy.
#
# The sprite services layer needs a foreground command it can supervise, so
# this wraps postgres directly rather than going through pg_ctlcluster.
#
# /var/run/postgresql is tmpfs on the sprite base image — it disappears on
# every reboot. Postgres will not start without it (it puts its socket and
# lock file there), so recreate it here before exec.
set -euo pipefail

sudo mkdir -p /var/run/postgresql
sudo chown postgres:postgres /var/run/postgresql
sudo chmod 2775 /var/run/postgresql

exec sudo -u postgres /usr/lib/postgresql/18/bin/postgres \
  -D /var/lib/postgresql/18/main \
  -c config_file=/etc/postgresql/18/main/postgresql.conf
