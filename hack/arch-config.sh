# ---------------------------------------------------------------------------
# Shared architecture configuration for AREDN OpenWrt builds
#
# Sourced by build.sh and openwrt/build.sh. Not executable on its own.
#
# Sets: GOARCH, GOMIPS, GOARM, PKG_ARCH
# ---------------------------------------------------------------------------

ALL_ARCHES="mips_24kc mipsel_24kc arm_cortex-a7_neon-vfpv4 aarch64_cortex-a53 x86_64"

print_arch_list() {
    cat <<'ARCH_LIST'
  mips_24kc                  ath79 - Ubiquiti, TP-Link, Mikrotik SXT/LHG/LDF, GL.iNet
  mipsel_24kc                ramips - GL-MT1300, HaLowLink, Heltec, Alfa Tube-AHM, Cudy TR1200
  arm_cortex-a7_neon-vfpv4   ipq40xx - Mikrotik hAP ac2/ac3, SXTsq 5ac, GL-B1300
  aarch64_cortex-a53         mediatek/filogic - OpenWrt One, Cudy TR3000
  x86_64                     x86/64 - VMware ESXi, Proxmox, VirtualBox, Bhyve
ARCH_LIST
}

# Node image used to build the MeshMap site, the same one MeshMap's Dockerfile uses.
# renovate: datasource=docker depName=node
NODE_IMAGE="node:25.8.2-alpine@sha256:cf38e1f3c28ac9d81cdc0c51d8220320b3b618780e44ef96a39f76f7dbfef023"

# apk-tools 3 image used to build APK packages for AREDN 4.26 and later.
# renovate: datasource=docker depName=alpine
APK_IMAGE="alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63"

configure_arch() {
    local arch="$1"
    GOMIPS=""
    GOARM=""
    case "$arch" in
        mips_24kc)
            GOARCH="mips"
            GOMIPS="softfloat"
            ;;
        mipsel_24kc)
            GOARCH="mipsle"
            GOMIPS="softfloat"
            ;;
        arm_cortex-a7_neon-vfpv4)
            GOARCH="arm"
            GOARM="7"
            ;;
        aarch64_cortex-a53)
            GOARCH="arm64"
            ;;
        x86_64)
            GOARCH="amd64"
            ;;
        *)
            echo "ERROR: Unknown architecture: $arch"
            return 1
            ;;
    esac
    PKG_ARCH="$arch"
}
