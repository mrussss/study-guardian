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
mkdir -p "${TARGET_DIR}/browser"

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
BACKUP_DIR=""
cleanup() {
    status=$?
    if [ "${status}" -ne 0 ] && [ -n "${BACKUP_DIR}" ] && [ -d "${BACKUP_DIR}" ]; then
        set +e
        for relative_path in "${EPHEMERAL_PATHS[@]}"; do rm -rf "${TARGET_DIR}/${relative_path}"; done
        for relative_path in "${EPHEMERAL_PATHS[@]}"; do
            if [ -e "${BACKUP_DIR}/${relative_path}" ]; then
                mkdir -p "${TARGET_DIR}/$(dirname "${relative_path}")"
                mv "${BACKUP_DIR}/${relative_path}" "${TARGET_DIR}/${relative_path}"
            fi
        done
        echo "[Deploy] Deployment failed; previous ephemeral runtime restored." >&2
    fi
    rm -rf "${STAGE_DIR}"
    if [ -n "${BACKUP_DIR}" ]; then rm -rf "${BACKUP_DIR}"; fi
    exit "${status}"
}
trap cleanup EXIT
mkdir -p "${STAGE_DIR}/bin" "${STAGE_DIR}/pet/src" "${STAGE_DIR}/pet/assets/skins" "${STAGE_DIR}/sensor/screen" "${STAGE_DIR}/scripts" "${STAGE_DIR}/browser"
cp -f "${REPO_ROOT}/dist/windows/bin/study-supervisor.exe" "${STAGE_DIR}/bin/study-supervisor.exe"
cp -f "${REPO_ROOT}/dist/windows/bin/config-helper.exe" "${STAGE_DIR}/bin/config-helper.exe"
cp -r "${REPO_ROOT}/dist/windows/pet/src/." "${STAGE_DIR}/pet/src/"
cp -f "${REPO_ROOT}/dist/windows/pet/requirements.txt" "${STAGE_DIR}/pet/requirements.txt"
cp -r "${REPO_ROOT}/dist/windows/pet/assets/skins/." "${STAGE_DIR}/pet/assets/skins/"
cp -r "${REPO_ROOT}/dist/windows/sensor/screen/." "${STAGE_DIR}/sensor/screen/"
cp -f "${REPO_ROOT}/dist/windows/sensor/requirements.txt" "${STAGE_DIR}/sensor/requirements.txt"
cp -f "${REPO_ROOT}/scripts/"*.ps1 "${STAGE_DIR}/scripts/" 2>/dev/null || true
cp -r "${REPO_ROOT}/dist/windows/browser/chatgpt-collector" "${STAGE_DIR}/browser/"

test -s "${STAGE_DIR}/bin/study-supervisor.exe"
test -s "${STAGE_DIR}/bin/config-helper.exe"
test -f "${STAGE_DIR}/pet/src/main.py"
test -f "${STAGE_DIR}/pet/requirements.txt"
test -f "${STAGE_DIR}/pet/assets/skins/studyguardian-pixel/manifest.json"
test -f "${STAGE_DIR}/sensor/screen/server.py"
test -f "${STAGE_DIR}/sensor/requirements.txt"
test -f "${STAGE_DIR}/browser/chatgpt-collector/manifest.json"

# 6. Stop the known runtime processes before touching locked Windows files.
STOP_SCRIPT="${TARGET_DIR}/scripts/stop-all.ps1"
if [ -f "${STOP_SCRIPT}" ]; then
    echo "[Deploy] Stopping Supervisor, Pet and Sensor..."
    WIN_STOP_SCRIPT="$(wslpath -w "${STOP_SCRIPT}")"
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File "${WIN_STOP_SCRIPT}"
    sleep 1
else
    echo "[Deploy] No previous stop script found; continuing as a fresh runtime deployment."
fi

