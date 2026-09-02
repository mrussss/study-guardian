#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"
echo "=== Go tests ==="
go test ./...

echo "=== Pet Python tests ==="
python3 -m unittest discover -s pet/tests -p 'test*.py'

echo "=== Sensor Python tests ==="
python3 -m unittest discover -s sensor/tests -p 'test*.py'

echo "=== Integration tests ==="
python3 -m unittest discover -s tests/integration -p 'test*.py'

echo "=== Deploy safety ==="
bash tests/test_deploy_safety.sh

echo "All automated tests passed."
