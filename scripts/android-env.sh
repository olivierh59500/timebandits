#!/bin/sh
# Shared Android settings for the Time Bandits build scripts.

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
android_cache="$project_root/.cache/android"
android_sdk=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
java_home_path=${JAVA_HOME:-}
ebiten_version=v2.10.2
android_api=23
compile_sdk=36
build_tools_version=36.0.0
ndk_version=${TIMEBANDITS_ANDROID_NDK_VERSION:-28.2.13676358}
application_id=com.olivierh.timebandits
android_target=${TIMEBANDITS_ANDROID_TARGET:-android/arm64}

if [ -z "$android_sdk" ]; then
    for candidate in /opt/homebrew/share/android-commandlinetools "$HOME/Library/Android/sdk" "$HOME/Android/Sdk"; do
        if [ -f "$candidate/platforms/android-$compile_sdk/android.jar" ]; then
            android_sdk=$candidate
            break
        fi
    done
fi
if [ -z "$java_home_path" ]; then
    for candidate in /opt/homebrew/opt/openjdk@21 /opt/homebrew/opt/openjdk@17 "/Applications/Android Studio.app/Contents/jbr/Contents/Home"; do
        if [ -x "$candidate/bin/java" ]; then
            java_home_path=$candidate
            break
        fi
    done
fi
if [ -z "$android_sdk" ] || [ ! -f "$android_sdk/platforms/android-$compile_sdk/android.jar" ]; then
    echo "Android SDK $compile_sdk is required." >&2
    exit 1
fi
android_ndk=${ANDROID_NDK_HOME:-"$android_sdk/ndk/$ndk_version"}
if [ ! -f "$android_ndk/meta/platforms.json" ]; then
    echo "Android NDK $ndk_version is required." >&2
    exit 1
fi
if [ -z "$java_home_path" ] || [ ! -x "$java_home_path/bin/java" ]; then
    echo "Java 17 or 21 is required." >&2
    exit 1
fi

export ANDROID_HOME="$android_sdk"
export ANDROID_SDK_ROOT="$android_sdk"
export ANDROID_NDK_HOME="$android_ndk"
export ANDROID_USER_HOME="$android_cache/sdk-user"
export JAVA_HOME="$java_home_path"
export PATH="$java_home_path/bin:$android_sdk/platform-tools:$PATH"
export GOWORK=off
export GOTOOLCHAIN=${GOTOOLCHAIN:-local}
export GOPATH=${TIMEBANDITS_ANDROID_GOPATH:-"$android_cache/go"}
export GOMODCACHE=${TIMEBANDITS_ANDROID_GOMODCACHE:-"$android_cache/go/pkg/mod"}
export GOCACHE=${TIMEBANDITS_ANDROID_GOCACHE:-"$android_cache/go-build"}
export GRADLE_USER_HOME=${TIMEBANDITS_ANDROID_GRADLE_HOME:-"$android_cache/gradle"}

adb_path="$android_sdk/platform-tools/adb"
apk_path="$project_root/android/app/build/outputs/apk/debug/app-debug.apk"
apksigner_path="$android_sdk/build-tools/$build_tools_version/apksigner"
zipalign_path="$android_sdk/build-tools/$build_tools_version/zipalign"
