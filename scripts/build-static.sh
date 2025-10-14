#!/bin/sh

GARM_SOURCE="/build/garm-agent"
git config --global --add safe.directory /build/garm-agent
cd $GARM_SOURCE

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ ! -z "$GARM_REF" ] && [ "$GARM_REF" != "$CURRENT_BRANCH" ];then
    git checkout $GARM_REF
fi

cd $GARM_SOURCE

OUTPUT_DIR="/build/output"
VERSION=$(git describe --tags --match='v[0-9]*' --dirty --always)
BUILD_DIR="$OUTPUT_DIR/$VERSION"


[ ! -d "$BUILD_DIR/linux" ] && mkdir -p "$BUILD_DIR/linux"
[ ! -d "$BUILD_DIR/windows" ] && mkdir -p "$BUILD_DIR/windows"

export CGO_ENABLED=1
USER_ID=${USER_ID:-$UID}
USER_GROUP=${USER_GROUP:-$(id -g)}

# Linux
GOOS=linux GOARCH=amd64 go build \
    -o $BUILD_DIR/linux/amd64/garm-agent \
    -tags osusergo,netgo,sqlite_omit_load_extension \
    -ldflags "-extldflags '-static' -s -w -X github.com/cloudbase/garm-agent/cmd.Version=$VERSION" .
GOOS=linux GOARCH=arm64 CC=aarch64-linux-musl-gcc go build \
    -o $BUILD_DIR/linux/arm64/garm-agent \
    -tags osusergo,netgo,sqlite_omit_load_extension \
    -ldflags "-extldflags '-static' -s -w -X github.com/cloudbase/garm-agent/cmd.Version=$VERSION" .

# Windows
GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-cc go build \
    -o $BUILD_DIR/windows/amd64/garm-agent.exe \
    -tags osusergo,netgo,sqlite_omit_load_extension \
    -ldflags "-s -w -X github.com/cloudbase/garm-agent/cmd.Version=$VERSION" .

git checkout $CURRENT_BRANCH || true
chown $USER_ID:$USER_GROUP -R "$OUTPUT_DIR"
