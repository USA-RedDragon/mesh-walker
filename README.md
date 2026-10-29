# Mesh Walker

Walks an AREDN mesh and writes the MeshMap data file. The `meshmap` package runs the walker on an AREDN node and serves MeshMap from that node.

## Walker

```sh
mesh-walker [--start NODE] [--out PATH] [--timeout 2m] [--retries 5] [--jitter 5s] [--concurrency 64] [--progress-interval 2s]
```

- `--start`: node to start from. The default is the hostname.
- `--out`: output file. The default is `/meshmap/data/out.json`. The walker streams responses to a temporary file in the same directory, writes `<out>.new`, and renames it to `<out>`.

The output format is the same as the mesh-manager walker: `{"date","hostsScraped","nonMapped","nodeInfo":[{"data":<sysinfo>}, ...]}`.

### Map file and node details

Next to `out.json` the walker also writes a slim map file, so MeshMap can draw the map without reading the full data file, and the full records split into detail files that MeshMap reads one at a time when a node popup opens. On the Oklahoma mesh `out.json` is 60.5 MB; `map.json` is 1.6 MB (370 KB gzip-compressed) and the detail files are 256 files of 4 to 200 KB, gzip-compressed.

- `map.json` and `map.json.gz`:

  ```
  {"version":1, "date", "hostsScraped", "nonMapped",
   "details":"details-<stamp>/", "shards":256,
   "trackerColumns":["hostname","ip","canonical_ip","quality","rev_quality","ping_success_time",
                     "rev_ping_success_time","ping_quality","rev_ping_quality","babel_route_count","babel_metric"],
   "nodes":[{"node","lat","lon","mlat","mlon","chanbw",
             "meshrf":{"status","channel","freq","chanbw","azimuth"},
             "node_details":{"mesh_supernode":true},
             "interfaces":[{"ip"}],
             "link_info":[{"hostname","linkType","signal","noise","bitrate","rate"}],
             "trackers":[[<one value per trackerColumns entry>], ...]}, ...]}
  ```

  `nodes` holds every node that has a location, in the same order as `out.json`. The fields keep their sysinfo names and values, and fields a node does not have are left out. `trackers` are the node's LQM trackers. Floats in them are rounded: qualities to 0.1, ping times (in seconds) to 0.00001.
- `details-<stamp>/<shard>.json.gz`: an object of canonical node name to the node's full sysinfo record, as in `out.json`. The canonical name is the node name without a leading `.` or a `.local.mesh` suffix, in upper case. The shard is the FNV-1a 32-bit hash of the UTF-8 bytes of the canonical name, modulo `shards`, as two lower-case hex digits. For example, `KI5VMF-OKLAHOMA` is in `7e`, `KI5VMF-OKLAHOMA-SUPERNODE` in `ec` and `KP4DJT-HAP` in `94`.

The walker writes the detail files into a new `details-<stamp>` directory, then `map.json`, each through a temporary file and a rename, so `map.json` never names files that are not there yet. It keeps the two newest detail directories, so a page that read the previous `map.json` can still open its popups.

## AREDN package

The `meshmap` package (IPK and APK) installs:

- `/usr/bin/mesh-walker`
- MeshMap's page at `/www/meshmap/index.html`, served by the node web server at `/meshmap/` on ports 80 and 8080
- MeshMap's scripts, styles and glyphs, stored gzipped in `/usr/share/meshmap/site/` and served by `/www/cgi-bin/meshmap-site` with `Content-Encoding: gzip`. The site is built with `/cgi-bin/meshmap-site/` as its base. Files under `assets/` have content hashes in their names and are cached for a year; clients that do not accept gzip get the files unpacked.
- `/etc/meshmap/appConfig.json`, linked from `/www/meshmap/appConfig.json` and kept across firmware upgrades
- `/www/meshmap/data`, a link to `/meshmap/data`
- `/www/cgi-bin/meshmap-data`, which serves `map.json` and the detail files from their `.gz` files with `Content-Encoding: gzip` (uhttpd cannot do this for static files) at `/cgi-bin/meshmap-data/map.json` and `/cgi-bin/meshmap-data/details-<stamp>/<shard>.json`

At boot the init script mounts the ext4 file system labeled `meshmap-data` on `/meshmap` if one exists. Without that disk the walker does not run, so it never writes to the node's flash. It logs one line per hour instead.

The walker runs about one minute after start when there is no data yet, and then every hour at minute 30. Nodes with less than 256 MB of RAM fetch 16 nodes at a time instead of 64.

On install the package saves the node's map URL to `/etc/meshmap/previous-map-url` and sets it to `/meshmap/#12/(lat)/(lon)`, so the map on the node's status page opens at the node. A map URL of `/meshmap/`, set by version 0.1.0, is updated to the new form. On removal it restores the saved URL.

The default `appConfig.json` reads `map.json` through the CGI script and uses the basemap served over the mesh by `KI5VMF-KUBERNETES`, then `https://tiles.squallar.app`.

## Build

```sh
git submodule update --init
./build.sh all        # or one of: mips_24kc mipsel_24kc arm_cortex-a7_neon-vfpv4 aarch64_cortex-a53 x86_64
```

The build needs Go and Docker. Packages are written to `openwrt/`.
