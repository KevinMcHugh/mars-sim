#!/usr/bin/env bash
# determinism-check.sh runs the golden world-hash test on every target a seed
# has to agree across: this machine, the other common CPU (amd64 on Apple
# silicon via Rosetta, arm64 elsewhere where it can run), and js/wasm under
# Node, which is what the browser frontend runs. A seed that hashes
# differently on one of them is a determinism bug, not a flaky test: floats,
# map order, or an int-size assumption. See docs/determinism.md.
set -euo pipefail
cd "$(dirname "$0")/.."

tests='TestGoldenWorldHash|TestDeterministic|TestChunk|TestLazy|TestGenerationStays|TestPreview'
pkg=./internal/sim
fail=0

run() {
	local label=$1
	shift
	printf '== %s\n' "$label"
	if ! "$@" go test -count=1 -run "$tests" "$pkg"; then
		fail=1
	fi
}

run "native ($(go env GOHOSTARCH))" env

case "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" in
darwin/arm64)
	if arch -x86_64 /usr/bin/true 2>/dev/null; then
		run "amd64 (Rosetta)" env GOARCH=amd64
	else
		echo "== amd64: skipped (Rosetta not installed)"
	fi
	;;
*)
	echo "== second CPU architecture: skipped (no emulator wired up for $(go env GOHOSTOS)/$(go env GOHOSTARCH))"
	;;
esac

# go test finds go_js_wasm_exec on PATH by itself; it lives in lib/wasm
# (Go 1.24+) or misc/wasm (older).
goroot="$(go env GOROOT)"
if ! command -v node >/dev/null; then
	echo "== js/wasm: skipped (node not on PATH)"
elif [ -x "$goroot/lib/wasm/go_js_wasm_exec" ]; then
	run "js/wasm (node $(node --version))" env PATH="$goroot/lib/wasm:$PATH" GOOS=js GOARCH=wasm
elif [ -x "$goroot/misc/wasm/go_js_wasm_exec" ]; then
	run "js/wasm (node $(node --version))" env PATH="$goroot/misc/wasm:$PATH" GOOS=js GOARCH=wasm
else
	echo "== js/wasm: skipped (go_js_wasm_exec not found under $goroot)"
fi

exit $fail
