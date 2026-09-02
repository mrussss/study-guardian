#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== [1/5] Running Go Unit Tests ==="
cd "${REPO_ROOT}"
go test ./...

echo "=== [2/5] Cross-Compiling Go binaries for Windows (amd64) ==="
rm -rf "${REPO_ROOT}/dist/windows"
mkdir -p "${REPO_ROOT}/dist/windows/bin"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o "${REPO_ROOT}/dist/windows/bin/study-supervisor.exe" \
    ./cmd/supervisor
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o "${REPO_ROOT}/dist/windows/bin/config-helper.exe" \
    ./cmd/config-helper

echo "=== [3/5] Preparing Python Artifacts ==="
mkdir -p "${REPO_ROOT}/dist/windows/pet"
mkdir -p "${REPO_ROOT}/dist/windows/sensor"

# Copy Pet source & assets
cp -r "${REPO_ROOT}/pet/src" "${REPO_ROOT}/dist/windows/pet/"
cp "${REPO_ROOT}/pet/requirements.txt" "${REPO_ROOT}/dist/windows/pet/requirements.txt"
mkdir -p "${REPO_ROOT}/dist/windows/pet/assets/skins"
cp -r "${REPO_ROOT}/pet/assets/skins/." "${REPO_ROOT}/dist/windows/pet/assets/skins/"

# Copy Sensor source
cp -r "${REPO_ROOT}/sensor/screen" "${REPO_ROOT}/dist/windows/sensor/"
cp "${REPO_ROOT}/sensor/requirements.txt" "${REPO_ROOT}/dist/windows/sensor/requirements.txt"

# Copy default config template
cp "${REPO_ROOT}/configs/default.yaml" "${REPO_ROOT}/dist/windows/config.default.yaml"

echo "=== [4/5] Copying Windows helper scripts ==="
mkdir -p "${REPO_ROOT}/dist/windows/scripts"
cp -f "${REPO_ROOT}/scripts/"*.ps1 "${REPO_ROOT}/dist/windows/scripts/" 2>/dev/null || true

echo "=== [5/5] Build Complete ==="
echo "Artifacts located at: ${REPO_ROOT}/dist/windows"
ls -la "${REPO_ROOT}/dist/windows/bin"
