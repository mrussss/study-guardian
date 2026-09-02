#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGET_DIR="/mnt/d/StudyGuardianDev"

echo "=== Testing Deploy Safety on ${TARGET_DIR} ==="

# 1. Create canary files in persistent directories
mkdir -p "${TARGET_DIR}/config" "${TARGET_DIR}/data" "${TARGET_DIR}/logs" "${TARGET_DIR}/run" "${TARGET_DIR}/handoff"
echo "config-canary-12345" > "${TARGET_DIR}/config/canary.txt"
echo "data-canary-12345" > "${TARGET_DIR}/data/canary.db"
echo "logs-canary-12345" > "${TARGET_DIR}/logs/canary.log"
echo "run-canary-12345" > "${TARGET_DIR}/run/canary.pid"
echo "handoff-canary-12345" > "${TARGET_DIR}/handoff/canary.json"

# 2. Run first deploy
echo "--- Running Deploy 1 ---"
bash "${REPO_ROOT}/scripts/deploy-windows.sh" "${TARGET_DIR}"

# 3. Verify canary files
grep -q "config-canary-12345" "${TARGET_DIR}/config/canary.txt"
grep -q "data-canary-12345" "${TARGET_DIR}/data/canary.db"
grep -q "logs-canary-12345" "${TARGET_DIR}/logs/canary.log"
grep -q "run-canary-12345" "${TARGET_DIR}/run/canary.pid"
grep -q "handoff-canary-12345" "${TARGET_DIR}/handoff/canary.json"

# Record auth.token
TOKEN_1="$(cat "${TARGET_DIR}/config/auth.token")"

# 4. Run second deploy
echo "--- Running Deploy 2 ---"
bash "${REPO_ROOT}/scripts/deploy-windows.sh" "${TARGET_DIR}"

# 5. Verify canary files and auth.token consistency
grep -q "config-canary-12345" "${TARGET_DIR}/config/canary.txt"
grep -q "data-canary-12345" "${TARGET_DIR}/data/canary.db"
grep -q "logs-canary-12345" "${TARGET_DIR}/logs/canary.log"
grep -q "run-canary-12345" "${TARGET_DIR}/run/canary.pid"
grep -q "handoff-canary-12345" "${TARGET_DIR}/handoff/canary.json"

TOKEN_2="$(cat "${TARGET_DIR}/config/auth.token")"
if [ "${TOKEN_1}" != "${TOKEN_2}" ]; then
    echo "FAIL: auth.token was overwritten during redeploy!"
    exit 1
fi

# Clean up canaries
rm -f "${TARGET_DIR}/config/canary.txt" "${TARGET_DIR}/data/canary.db" "${TARGET_DIR}/logs/canary.log" "${TARGET_DIR}/run/canary.pid" "${TARGET_DIR}/handoff/canary.json"

echo "PASS: Deploy safety verified! All persistent directories and tokens preserved across multiple deployments."
