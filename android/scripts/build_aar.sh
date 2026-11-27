#!/usr/bin/env bash
# Build android/app/libs/wgmobile.aar from pkg/wgmobile.
#
# Run from the repository root (any working directory is fine; paths are
# resolved from this file):
#
#   ./android/scripts/build_aar.sh
#
# Required environment, typical on a Mac that already has Android Studio:
#   ANDROID_HOME      Android SDK. Example:
#                     export ANDROID_HOME="$HOME/Library/Android/sdk"
#   ANDROID_NDK_HOME  Unpacked NDK (Side by side). Example:
#                     export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/26.1.10909125"
#   gomobile, gobind  On PATH. Install once:
#                     go install golang.org/x/mobile/cmd/gomobile@latest
#                     go install golang.org/x/mobile/cmd/gobind@latest
#                     export PATH="$(go env GOPATH)/bin:$PATH"
#                     gomobile init
#
# The AAR is a build product and is not committed. Open the android/ directory
# in Android Studio after this script succeeds.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail() {
  echo "error: $*" >&2
  exit 1
}

[[ -f "$ROOT/go.mod" ]] || fail "go.mod not found at $ROOT"

if ! command -v go >/dev/null 2>&1; then
  fail "go is not installed"
fi

if ! command -v gomobile >/dev/null 2>&1; then
  fail "gomobile is not on PATH. Install it, then re-run:
  go install golang.org/x/mobile/cmd/gomobile@latest
  go install golang.org/x/mobile/cmd/gobind@latest
  export PATH=\"\$(go env GOPATH)/bin:\$PATH\"
  gomobile init"
fi

if ! command -v gobind >/dev/null 2>&1; then
  fail "gobind is not on PATH. Install it with:
  go install golang.org/x/mobile/cmd/gobind@latest
  export PATH=\"\$(go env GOPATH)/bin:\$PATH\""
fi

if [[ -z "${ANDROID_HOME:-}" ]]; then
  fail "ANDROID_HOME is not set. Example:
  export ANDROID_HOME=\"\$HOME/Library/Android/sdk\""
fi
if [[ ! -d "$ANDROID_HOME" ]]; then
  fail "ANDROID_HOME does not exist: $ANDROID_HOME"
fi

if [[ -z "${ANDROID_NDK_HOME:-}" ]]; then
  fail "ANDROID_NDK_HOME is not set. Install NDK (Side by side) and point at it. Example:
  export ANDROID_NDK_HOME=\"\$ANDROID_HOME/ndk/<version>\""
fi
if [[ ! -d "$ANDROID_NDK_HOME" ]]; then
  fail "ANDROID_NDK_HOME does not exist: $ANDROID_NDK_HOME"
fi
if [[ ! -f "$ANDROID_NDK_HOME/source.properties" ]]; then
  fail "ANDROID_NDK_HOME does not look like an NDK (missing source.properties): $ANDROID_NDK_HOME"
fi

export ANDROID_HOME ANDROID_NDK_HOME

if ! go list ./pkg/wgmobile >/dev/null; then
  fail "cannot load github.com/user/mywgclient/pkg/wgmobile (run from the repo, and check go.mod)"
fi

OUT="$ROOT/android/app/libs/wgmobile.aar"
mkdir -p "$(dirname "$OUT")"

echo "binding pkg/wgmobile -> $OUT"
echo "ANDROID_HOME=$ANDROID_HOME"
echo "ANDROID_NDK_HOME=$ANDROID_NDK_HOME"

gomobile bind -target=android -androidapi 26 \
  -o "$OUT" \
  github.com/user/mywgclient/pkg/wgmobile

echo "wrote $OUT"
