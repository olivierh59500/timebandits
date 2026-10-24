#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
"$script_dir/build-android.sh"
. "$script_dir/android-env.sh"
if [ -z "${ANDROID_SERIAL:-}" ]; then
    device_count=$("$adb_path" devices | awk 'NR > 1 && $2 == "device" { count++ } END { print count + 0 }')
    if [ "$device_count" -ne 1 ]; then
        echo "Exactly one authorized Android device is required." >&2
        exit 1
    fi
    ANDROID_SERIAL=$("$adb_path" devices | awk 'NR > 1 && $2 == "device" { print $1; exit }')
    export ANDROID_SERIAL
fi
"$adb_path" install -r "$apk_path"
"$adb_path" shell am start -S -W -n "$application_id/.MainActivity"
