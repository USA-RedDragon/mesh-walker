#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Package pre-built binaries and the MeshMap site as OpenWrt IPK + APK
#
# Usage:
#   ./openwrt/build.sh <arch>       Package one architecture
#   ./openwrt/build.sh all          Package all AREDN architectures
#
# Expects dist/site/ and dist/<arch>/mesh-walker from the root build.sh.
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PKG_NAME="meshmap"
PKG_VERSION="${PKG_VERSION:-0.0.0}"
DIST="$PROJECT_ROOT/dist"

# shellcheck source=../hack/arch-config.sh
source "$PROJECT_ROOT/hack/arch-config.sh"

usage() {
    echo "Usage: $(basename "$0") <arch|all>"
    echo ""
    echo "Architectures:"
    print_arch_list
    echo "  all                        Package all of the above"
    exit 1
}

[ $# -lt 1 ] && usage

REQUESTED="$1"
if [ "$REQUESTED" = "all" ]; then
    ARCHES="$ALL_ARCHES"
else
    ARCHES="$REQUESTED"
fi

# ---------------------------------------------------------------------------
# Package one architecture
# ---------------------------------------------------------------------------
package_arch() {
    local arch="$1"
    configure_arch "$arch"

    BINARY="$DIST/$arch/mesh-walker"

    if [ ! -f "$BINARY" ]; then
        echo "ERROR: Binary not found: $BINARY"
        echo "       Run ./build.sh $arch first to compile."
        exit 1
    fi
    if [ ! -f "$DIST/site/index.html" ]; then
        echo "ERROR: MeshMap site not found: $DIST/site"
        echo "       Run ./build.sh $arch first to build it."
        exit 1
    fi

    package_binary "$BINARY" "$PKG_ARCH"
}

# ---------------------------------------------------------------------------
# Package a compiled binary as IPK + APK
# ---------------------------------------------------------------------------
package_binary() {
    local binary="$1"
    local pkg_arch="$2"

    echo "==> [$pkg_arch] Packaging IPK + APK (version $PKG_VERSION)..."

    local root
    root=$(mktemp -d)

    # -- Stage data tree ----------------------------------------------------
    mkdir -p "$root/data/usr/bin" \
             "$root/data/usr/libexec/meshmap" \
             "$root/data/etc/init.d" \
             "$root/data/etc/meshmap" \
             "$root/data/etc/arednsysupgrade.d" \
             "$root/data/www/meshmap"

    cp "$binary"                      "$root/data/usr/bin/mesh-walker"
    chmod 755                         "$root/data/usr/bin/mesh-walker"
    cp "$SCRIPT_DIR/meshmap.init"     "$root/data/etc/init.d/$PKG_NAME"
    chmod 755                         "$root/data/etc/init.d/$PKG_NAME"
    cp "$SCRIPT_DIR/scheduler"        "$root/data/usr/libexec/meshmap/scheduler"
    cp "$SCRIPT_DIR/mapurl"           "$root/data/usr/libexec/meshmap/mapurl"
    chmod 755                         "$root/data/usr/libexec/meshmap/scheduler" \
                                      "$root/data/usr/libexec/meshmap/mapurl"

    # MeshMap site. appConfig.json lives in /etc so it can be edited and kept
    # across firmware upgrades; data/ points at the walker output.
    cp -R "$DIST/site/." "$root/data/www/meshmap/"
    rm -rf "$root/data/www/meshmap/appConfig.json" "$root/data/www/meshmap/data"
    cp "$PROJECT_ROOT/meshmap/public/appConfig.json" "$root/data/etc/meshmap/appConfig.json"
    ln -s /etc/meshmap/appConfig.json "$root/data/www/meshmap/appConfig.json"
    ln -s /meshmap/data               "$root/data/www/meshmap/data"
    find "$root/data/www/meshmap" -type d -exec chmod 755 {} +
    find "$root/data/www/meshmap" -type f -exec chmod 644 {} +
    chmod 644 "$root/data/etc/meshmap/appConfig.json"

    # Preserve config across AREDN firmware upgrades
    printf '%s\n' /etc/meshmap/appConfig.json /etc/meshmap/previous-map-url \
        > "$root/data/etc/arednsysupgrade.d/KI5VMF.$PKG_NAME"

    # -- Build IPK ----------------------------------------------------------
    mkdir -p "$root/control"
    echo "2.0" > "$root/debian-binary"

    cat > "$root/control/control" <<CONTROL
Package: ${PKG_NAME}
Version: ${PKG_VERSION}
Depends:
Provides:
Source: package/${PKG_NAME}
Section: net
Priority: optional
Maintainer: Jacob McSwain (KI5VMF)
Architecture: ${pkg_arch}
Description: Walk the AREDN mesh hourly and serve MeshMap from this node
CONTROL

    echo "/etc/meshmap/appConfig.json" > "$root/control/conffiles"

    cp "$SCRIPT_DIR/postinst" "$root/control/postinst"
    cp "$SCRIPT_DIR/prerm"    "$root/control/prerm"
    chmod 755 "$root/control/postinst" "$root/control/prerm"

    (cd "$root/control" && tar --owner=0 --group=0 --numeric-owner -czf ../control.tar.gz .)
    (cd "$root/data"    && tar --owner=0 --group=0 --numeric-owner -czf ../data.tar.gz .)

    local ipk="${PKG_NAME}_${PKG_VERSION}_${pkg_arch}.ipk"
    (cd "$root" && tar --owner=0 --group=0 --numeric-owner -czf "$ipk" control.tar.gz data.tar.gz debian-binary)

    rm -f "$SCRIPT_DIR/${PKG_NAME}_"*"_${pkg_arch}.ipk"
    mv "$root/$ipk" "$SCRIPT_DIR/"
    echo "    IPK: $SCRIPT_DIR/$ipk"

    # -- Build APK ----------------------------------------------------------
    # AREDN 4.26 (OpenWrt 25.12) uses apk-tools 3, which rejects a plain
    # tar.gz with a .PKGINFO ("v2 package format error"). Build a native
    # apk v3 package with apk mkpkg, the same tool OpenWrt uses.
    local apk="${PKG_NAME}-${pkg_arch}-${PKG_VERSION}.apk"
    rm -f "$SCRIPT_DIR/${PKG_NAME}-${pkg_arch}-"*.apk

    mkdir -p "$root/scripts"
    cp "$SCRIPT_DIR/postinst"    "$root/scripts/post-install"
    cp "$SCRIPT_DIR/prerm"       "$root/scripts/pre-deinstall"
    cp "$SCRIPT_DIR/postupgrade" "$root/scripts/post-upgrade"

    # shellcheck disable=SC2086
    docker run --rm ${DOCKER_RUN_ARGS:-} \
        -v "$root":/pkg:ro \
        -v "$SCRIPT_DIR":/out \
        -e "APK=$apk" \
        -e "PKG_NAME=$PKG_NAME" \
        -e "PKG_VERSION=$PKG_VERSION" \
        -e "PKG_ARCH=$pkg_arch" \
        -e "OWNER=$(id -u):$(id -g)" \
        "$APK_IMAGE" \
        sh -euc '
            cp -a /pkg/data /build
            chown -R 0:0 /build
            apk mkpkg \
                --info "name:$PKG_NAME" \
                --info "version:$PKG_VERSION" \
                --info "description:Walk the AREDN mesh hourly and serve MeshMap from this node" \
                --info "arch:$PKG_ARCH" \
                --info "license:AGPL-3.0-or-later" \
                --info "origin:$PKG_NAME" \
                --info "url:https://github.com/USA-RedDragon/mesh-walker" \
                --info "maintainer:Jacob McSwain (KI5VMF) <jacob@mcswain.dev>" \
                --script "post-install:/pkg/scripts/post-install" \
                --script "pre-deinstall:/pkg/scripts/pre-deinstall" \
                --script "post-upgrade:/pkg/scripts/post-upgrade" \
                --files /build \
                --output "/out/$APK"
            chown "$OWNER" "/out/$APK"
        '
    echo "    APK: $SCRIPT_DIR/$apk"

    # Convenience symlink for latest version
    ln -sf "$SCRIPT_DIR/$ipk" "$SCRIPT_DIR/${PKG_NAME}_${pkg_arch}.ipk"
    ln -sf "$SCRIPT_DIR/$apk" "$SCRIPT_DIR/${PKG_NAME}_${pkg_arch}.apk"

    rm -rf "$root"
}

# ---------------------------------------------------------------------------
# Main: iterate requested architectures
# ---------------------------------------------------------------------------
for arch in $ARCHES; do
    package_arch "$arch"
done

echo ""
echo "==> All requested packages complete."
