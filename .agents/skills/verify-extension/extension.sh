#!/usr/bin/env bash
# Build the freehire extension from this checkout into a stable unpacked folder, and
# report what the signed-in Chrome profile has loaded. Read-only except `build`.
#
#   extension.sh build    wxt build (prod API, pinned dev key) -> $FH_EXT_DIR
#   extension.sh doctor   build vs installed: versions, ids, enabled state, path
#   extension.sh evidence print (and create) a fresh evidence directory
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
FH_EXT_DIR="${FH_EXT_DIR:-$HOME/.freehire-verify/extension}"
FH_CHROME_PROFILE="${FH_CHROME_PROFILE:-Profile 1}"
CHROME_DIR="$HOME/Library/Application Support/Google/Chrome"
# The unpacked build's id comes from DEV_KEY in extension/wxt.config.ts; the store build's id
# is assigned by the Chrome Web Store.
DEV_ID=ebhfafmnihndlaoibgjnfjhkjlnaencj
STORE_ID=ijfaechijopdlikalojadpojmpilplnj

cmd="${1:-doctor}"
case "$cmd" in
  build)
    cd "$ROOT/extension"
    [ -d node_modules ] || pnpm install --frozen-lockfile
    npx wxt build
    rm -rf "$FH_EXT_DIR"
    mkdir -p "$(dirname "$FH_EXT_DIR")"
    cp -R .output/chrome-mv3 "$FH_EXT_DIR"
    echo "built $(python3 -c "import json;print(json.load(open('$FH_EXT_DIR/manifest.json'))['version'])") -> $FH_EXT_DIR"
    echo "now: chrome://extensions -> reload the unpacked freehire card (or Load unpacked the first time)"
    ;;
  doctor)
    python3 - "$FH_EXT_DIR" "$CHROME_DIR/$FH_CHROME_PROFILE" "$DEV_ID" "$STORE_ID" <<'PY'
import json, os, sys
ext_dir, profile, dev_id, store_id = sys.argv[1:]
ok = True
try:
    built = json.load(open(os.path.join(ext_dir, "manifest.json")))
    print(f"build      {built['version']} at {ext_dir} (key pinned: {'key' in built})")
except FileNotFoundError:
    print(f"build      MISSING at {ext_dir}: run extension.sh build"); ok = False
try:
    prefs = json.load(open(os.path.join(profile, "Secure Preferences")))
except FileNotFoundError:
    print(f"profile    MISSING: {profile}"); sys.exit(1)
settings = prefs.get("extensions", {}).get("settings", {})
for label, eid in (("unpacked", dev_id), ("store", store_id)):
    e = settings.get(eid)
    if not e:
        print(f"{label:10} not installed ({eid})")
        if label == "unpacked": ok = False
        continue
    enabled = not e.get("disable_reasons")
    path = e.get("path", "")
    version = e.get("manifest", {}).get("version")
    if not version and os.path.isabs(path):  # an unpacked entry keeps no manifest copy
        version = json.load(open(os.path.join(path, "manifest.json")))["version"]
    print(f"{label:10} {version} enabled={enabled} path={path}")
    if label == "unpacked" and (not enabled or os.path.realpath(path) != os.path.realpath(ext_dir)):
        ok = False
    if label == "store" and enabled:
        print("           store build is ON: two freehire panels compete, turn it off for the run")
print("doctor     OK" if ok else "doctor     NOT READY")
sys.exit(0 if ok else 1)
PY
    echo "note: Chrome writes Secure Preferences lazily; a toggle made seconds ago may not show yet"
    ;;
  evidence)
    dir="${FH_EVIDENCE_DIR:-$HOME/.freehire-verify/evidence}/$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$dir"
    echo "$dir"
    ;;
  *)
    echo "usage: $0 build|doctor|evidence" >&2; exit 2 ;;
esac
