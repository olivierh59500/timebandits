#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$script_dir/android-env.sh"
cd "$project_root"
mkdir -p android/app/libs "$GOPATH" "$GOMODCACHE" "$GOCACHE" "$GRADLE_USER_HOME" "$ANDROID_USER_HOME"

if [ "$(go list -m -f '{{.Version}}' github.com/hajimehoshi/ebiten/v2)" != "$ebiten_version" ]; then
    echo "Ebitengine and ebitenmobile versions differ." >&2
    exit 1
fi
binding_tools="$android_cache/bind-tools"
mkdir -p "$binding_tools"
cat > "$binding_tools/go.mod" <<EOF
module timebandits-android-tools

go 1.26.0

require github.com/hajimehoshi/ebiten/v2 $ebiten_version
EOF
(
    cd "$binding_tools"
    CGO_ENABLED=0 go build -mod=mod -o "$binding_tools/ebitenmobile" github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile
)
CGO_ENABLED=0 \
CGO_LDFLAGS="${CGO_LDFLAGS:-} -Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384" \
"$binding_tools/ebitenmobile" bind -target "$android_target" -androidapi "$android_api" \
    -javapkg "$application_id" -o android/app/libs/timebandits.aar ./mobile

debug_keystore="$android_cache/debug.keystore"
if [ ! -f "$debug_keystore" ]; then
    "$JAVA_HOME/bin/keytool" -genkeypair -keystore "$debug_keystore" -storetype PKCS12 \
        -storepass android -alias androiddebugkey -keypass android \
        -dname "CN=Android Debug,O=Android,C=US" -keyalg RSA -keysize 2048 -validity 10000
fi
rm -f "$apk_path"
"$project_root/android/gradlew" -p "$project_root/android" --console=plain :app:assembleDebug
"$apksigner_path" verify "$apk_path"
"$zipalign_path" -c -P 16 4 "$apk_path"
printf 'APK ready: %s\n' "$apk_path"
