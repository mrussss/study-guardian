#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
FIXTURE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/studyguardian-deploy-safety.XXXXXX")"
cleanup() { rm -rf -- "${FIXTURE_ROOT}"; }
trap cleanup EXIT

FIXTURE_REPO="${FIXTURE_ROOT}/repo"
TARGET_DIR="${FIXTURE_ROOT}/runtime"
mkdir -p "${FIXTURE_REPO}/scripts" "${FIXTURE_REPO}/configs" "${FIXTURE_ROOT}/bin"

# Exercise the production script's filesystem behavior without targeting the
# user's installed runtime or launching Windows processes. Only its exact target
# allowlist entry changes in this disposable copy; a changed guard fails closed.
# Windows process stop/health checks require a separate real-runtime smoke test.
python3 - "${REPO_ROOT}/scripts/deploy-windows.sh" "${FIXTURE_REPO}/scripts/deploy-windows.sh" "${TARGET_DIR}" <<'PY'
from pathlib import Path
import shlex
import sys

source, destination, target = sys.argv[1:]
script = Path(source).read_text()
guard = '    /mnt/d/StudyGuardianDev) ;;'
if script.count(guard) != 1:
    raise SystemExit('Deploy target guard changed; update the isolated test adapter.')
Path(destination).write_text(script.replace(guard, f'    {shlex.quote(target)}) ;;'))
PY

export DEPLOY_TEST_CALLS="${FIXTURE_ROOT}/powershell-calls.log"
export POWERSHELL_BIN="${FIXTURE_ROOT}/bin/powershell-stub"
cat > "${POWERSHELL_BIN}" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
for argument in "$@"; do
    case "${argument}" in
        -File) printf 'stop\n' >> "${DEPLOY_TEST_CALLS}"; exit 0 ;;
        -Command)
            printf 'smoke\n' >> "${DEPLOY_TEST_CALLS}"
            if [ "${DEPLOY_TEST_FAIL_SMOKE:-0}" = 1 ]; then exit 17; fi
            exit 0 ;;
    esac
done
echo 'Unexpected PowerShell invocation' >&2
exit 1
SH
# Path conversion is consumed only by the process stub, so this test also works
# in Linux environments without WSL interop or wslpath.
cat > "${FIXTURE_ROOT}/bin/wslpath" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
test "$#" -eq 2
test "$1" = -w
printf '%s\n' "$2"
SH
chmod +x "${POWERSHELL_BIN}" "${FIXTURE_ROOT}/bin/wslpath"
export PATH="${FIXTURE_ROOT}/bin:${PATH}"

# Minimal already-built artifacts satisfy the production deploy checks. No
# compiler, Windows executable, database, or user configuration is needed.
ARTIFACTS="${FIXTURE_REPO}/dist/windows"
mkdir -p "${ARTIFACTS}/bin" "${ARTIFACTS}/pet/src" \
    "${ARTIFACTS}/pet/assets/skins/studyguardian-pixel" \
    "${ARTIFACTS}/sensor/screen" "${ARTIFACTS}/browser/chatgpt-collector/dist"
printf 'supervisor-v1\n' > "${ARTIFACTS}/bin/study-supervisor.exe"
printf 'config-helper\n' > "${ARTIFACTS}/bin/config-helper.exe"
printf '# fixture pet\n' > "${ARTIFACTS}/pet/src/main.py"
printf '# fixture sensor\n' > "${ARTIFACTS}/sensor/screen/server.py"
printf '# fixture dependencies\n' > "${ARTIFACTS}/pet/requirements.txt"
printf '# fixture dependencies\n' > "${ARTIFACTS}/sensor/requirements.txt"
printf '{}\n' > "${ARTIFACTS}/pet/assets/skins/studyguardian-pixel/manifest.json"
printf '{}\n' > "${ARTIFACTS}/browser/chatgpt-collector/manifest.json"
printf 'console.log("fixture");\n' > "${ARTIFACTS}/browser/chatgpt-collector/dist/content.js"
printf '# fixture stop\n' > "${FIXTURE_REPO}/scripts/stop-all.ps1"
printf 'default-fixture-config\n' > "${FIXTURE_REPO}/configs/default.yaml"