# 7. Replace exact program-owned paths with a recoverable backup. Persistent
# data, config, logs, run, handoff and both Python virtual environments are
# deliberately outside this replacement set.
echo "[Deploy] Replacing program-owned runtime paths with rollback protection..."
BACKUP_DIR="$(mktemp -d "${TARGET_DIR}/.deploy-backup.XXXXXX")"
EPHEMERAL_PATHS=(
    "bin/study-supervisor.exe"
    "bin/config-helper.exe"
    "pet/src"
    "pet/requirements.txt"
    "pet/assets/skins"
    "sensor/screen"
    "sensor/requirements.txt"
    "scripts"
    "browser/chatgpt-collector"
)
for relative_path in "${EPHEMERAL_PATHS[@]}"; do
    source_path="${TARGET_DIR}/${relative_path}"
    if [ -e "${source_path}" ]; then
        mkdir -p "${BACKUP_DIR}/$(dirname "${relative_path}")"
        mv "${source_path}" "${BACKUP_DIR}/${relative_path}"
    fi
done

mkdir -p "${TARGET_DIR}/bin" "${TARGET_DIR}/pet" "${TARGET_DIR}/pet/assets" "${TARGET_DIR}/sensor" "${TARGET_DIR}/browser"
mv "${STAGE_DIR}/pet/src" "${TARGET_DIR}/pet/src"
mv "${STAGE_DIR}/pet/requirements.txt" "${TARGET_DIR}/pet/requirements.txt"
mv "${STAGE_DIR}/pet/assets/skins" "${TARGET_DIR}/pet/assets/skins"
mv "${STAGE_DIR}/sensor/screen" "${TARGET_DIR}/sensor/screen"
mv "${STAGE_DIR}/sensor/requirements.txt" "${TARGET_DIR}/sensor/requirements.txt"
mv "${STAGE_DIR}/bin/study-supervisor.exe" "${TARGET_DIR}/bin/study-supervisor.exe"
mv "${STAGE_DIR}/bin/config-helper.exe" "${TARGET_DIR}/bin/config-helper.exe"
mv "${STAGE_DIR}/scripts" "${TARGET_DIR}/scripts"
mv "${STAGE_DIR}/browser/chatgpt-collector" "${TARGET_DIR}/browser/chatgpt-collector"

# Validate the live replacement before deleting the backup. Any failed test
# or later deploy step is handled by the EXIT trap above.
test -s "${TARGET_DIR}/bin/study-supervisor.exe"
test -f "${TARGET_DIR}/pet/src/main.py"
test -f "${TARGET_DIR}/sensor/screen/server.py"
test -f "${TARGET_DIR}/browser/chatgpt-collector/manifest.json"

# Start only the newly deployed Supervisor for a bounded localhost health
# smoke. This validates the executable and config/database wiring without
# changing the user's normal runtime state; the normal launcher starts all
# components after Deploy returns.
WIN_TARGET_DIR="$(wslpath -w "${TARGET_DIR}")"
WIN_CONFIG_FILE="$(wslpath -w "${CONFIG_FILE}")"
WIN_TOKEN_FILE="$(wslpath -w "${TOKEN_FILE}")"
WIN_DB_FILE="$(wslpath -w "${TARGET_DIR}/data/studyguardian.db")"
SMOKE_COMMAND="\$ErrorActionPreference='Stop'; \$p=Start-Process -FilePath '${WIN_TARGET_DIR}\\bin\\study-supervisor.exe' -ArgumentList @('-config','${WIN_CONFIG_FILE}','-token','${WIN_TOKEN_FILE}','-collector-token','${WIN_TARGET_DIR}\\config\\collector-token','-db','${WIN_DB_FILE}') -WorkingDirectory '${WIN_TARGET_DIR}' -WindowStyle Hidden -PassThru; try { for(\$i=0;\$i -lt 20;\$i++){ try { \$h=Invoke-RestMethod -Uri 'http://127.0.0.1:17321/healthz'; if(\$h.status -eq 'ok'){ exit 0 } } catch {} Start-Sleep -Milliseconds 500 }; exit 1 } finally { if(\$p -and -not \$p.HasExited){ Stop-Process -Id \$p.Id -Force } }"
echo "[Deploy] Running Supervisor health smoke..."
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "${SMOKE_COMMAND}"

rm -rf "${BACKUP_DIR}"
BACKUP_DIR=""

echo "=========================================================="
echo "Deployment successful!"
echo "Persistent directories protected:"
echo " - Config:  ${TARGET_DIR}/config"
echo " - Data:    ${TARGET_DIR}/data"
echo " - Logs:    ${TARGET_DIR}/logs"
echo " - Run:     ${TARGET_DIR}/run"
echo " - Handoff: ${TARGET_DIR}/handoff"
echo "=========================================================="
