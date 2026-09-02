#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== [1/4] Running Go Unit Tests ==="
cd "${REPO_ROOT}"
go test ./...

echo "=== [2/4] Cross-Compiling Go Supervisor for Windows (amd64) ==="
mkdir -p "${REPO_ROOT}/dist/windows/bin"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o "${REPO_ROOT}/dist/windows/bin/study-supervisor.exe" \
    ./cmd/supervisor

echo "=== [3/4] Preparing Python Artifacts ==="
mkdir -p "${REPO_ROOT}/dist/windows/pet"
mkdir -p "${REPO_ROOT}/dist/windows/sensor"

# Copy Pet source & assets
cp -r "${REPO_ROOT}/pet/src" "${REPO_ROOT}/dist/windows/pet/"
if [ -d "${REPO_ROOT}/pet/assets" ]; then
    cp -r "${REPO_ROOT}/pet/assets" "${REPO_ROOT}/dist/windows/pet/"
fi

# Copy Sensor source
cp -r "${REPO_ROOT}/sensor/screen" "${REPO_ROOT}/dist/windows/sensor/"

# Copy default config template
cp "${REPO_ROOT}/configs/default.yaml" "${REPO_ROOT}/dist/windows/config.default.yaml"

echo "=== [4/4] Build Complete ==="
echo "Artifacts located at: ${REPO_ROOT}/dist/windows"
ls -la "${REPO_ROOT}/dist/windows/bin"
