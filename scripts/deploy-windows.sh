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

TARGET_DIR="$(realpath -m "${TARGET_DIR}")"
case "${TARGET_DIR}" in
    /mnt/d/StudyGuardianDev) ;;
    *) echo "[Deploy] Refusing unexpected target: ${TARGET_DIR}" >&2; exit 1 ;;
esac

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
mkdir -p "${TARGET_DIR}/scripts"

# 3. Ensure auth.token exists
TOKEN_FILE="${TARGET_DIR}/config/auth.token"
if [ ! -f "${TOKEN_FILE}" ] || [ ! -s "${TOKEN_FILE}" ]; then
    echo "[Deploy] Generating new auth.token..."
    python3 -c "import secrets; print(secrets.token_hex(16))" > "${TOKEN_FILE}"
    chmod 600 "${TOKEN_FILE}"
fi

# 4. Ensure config.yaml exists
CONFIG_FILE="${TARGET_DIR}/config/config.yaml"
if [ ! -f "${CONFIG_FILE}" ]; then
    echo "[Deploy] Initializing default config.yaml..."
    cp "${REPO_ROOT}/configs/default.yaml" "${CONFIG_FILE}"
fi

# 5. Stage all replaceable runtime files before touching the live tree.
STAGE_DIR="$(mktemp -d "${TARGET_DIR}/.deploy-staging.XXXXXX")"
cleanup() { rm -rf "${STAGE_DIR}"; }
trap cleanup EXIT
mkdir -p "${STAGE_DIR}/bin" "${STAGE_DIR}/pet/src" "${STAGE_DIR}/pet/assets/skins" "${STAGE_DIR}/sensor/screen" "${STAGE_DIR}/scripts"
cp -f "${REPO_ROOT}/dist/windows/bin/study-supervisor.exe" "${STAGE_DIR}/bin/study-supervisor.exe"
cp -f "${REPO_ROOT}/dist/windows/bin/config-helper.exe" "${STAGE_DIR}/bin/config-helper.exe"
cp -r "${REPO_ROOT}/dist/windows/pet/src/." "${STAGE_DIR}/pet/src/"
cp -f "${REPO_ROOT}/dist/windows/pet/requirements.txt" "${STAGE_DIR}/pet/requirements.txt"
cp -r "${REPO_ROOT}/dist/windows/pet/assets/skins/." "${STAGE_DIR}/pet/assets/skins/"
cp -r "${REPO_ROOT}/dist/windows/sensor/screen/." "${STAGE_DIR}/sensor/screen/"
cp -f "${REPO_ROOT}/dist/windows/sensor/requirements.txt" "${STAGE_DIR}/sensor/requirements.txt"
cp -f "${REPO_ROOT}/scripts/"*.ps1 "${STAGE_DIR}/scripts/" 2>/dev/null || true

test -s "${STAGE_DIR}/bin/study-supervisor.exe"
test -s "${STAGE_DIR}/bin/config-helper.exe"
test -f "${STAGE_DIR}/pet/src/main.py"
test -f "${STAGE_DIR}/pet/requirements.txt"
test -f "${STAGE_DIR}/pet/assets/skins/studyguardian-pixel/manifest.json"
test -f "${STAGE_DIR}/sensor/screen/server.py"
test -f "${STAGE_DIR}/sensor/requirements.txt"

# 6. Replace only exact program-owned paths. Persistent data and both Python
# virtual environments are deliberately outside this replacement set.
echo "[Deploy] Replacing program-owned runtime paths..."
rm -rf "${TARGET_DIR}/pet/src" "${TARGET_DIR}/pet/assets/skins" "${TARGET_DIR}/sensor/screen"
mkdir -p "${TARGET_DIR}/pet" "${TARGET_DIR}/pet/assets" "${TARGET_DIR}/sensor"
mv "${STAGE_DIR}/pet/src" "${TARGET_DIR}/pet/src"
mv "${STAGE_DIR}/pet/requirements.txt" "${TARGET_DIR}/pet/requirements.txt"
mv "${STAGE_DIR}/pet/assets/skins" "${TARGET_DIR}/pet/assets/skins"
mv "${STAGE_DIR}/sensor/screen" "${TARGET_DIR}/sensor/screen"
mv "${STAGE_DIR}/sensor/requirements.txt" "${TARGET_DIR}/sensor/requirements.txt"
mv "${STAGE_DIR}/bin/study-supervisor.exe" "${TARGET_DIR}/bin/study-supervisor.exe"
mv "${STAGE_DIR}/bin/config-helper.exe" "${TARGET_DIR}/bin/config-helper.exe"
rm -rf "${TARGET_DIR}/scripts"
mv "${STAGE_DIR}/scripts" "${TARGET_DIR}/scripts"

echo "=========================================================="
echo "Deployment successful!"
echo "Persistent directories protected:"
echo " - Config:  ${TARGET_DIR}/config"
echo " - Data:    ${TARGET_DIR}/data"
echo " - Logs:    ${TARGET_DIR}/logs"
echo " - Run:     ${TARGET_DIR}/run"
echo " - Handoff: ${TARGET_DIR}/handoff"
echo "=========================================================="
