#!/bin/sh
# Build the engine for the browser: cmd/mars-sim-wasm as web/public/mars-sim.wasm,
# plus Go's JS glue (wasm_exec.js) from the same toolchain. Vite serves and
# ships web/public as is, next to the worker (web/public/worker.js).
set -eu

cd "$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"

# The toolchain's copy is read-only, and cp keeps its mode, so remove the last
# build's copy first (cp cannot overwrite it) and leave the new one writable.
rm -f web/public/wasm_exec.js
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js
chmod u+w web/public/wasm_exec.js
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o web/public/mars-sim.wasm ./cmd/mars-sim-wasm
printf 'built web/public/mars-sim.wasm\n'

# The species pack pulled from Creature Lab (go run . -fetch-species URL),
# served beside the page, which hands it to the engine at each new game. No
# pack at the root means none here either, so a stale copy cannot linger.
if [ -f species-pack.json ]; then
  cp species-pack.json web/public/species-pack.json
  printf 'copied species-pack.json\n'
else
  rm -f web/public/species-pack.json
fi
