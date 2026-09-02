#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGET_DIR="${1:-/mnt/d/StudyGuardianDev}"

echo "=========================================================="
echo "StudyGuardian Deploy to Windows: ${TARGET_DIR}"
echo "=========================================================="

if [ ! -d "${TARGET_DIR}" ]; then
    echo "[Deploy] Target directory ${TARGET_DIR} does not exist. Creating..."
    mkdir -p "${TARGET_DIR}"
fi

# 1. Check build artifacts
if [ ! -f "${REPO_ROOT}/dist/windows/bin/study-supervisor.exe" ]; then
    echo "[Deploy] Build artifacts not found. Running build-windows.sh first..."
    bash "${REPO_ROOT}/scripts/build-windows.sh"
fi

# 2. Ensure persistent directories exist (NEVER DELETE THESE)
mkdir -p "${TARGET_DIR}/bin"
mkdir -p "${TARGET_DIR}/pet"
mkdir -p "${TARGET_DIR}/sensor"
mkdir -p "${TARGET_DIR}/config"
mkdir -p "${TARGET_DIR}/data"
mkdir -p "${TARGET_DIR}/logs"
mkdir -p "${TARGET_DIR}/run"
mkdir -p "${TARGET_DIR}/handoff"

# 3. Ensure auth.token exists
TOKEN_FILE="${TARGET_DIR}/config/auth.token"
if [ ! -f "${TOKEN_FILE}" ] || [ ! -s "${TOKEN_FILE}" ]; then
    echo "[Deploy] Generating new auth.token..."
    # Generate 16 bytes random hex token
    python3 -c "import secrets; print(secrets.token_hex(16))" > "${TOKEN_FILE}"
    chmod 600 "${TOKEN_FILE}"
fi
AUTH_TOKEN="$(cat "${TOKEN_FILE}" | tr -d '\r\n')"

# 4. Ensure config.yaml exists
CONFIG_FILE="${TARGET_DIR}/config/config.yaml"
if [ ! -f "${CONFIG_FILE}" ]; then
    echo "[Deploy] Initializing default config.yaml..."
    cp "${REPO_ROOT}/configs/default.yaml" "${CONFIG_FILE}"
fi

# 5. Deploy binary
echo "[Deploy] Updating bin/study-supervisor.exe..."
cp -f "${REPO_ROOT}/dist/windows/bin/study-supervisor.exe" "${TARGET_DIR}/bin/study-supervisor.exe"

# 6. Deploy Pet (preserve .venv)
echo "[Deploy] Updating pet runtime files..."
mkdir -p "${TARGET_DIR}/pet/src"
cp -r "${REPO_ROOT}/dist/windows/pet/src/"* "${TARGET_DIR}/pet/src/"
if [ -d "${REPO_ROOT}/dist/windows/pet/assets" ]; then
    mkdir -p "${TARGET_DIR}/pet/assets"
    cp -r "${REPO_ROOT}/dist/windows/pet/assets/"* "${TARGET_DIR}/pet/assets/"
fi

# 7. Deploy Sensor (preserve .venv)
echo "[Deploy] Updating sensor runtime files..."
mkdir -p "${TARGET_DIR}/sensor/screen"
cp -r "${REPO_ROOT}/dist/windows/sensor/screen/"* "${TARGET_DIR}/sensor/screen/"

echo "=========================================================="
echo "Deployment successful!"
echo "Persistent directories protected:"
echo " - Config:  ${TARGET_DIR}/config"
echo " - Data:    ${TARGET_DIR}/data"
echo " - Logs:    ${TARGET_DIR}/logs"
echo " - Run:     ${TARGET_DIR}/run"
echo " - Handoff: ${TARGET_DIR}/handoff"
echo "=========================================================="
