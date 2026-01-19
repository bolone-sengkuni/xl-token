#!/usr/bin/env bash
set -euo pipefail

NDK="/c/Android/android-ndk-r27d"
TOOLCHAIN="$NDK/toolchains/llvm/prebuilt/windows-x86_64/bin"

export GOOS=android
export GOARCH=arm64
export CGO_ENABLED=1
export CC="$TOOLCHAIN/aarch64-linux-android21-clang"

# opsional: cek file clang ada
[ -f "$CC" ] || { echo "clang not found: $CC"; exit 1; }

go build -o xl-token
