#!/usr/bin/env bash
# determinism-check.sh runs the golden world-hash test on every target a seed
# has to agree across: this machine, the other common CPU (amd64 on Apple
# silicon via Rosetta, arm64 elsewhere where it can run), and js/wasm under
# Node, which is what the browser frontend runs. A seed that hashes
# differently on one of them is a determinism bug, not a flaky test: floats,
# map order, or an int-size assumption. See docs/determinism.md.
set -euo pipefail
cd "$(dirname "$0")/.."

tests='TestGoldenWorldHash|TestDeterministic|TestChunk'
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

run "native ($(go env GOARCH))" env

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

if command -v node >/dev/null; then
	wasmexec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec"
	[ -x "$wasmexec" ] || wasmexec="$(go env GOROOT)/misc/wasm/go_js_wasm_exec"
	run "js/wasm (node $(node --version))" env GOOS=js GOARCH=wasm GOFLAGS="-exec=$wasmexec"
else
	echo "== js/wasm: skipped (node not on PATH)"
fi

exit $fail
