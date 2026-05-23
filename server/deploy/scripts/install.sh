#!/usr/bin/env bash
# DHP Registry install script — openEuler / RHEL-family
# Prerequisites: user has already uploaded and extracted the tarball to INSTALL_DIR.
set -euo pipefail

INSTALL_DIR="/data/app/halo-digital"
USER_NAME="app"

# ── 1. Create service user ────────────────────────────────────────────────
echo "Creating user ${USER_NAME}..."
if ! getent group "${USER_NAME}" > /dev/null 2>&1; then
    groupadd --system "${USER_NAME}"
fi
if ! id -u "${USER_NAME}" > /dev/null 2>&1; then
    useradd --system --home-dir "${INSTALL_DIR}" --shell /sbin/nologin --gid "${USER_NAME}" "${USER_NAME}"
fi

# ── 2. Verify extracted files ─────────────────────────────────────────────
if [ ! -x "${INSTALL_DIR}/dhp-registry" ]; then
    echo "ERROR: ${INSTALL_DIR}/dhp-registry not found or not executable." >&2
    echo "       Please upload and extract the tarball to ${INSTALL_DIR} first." >&2
    exit 1
fi

if [ ! -f "${INSTALL_DIR}/config.yaml" ]; then
    echo "ERROR: ${INSTALL_DIR}/config.yaml not found." >&2
    echo "       Copy config.yaml.example to config.yaml and edit token before running install." >&2
    exit 1
fi

# ── 3. Fix ownership ──────────────────────────────────────────────────────
echo "Setting ownership..."
chown -R "${USER_NAME}:${USER_NAME}" "${INSTALL_DIR}"

# ── 4. Create data directory ──────────────────────────────────────────────
mkdir -p "${INSTALL_DIR}/halo-data"
chown -R "${USER_NAME}:${USER_NAME}" "${INSTALL_DIR}/halo-data"

# ── 5. Install systemd service ────────────────────────────────────────────
echo "Installing systemd service..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SERVICE_FILE="${SCRIPT_DIR}/../systemd/dhp-registry.service"
if [ ! -f "${SERVICE_FILE}" ]; then
    # Fallback: look relative to INSTALL_DIR
    SERVICE_FILE="${INSTALL_DIR}/deploy/systemd/dhp-registry.service"
fi
if [ ! -f "${SERVICE_FILE}" ]; then
    echo "ERROR: dhp-registry.service not found. Searched:" >&2
    echo "  ${SCRIPT_DIR}/../systemd/dhp-registry.service" >&2
    echo "  ${INSTALL_DIR}/deploy/systemd/dhp-registry.service" >&2
    exit 1
fi
cp "${SERVICE_FILE}" /etc/systemd/system/dhp-registry.service
systemctl daemon-reload
systemctl enable dhp-registry

# ── 6. Firewall (openEuler uses firewalld) ────────────────────────────────
if command -v firewall-cmd &>/dev/null; then
    echo "Opening port 8099 in firewalld..."
    firewall-cmd --add-port=8099/tcp --permanent 2>/dev/null || true
    firewall-cmd --reload 2>/dev/null || true
fi

# ── 7. SELinux hint ───────────────────────────────────────────────────────
if command -v getenforce &>/dev/null && [ "$(getenforce)" = "Enforcing" ]; then
    echo "NOTICE: SELinux is Enforcing. If the service fails to start, run:"
    echo "  setsebool -P httpd_can_network_connect 1"
    echo "  restorecon -Rv ${INSTALL_DIR}"
fi

# ── 8. Start service ──────────────────────────────────────────────────────
echo "Starting dhp-registry..."
systemctl restart dhp-registry
sleep 1
systemctl status dhp-registry --no-pager || true

echo ""
echo "Done. Useful commands:"
echo "  systemctl status dhp-registry"
echo "  journalctl -u dhp-registry -f"
echo "  curl http://localhost:8099/digital-humans.json"
