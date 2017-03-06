#!/usr/bin/env bash

set -e

BOT_VERSION=$(git describe --tags)
BUILD_TIME=$(date +%T-%D)
BUILD_USER="$USER"
BUILD_HOST=$(hostname)
CFLAGS=""

if [[ "$CI" == "true" ]]; then
    GOTARGET="${GOTARGET?:'A target is mandatory'}"
else
    GOTARGET="Karen"
    CFLAGS="${CFLAGS} ${@}"
fi

set -x

go-bindata -nomemcopy -nocompress -pkg helpers -o helpers/assets.go _assets/

go build ${CFLAGS} \
    -o "${GOTARGET}" \
    --ldflags="
-X git.lukas.moe/sn0w/Karen/version.BOT_VERSION=${BOT_VERSION}
-X git.lukas.moe/sn0w/Karen/version.BUILD_TIME=${BUILD_TIME}
-X git.lukas.moe/sn0w/Karen/version.BUILD_USER=${BUILD_USER}
-X git.lukas.moe/sn0w/Karen/version.BUILD_HOST=${BUILD_HOST}" .

