# Mesh Walker

Walks an AREDN mesh and writes the MeshMap data file. The `meshmap` package runs the walker on an AREDN node and serves MeshMap from that node.

## Walker

```sh
mesh-walker [--start NODE] [--out PATH] [--timeout 2m] [--retries 5] [--jitter 5s] [--concurrency 64] [--progress-interval 2s]
```

- `--start`: node to start from. The default is the hostname.
- `--out`: output file. The default is `/meshmap/data/out.json`. The walker streams responses to a temporary file in the same directory, writes `<out>.new`, and renames it to `<out>`.

The output format is the same as the mesh-manager walker: `{"date","hostsScraped","nonMapped","nodeInfo":[{"data":<sysinfo>}, ...]}`.

## AREDN package

The `meshmap` package (IPK and APK) installs:

- `/usr/bin/mesh-walker`
- MeshMap at `/www/meshmap/`, served by the node web server at `/meshmap/` on ports 80 and 8080
- `/etc/meshmap/appConfig.json`, linked from `/www/meshmap/appConfig.json` and kept across firmware upgrades
- `/www/meshmap/data`, a link to `/meshmap/data`

At boot the init script mounts the ext4 file system labeled `meshmap-data` on `/meshmap` if one exists. Without that disk the walker does not run, so it never writes to the node's flash. It logs one line per hour instead.

The walker runs about one minute after start when there is no data yet, and then every hour at minute 30. Nodes with less than 256 MB of RAM fetch 16 nodes at a time instead of 64.

On install the package saves the node's map URL to `/etc/meshmap/previous-map-url` and sets it to `/meshmap/`. On removal it restores the saved URL.

## Build

```sh
git submodule update --init
./build.sh all        # or one of: mips_24kc mipsel_24kc arm_cortex-a7_neon-vfpv4 aarch64_cortex-a53 x86_64
```

The build needs Go and Docker. Packages are written to `openwrt/`.