mkdir -p "${TARGET_DIR}/pet/src" "${TARGET_DIR}/scripts" \
    "${TARGET_DIR}/config/pet-skins/user-canary" \
    "${TARGET_DIR}/pet/.venv" "${TARGET_DIR}/sensor/.venv"
for directory in config data logs run handoff; do
    mkdir -p "${TARGET_DIR}/${directory}"
    printf '%s-canary\n' "${directory}" > "${TARGET_DIR}/${directory}/canary"
done
printf 'fixture-auth-token\n' > "${TARGET_DIR}/config/auth.token"
printf 'existing-fixture-config\n' > "${TARGET_DIR}/config/config.yaml"
printf 'fixture-user-skin\n' > "${TARGET_DIR}/config/pet-skins/user-canary/manifest.json"
printf 'fixture-database\n' > "${TARGET_DIR}/data/studyguardian.db"
printf 'stale-pet-source\n' > "${TARGET_DIR}/pet/src/stale-v06.py"
printf 'old-stop-script\n' > "${TARGET_DIR}/scripts/stop-all.ps1"
printf 'pet-venv-canary\n' > "${TARGET_DIR}/pet/.venv/canary"
printf 'sensor-venv-canary\n' > "${TARGET_DIR}/sensor/.venv/canary"

PERSISTENT_SNAPSHOT="${FIXTURE_ROOT}/persistent-before"
mkdir -p "${PERSISTENT_SNAPSHOT}"
for directory in config data logs run handoff pet/.venv sensor/.venv; do
    mkdir -p "${PERSISTENT_SNAPSHOT}/$(dirname "${directory}")"
    cp -a "${TARGET_DIR}/${directory}" "${PERSISTENT_SNAPSHOT}/${directory}"
done
assert_persistent_unchanged() {
    for directory in config data logs run handoff pet/.venv sensor/.venv; do
        diff -r "${PERSISTENT_SNAPSHOT}/${directory}" "${TARGET_DIR}/${directory}"
    done
}

echo '=== Isolated deploy safety: unexpected target rejected ==='
if bash "${FIXTURE_REPO}/scripts/deploy-windows.sh" "${FIXTURE_ROOT}/unexpected-target"; then
    echo 'FAIL: unexpected deployment target was accepted' >&2
    exit 1
fi
test ! -e "${DEPLOY_TEST_CALLS}"

echo '=== Isolated deploy safety: first deployment ==='
bash "${FIXTURE_REPO}/scripts/deploy-windows.sh" "${TARGET_DIR}"
assert_persistent_unchanged
test ! -e "${TARGET_DIR}/pet/src/stale-v06.py"
cmp "${ARTIFACTS}/bin/study-supervisor.exe" "${TARGET_DIR}/bin/study-supervisor.exe"

echo '=== Isolated deploy safety: repeated deployment ==='
printf 'supervisor-v2\n' > "${ARTIFACTS}/bin/study-supervisor.exe"
bash "${FIXTURE_REPO}/scripts/deploy-windows.sh" "${TARGET_DIR}"
assert_persistent_unchanged
cmp "${ARTIFACTS}/bin/study-supervisor.exe" "${TARGET_DIR}/bin/study-supervisor.exe"

echo '=== Isolated deploy safety: failed smoke restores previous runtime ==='
cp -a "${TARGET_DIR}" "${FIXTURE_ROOT}/runtime-before-failure"
printf 'supervisor-v3\n' > "${ARTIFACTS}/bin/study-supervisor.exe"
if DEPLOY_TEST_FAIL_SMOKE=1 bash "${FIXTURE_REPO}/scripts/deploy-windows.sh" "${TARGET_DIR}"; then
    echo 'FAIL: deployment succeeded after the health smoke failed' >&2
    exit 1
fi
diff -r "${FIXTURE_ROOT}/runtime-before-failure" "${TARGET_DIR}"
assert_persistent_unchanged
test "$(grep -c '^stop$' "${DEPLOY_TEST_CALLS}")" -eq 3
test "$(grep -c '^smoke$' "${DEPLOY_TEST_CALLS}")" -eq 3

echo 'PASS: four isolated deploy safety scenarios; persistent data and previous runtime preserved.'
