#!/bin/sh
# Build Scum Lab's WASM module. --open also serves the repo and opens the page.
set -eu

open_page=0
for arg in "$@"; do
  case "$arg" in
    --open) open_page=1 ;;
    -h|--help)
      printf 'usage: tools/scum-lab/build.sh [--open]\n'
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$arg" >&2
      exit 1
      ;;
  esac
done

cd "$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"

cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" tools/scum-lab/wasm_exec.js
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o tools/scum-lab/scumlab.wasm ./tools/scum-lab/wasm
printf 'built tools/scum-lab/scumlab.wasm\n'

if [ "$open_page" -eq 0 ]; then
  exit 0
fi

port=8765
url="http://127.0.0.1:${port}/tools/scum-lab/index.html"

browse() {
  if command -v open >/dev/null 2>&1; then
    open "$url"
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url"
  else
    printf '%s\n' "$url"
  fi
}

if curl -sf -o /dev/null --max-time 1 "$url"; then
  browse
  exit 0
fi

ruby -run -e httpd . -p "$port" -b 127.0.0.1 &
server=$!
trap 'kill "$server" 2>/dev/null || true' EXIT INT TERM

ready=0
i=0
while [ "$i" -lt 25 ]; do
  if curl -sf -o /dev/null --max-time 1 "$url"; then
    ready=1
    break
  fi
  if ! kill -0 "$server" 2>/dev/null; then
    break
  fi
  i=$((i + 1))
  sleep 0.2
done

if [ "$ready" -ne 1 ]; then
  printf 'could not serve %s\n' "$url" >&2
  exit 1
fi

printf 'serving %s\n' "$url"
browse
wait "$server"
