#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Multi-architecture build for AREDN OpenWrt
#
# Usage:
#   ./build.sh <arch>       Build one architecture
#   ./build.sh all          Build all AREDN architectures
#
# Builds the MeshMap site and static mesh-walker binaries, then packages them
# as IPK + APK.
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"
PKG_NAME="meshmap"
PKG_VERSION="${PKG_VERSION:-0.0.0}"
DIST="$PROJECT_ROOT/dist"

# shellcheck source=hack/arch-config.sh
source "$PROJECT_ROOT/hack/arch-config.sh"

usage() {
    echo "Usage: $(basename "$0") <arch|all>"
    echo ""
    echo "Architectures:"
    print_arch_list
    echo "  all                        Build all of the above (default)"
    exit 1
}

REQUESTED="${1:-all}"
case "$REQUESTED" in
    -h|--help) usage ;;
esac
if [ "$REQUESTED" = "all" ]; then
    ARCHES="$ALL_ARCHES"
else
    ARCHES="$REQUESTED"
fi

build_site() {
    if [ ! -f "$PROJECT_ROOT/meshmap/package.json" ]; then
        echo "ERROR: meshmap submodule is missing. Run: git submodule update --init"
        exit 1
    fi

    echo ""
    echo "================================================================"
    echo "==> Building MeshMap site"
    echo "================================================================"

    rm -rf "$DIST/site"
    mkdir -p "$DIST/site"

    # Build from a copy so node_modules never lands in the submodule. The
    # relative base makes the site work under /meshmap/.
    # shellcheck disable=SC2086
    docker run --rm ${DOCKER_RUN_ARGS:-} \
        --user "$(id -u):$(id -g)" \
        -v "$PROJECT_ROOT/meshmap":/src:ro \
        -v "$DIST/site":/out \
        -e HOME=/tmp/build \
        "$NODE_IMAGE" \
        sh -euc '
            mkdir -p /tmp/build/app
            cd /tmp/build/app
            cp -r /src/package.json /src/package-lock.json /src/src /src/index.html \
                /src/vite.config.js /src/.env /src/public .
            npm ci --ignore-scripts --no-audit --no-fund
            npm run build -- --base=./ --outDir /out --emptyOutDir
        '
}

build_arch() {
    local arch="$1"
    configure_arch "$arch"

    echo ""
    echo "================================================================"
    echo "==> [$arch] Building mesh-walker"
    echo "================================================================"

    mkdir -p "$DIST/$arch"
    (
        cd "$PROJECT_ROOT"
        CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" GOMIPS="$GOMIPS" GOARM="$GOARM" \
            go build -trimpath -ldflags "-s -w -buildid= -X main.version=$PKG_VERSION" \
            -o "$DIST/$arch/mesh-walker" .
    )

    echo "==> [$arch] Build complete:"
    ls -lh "$DIST/$arch/mesh-walker"
    if command -v file >/dev/null; then
        file "$DIST/$arch/mesh-walker"
    fi
}

if [ "${SKIP_SITE:-0}" != "1" ] || [ ! -f "$DIST/site/index.html" ]; then
    build_site
fi

for arch in $ARCHES; do
    build_arch "$arch"
done

echo ""
echo "==> All binaries built. Packaging..."
export PKG_VERSION
"$PROJECT_ROOT/openwrt/build.sh" "$REQUESTED"
