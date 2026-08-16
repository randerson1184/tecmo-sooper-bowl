#!/bin/sh
# Produce web/game.wasm + web/wasm_exec.js for local serve or Pages.
set -e
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"

execjs="$(go env GOROOT)/lib/wasm/wasm_exec.js"
if [ ! -f "$execjs" ]; then
  execjs="$(go env GOROOT)/misc/wasm/wasm_exec.js"
fi
if [ ! -f "$execjs" ]; then
  echo "wasm_exec.js not found under $(go env GOROOT)" >&2
  exit 1
fi

build="${BUILD_ID:-dev}"
echo "building web/game.wasm (build=$build)…"
GOOS=js GOARCH=wasm go build -ldflags "-X main.buildID=${build}" -o web/game.wasm ./cmd/game
cp "$execjs" web/wasm_exec.js
touch web/.nojekyll
echo "ok"
