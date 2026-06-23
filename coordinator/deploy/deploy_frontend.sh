#!/usr/bin/env bash

set -euo pipefail

# One-click frontend deploy for a VPS where coordinator is already running.
#
# Required env:
#   DOMAIN=<domain-or-ip>
#   PUBLIC_ORIGIN=<frontend-visible API origin, e.g. https://domain>
#
# Optional env:
#   INSTALL_SSL=true|false                 (default: false)
#   COORDINATOR_PORT=<host port>           (default: 8080)
#   FRONTEND_REPO_URL=<git url>            (default: dearjohndoe/mytonprovider-org)
#   FRONTEND_BRANCH=<git branch>           (default: master)
#   NODE_MAJOR=<node major version>        (default: 20)

DOMAIN="${DOMAIN:-}"
PUBLIC_ORIGIN="${PUBLIC_ORIGIN:-}"
INSTALL_SSL="${INSTALL_SSL:-false}"
COORDINATOR_PORT="${COORDINATOR_PORT:-8080}"
FRONTEND_REPO_URL="${FRONTEND_REPO_URL:-https://github.com/dearjohndoe/mytonprovider-org.git}"
FRONTEND_BRANCH="${FRONTEND_BRANCH:-master}"
NODE_MAJOR="${NODE_MAJOR:-20}"

info() {
  printf '[INFO] %s\n' "$1"
}

warn() {
  printf '[WARN] %s\n' "$1"
}

fatal() {
  printf '[ERROR] %s\n' "$1" >&2
  exit 1
}

run_as_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    fatal "This script needs root privileges. Re-run as root or install sudo."
  fi
}

usage() {
  cat <<'EOF'
Usage:
  DOMAIN=front.example.com \
  PUBLIC_ORIGIN=https://front.example.com \
  INSTALL_SSL=true \
  COORDINATOR_PORT=8080 \
  bash coordinator/deploy/deploy_frontend.sh
EOF
}

if [[ -z "$DOMAIN" || -z "$PUBLIC_ORIGIN" ]]; then
  usage
  fatal "DOMAIN and PUBLIC_ORIGIN are required"
fi

if [[ "$DOMAIN" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  SITE_NAME="ip-${DOMAIN//./-}"
else
  SITE_NAME="$DOMAIN"
fi

WEB_ROOT="/var/www/${SITE_NAME}"
NGINX_CONFIG="/etc/nginx/sites-available/${SITE_NAME}"
NGINX_ENABLED="/etc/nginx/sites-enabled/${SITE_NAME}"
WORK_DIR="$(mktemp -d /tmp/mtpo-frontend-deploy-XXXXXX)"
FRONTEND_DIR="${WORK_DIR}/frontend"

cleanup() {
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

info "Installing system dependencies"
run_as_root apt-get update
run_as_root apt-get install -y curl git nginx ca-certificates gnupg

if ! command -v node >/dev/null 2>&1; then
  info "Installing Node.js ${NODE_MAJOR}.x"
  curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | run_as_root bash -
  run_as_root apt-get install -y nodejs
fi

if ! command -v npm >/dev/null 2>&1; then
  fatal "npm is not available after Node.js installation"
fi

info "Cloning frontend repo ${FRONTEND_REPO_URL} (${FRONTEND_BRANCH})"
git clone --depth 1 --branch "${FRONTEND_BRANCH}" "${FRONTEND_REPO_URL}" "${FRONTEND_DIR}"

cd "${FRONTEND_DIR}"

if [[ ! -f "lib/api.ts" ]]; then
  fatal "lib/api.ts not found in frontend repo; cannot patch API origin"
fi

info "Patching frontend API origin to ${PUBLIC_ORIGIN}"
sed -i "s|https://mytonprovider.org|${PUBLIC_ORIGIN}|g" lib/api.ts

info "Installing frontend dependencies"
npm install --legacy-peer-deps

info "Building frontend"
npm run build

if [[ -d "out" ]]; then
  BUILD_DIR="out"
elif [[ -d "dist" ]]; then
  BUILD_DIR="dist"
elif [[ -d "build" ]]; then
  BUILD_DIR="build"
else
  fatal "Build output directory not found (expected out/, dist/, or build/)"
fi

info "Deploying static files to ${WEB_ROOT}"
run_as_root rm -rf "${WEB_ROOT}"
run_as_root mkdir -p "${WEB_ROOT}"
run_as_root cp -r "${BUILD_DIR}/." "${WEB_ROOT}/"
run_as_root chown -R www-data:www-data "${WEB_ROOT}"
run_as_root chmod -R 755 "${WEB_ROOT}"

info "Writing nginx config ${NGINX_CONFIG}"
run_as_root tee "${NGINX_CONFIG}" >/dev/null <<EOF
server {
  listen 80;
  listen [::]:80;
  server_name ${DOMAIN};
  root ${WEB_ROOT};
  index index.html;

  add_header X-Frame-Options "SAMEORIGIN" always;
  add_header X-Content-Type-Options "nosniff" always;
  add_header Referrer-Policy "strict-origin-when-cross-origin" always;

  location /api/ {
    proxy_pass http://127.0.0.1:${COORDINATOR_PORT};
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
  }

  location /health {
    proxy_pass http://127.0.0.1:${COORDINATOR_PORT};
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
  }

  location /metrics {
    proxy_pass http://127.0.0.1:${COORDINATOR_PORT};
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
  }

  location / {
    try_files \$uri \$uri/ /index.html;
  }

  location ~ /\. {
    deny all;
  }
}
EOF

run_as_root ln -sf "${NGINX_CONFIG}" "${NGINX_ENABLED}"
run_as_root rm -f /etc/nginx/sites-enabled/default
run_as_root nginx -t
run_as_root systemctl enable nginx
run_as_root systemctl restart nginx

if [[ "${INSTALL_SSL}" == "true" ]]; then
  if [[ "${DOMAIN}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    warn "INSTALL_SSL=true ignored for IP domain (${DOMAIN}); SSL certificates require a domain name"
  else
    info "Installing certbot and requesting SSL cert for ${DOMAIN}"
    run_as_root apt-get install -y certbot python3-certbot-nginx
    if ! run_as_root certbot --nginx -d "${DOMAIN}" --non-interactive --agree-tos -m "admin@${DOMAIN}" --redirect; then
      warn "certbot failed; frontend remains available over HTTP"
    fi
  fi
fi

info "Frontend deploy complete"
printf 'Frontend URL: http://%s\n' "${DOMAIN}"
printf 'API proxy URL: http://%s/api/\n' "${DOMAIN}"
