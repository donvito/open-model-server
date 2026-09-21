#!/usr/bin/env bash
set -euo pipefail

skip_install=false
case "${1:-}" in
  --skip-install) skip_install=true; shift ;;
  --help|-h) echo 'Usage: bash build.sh [--skip-install]'; exit 0 ;;
esac
if (( $# > 0 )); then
  echo 'Usage: bash build.sh [--skip-install]' >&2
  exit 1
fi

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
for tool in node npm go; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Required tool not found: $tool" >&2; exit 1; }
done

target_os=$(go env GOOS)
output=bin/modelserver
if [[ "$target_os" == windows ]]; then
  output+=.exe
fi

echo 'Building dashboard...'
(
  cd web
  if [[ "$skip_install" == false ]]; then
    npm ci
  fi
  # Use the native shell for npm scripts when invoked from Windows Git Bash.
  if [[ "${OS:-}" == Windows_NT ]]; then
    npm --script-shell=cmd.exe run build
  else
    npm run build
  fi
)

echo 'Building server...'
mkdir -p bin
go build -o "$output" ./cmd/modelserver
echo "Built $output (dashboard embedded)."
