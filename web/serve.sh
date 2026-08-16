#!/bin/sh
set -e
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
"$root/web/build.sh"
echo "open http://127.0.0.1:8080/"
echo "(Ctrl+C to stop)"
cd "$root/web"
exec python3 -m http.server 8080 --bind 127.0.0.1
