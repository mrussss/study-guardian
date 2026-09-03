#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== [1/6] Running Go Unit Tests ==="
cd "${REPO_ROOT}"
go test ./...

echo "=== [2/6] Cross-Compiling Go binaries for Windows (amd64) ==="
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

echo "=== [3/6] Preparing Python Artifacts ==="
mkdir -p "${REPO_ROOT}/dist/windows/pet"
mkdir -p "${REPO_ROOT}/dist/windows/sensor"
mkdir -p "${REPO_ROOT}/dist/windows/browser"

# Copy Pet source & assets
cp -r "${REPO_ROOT}/pet/src" "${REPO_ROOT}/dist/windows/pet/"
cp "${REPO_ROOT}/pet/requirements.txt" "${REPO_ROOT}/dist/windows/pet/requirements.txt"
mkdir -p "${REPO_ROOT}/dist/windows/pet/assets/skins"
cp -r "${REPO_ROOT}/pet/assets/skins/." "${REPO_ROOT}/dist/windows/pet/assets/skins/"

# Copy Sensor source
cp -r "${REPO_ROOT}/sensor/screen" "${REPO_ROOT}/dist/windows/sensor/"
cp "${REPO_ROOT}/sensor/requirements.txt" "${REPO_ROOT}/dist/windows/sensor/requirements.txt"

# Copy the DOM-only Manifest V3 collector source (never local extension storage).
cp -r "${REPO_ROOT}/browser/chatgpt-collector" "${REPO_ROOT}/dist/windows/browser/"
rm -rf "${REPO_ROOT}/dist/windows/browser/chatgpt-collector/node_modules"
echo "=== [4/6] Bundling Chrome Content Script ==="
COLLECTOR_SOURCE="${REPO_ROOT}/browser/chatgpt-collector"
COLLECTOR_ARTIFACT="${REPO_ROOT}/dist/windows/browser/chatgpt-collector"
mkdir -p "${REPO_ROOT}/dist/windows/browser/chatgpt-collector/dist"
python3 "${REPO_ROOT}/scripts/bundle-content.py" \
    "${COLLECTOR_SOURCE}/src/content.js" \
    "${COLLECTOR_ARTIFACT}/dist/content.js"
test -s "${COLLECTOR_ARTIFACT}/dist/content.js"
if grep -Eq '^[[:space:]]*import[[:space:]]' "${COLLECTOR_ARTIFACT}/dist/content.js"; then
    echo "Content Script bundle still contains a static import" >&2
    exit 1
fi
test -f "${REPO_ROOT}/dist/windows/browser/chatgpt-collector/manifest.json"
test -s "${REPO_ROOT}/dist/windows/browser/chatgpt-collector/dist/content.js"
test ! -e "${REPO_ROOT}/dist/windows/browser/chatgpt-collector/node_modules"

# Copy default config template
cp "${REPO_ROOT}/configs/default.yaml" "${REPO_ROOT}/dist/windows/config.default.yaml"

echo "=== [5/6] Copying Windows helper scripts ==="
mkdir -p "${REPO_ROOT}/dist/windows/scripts"
cp -f "${REPO_ROOT}/scripts/"*.ps1 "${REPO_ROOT}/dist/windows/scripts/" 2>/dev/null || true

echo "=== [6/6] Build Complete ==="
echo "Artifacts located at: ${REPO_ROOT}/dist/windows"
ls -la "${REPO_ROOT}/dist/windows/bin"
