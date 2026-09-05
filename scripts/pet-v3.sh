#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PET_ROOT="${REPO_ROOT}/pet-v3"
POWERSHELL_BIN="${POWERSHELL_BIN:-/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe}"
BUILD_ROOT_WIN="${STUDYGUARDIAN_BUILD_ROOT_WIN:-D:\\StudyGuardianBuild}"

usage() {
    cat <<'EOF'
Usage: ./scripts/pet-v3.sh <command>

  dev          Start the WSL Vite development server.
  check        Run Pet frontend tests, type/build checks, and diff checks.
  native       Produce an incremental Windows debug executable.
  candidate    Run checks, Windows Rust tests, build, deploy, and verify Pet only.
  build        Produce a tested Windows release Pet artifact in dist/windows.
  deploy       Deploy the existing Windows debug artifact and restart Pet only.
  verify       Compare the debug artifact with the running Pet executable.
  cache-status Show the persistent Windows build-cache size.
  release      Run the existing full-product Windows build pipeline.
EOF
}

require_windows_tools() {
    if [[ ! -x "${POWERSHELL_BIN}" ]]; then
        echo "Windows PowerShell is unavailable: ${POWERSHELL_BIN}" >&2
        exit 1
    fi
}

git_commit="$(git -C "${REPO_ROOT}" rev-parse HEAD)"
if [[ -n "$(git -C "${REPO_ROOT}" status --porcelain)" ]]; then git_dirty=true; else git_dirty=false; fi
repo_root_win="$(wslpath -w "${REPO_ROOT}")"

run_frontend_tests() (
    cd "${PET_ROOT}"
    npm test
    git -C "${REPO_ROOT}" diff --check
)

build_native() {
    local configuration="$1"
    local output_path="${2:-}"
    local rust_tests="${3:-false}"
    require_windows_tools
    args=(
        -NoProfile -ExecutionPolicy Bypass
        -File "$(wslpath -w "${SCRIPT_DIR}/build-pet-v3-windows.ps1")"
        -RepoRoot "${repo_root_win}"
        -BuildRoot "${BUILD_ROOT_WIN}"
        -Configuration "${configuration}"
        -GitCommit "${git_commit}"
        -GitDirty "${git_dirty}"
    )
    if [[ -n "${output_path}" ]]; then args+=( -OutputPath "${output_path}" ); fi
    if [[ "${rust_tests}" == true ]]; then args+=( -RunRustTests ); fi
    "${POWERSHELL_BIN}" "${args[@]}"
}

command="${1:-help}"
case "${command}" in
    dev)
        cd "${PET_ROOT}"
        exec npm run dev
        ;;
    check)
        cd "${PET_ROOT}"
        npm test
        npm run build
        git -C "${REPO_ROOT}" diff --check
        ;;
    native)
        build_native Debug
        ;;
    deploy)
        require_windows_tools
        "${POWERSHELL_BIN}" -NoProfile -ExecutionPolicy Bypass -File \
            "$(wslpath -w "${SCRIPT_DIR}/deploy-pet-v3-windows.ps1")"
        ;;
    verify)
        require_windows_tools
        "${POWERSHELL_BIN}" -NoProfile -ExecutionPolicy Bypass -File \
            "$(wslpath -w "${SCRIPT_DIR}/deploy-pet-v3-windows.ps1")" -VerifyOnly
        ;;
    candidate)
        run_frontend_tests
        build_native Debug "" true
        "${BASH_SOURCE[0]}" deploy
        "${BASH_SOURCE[0]}" verify
        ;;
    build)
        run_frontend_tests
        output_path="$(wslpath -w "${REPO_ROOT}/dist/windows/pet-v3/StudyGuardian.exe")"
        build_native Release "${output_path}" true
        ;;
    cache-status)
        require_windows_tools
        "${POWERSHELL_BIN}" -NoProfile -Command \
            "\$p='${BUILD_ROOT_WIN}'; if(Test-Path -LiteralPath \$p){\$f=Get-ChildItem -LiteralPath \$p -Recurse -File -Force -ErrorAction SilentlyContinue; [pscustomobject]@{Path=\$p;Files=\$f.Count;SizeGB=[math]::Round(((\$f|Measure-Object Length -Sum).Sum/1GB),2)}|Format-List}else{Write-Host 'Build cache is empty.'}"
        ;;
    release)
        exec bash "${SCRIPT_DIR}/build-windows.sh"
        ;;
    help|-h|--help)
        usage
        ;;
    *)
        echo "Unknown command: ${command}" >&2
        usage >&2
        exit 2
        ;;
esac
