#!/bin/sh
# Build the browser spike: the engine as WASM plus Go's JS glue, next to the
# page. --serve also serves the page on http://127.0.0.1:8766/spike/.
set -eu

cd "$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)"

cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/spike/wasm_exec.js
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web/spike/mars-sim.wasm ./cmd/mars-sim-wasm
printf 'built web/spike/mars-sim.wasm\n'

case "${1:-}" in
  --serve)
    # Serve web/, not web/spike/: the page imports ../wire/decode.js.
    printf 'serving http://127.0.0.1:8766/spike/\n'
    exec python3 -m http.server 8766 --bind 127.0.0.1 --directory web
    ;;
  "") ;;
  *)
    printf 'usage: web/spike/build.sh [--serve]\n' >&2
    exit 1
    ;;
esac
