#!/bin/sh
# Serve the MeshMap site's scripts, styles and glyphs gzip-compressed.
# uhttpd has no Content-Encoding support for static files, so the package
# ships these files already gzipped and this sends them as they are. Clients
# that do not accept gzip get them unpacked.
#
# The site is built with this script as its base, so index.html at /meshmap/
# loads its scripts, styles and glyphs from /cgi-bin/meshmap-site/.

SITE=/usr/share/meshmap/site

not_found() {
    printf 'Status: 404 Not Found\r\nContent-Type: text/plain\r\n\r\nNot found\n'
    exit 0
}

case "$PATH_INFO" in
    *..* | *//* | */.* | *[!A-Za-z0-9._/\ -]*) not_found ;;
    /assets/?*) cache='public, max-age=31536000, immutable' ;;
    /?*) cache='public, max-age=86400' ;;
    *) not_found ;;
esac

case "$PATH_INFO" in
    *.js) type='text/javascript' ;;
    *.css) type='text/css' ;;
    *.json) type='application/json' ;;
    *.svg) type='image/svg+xml' ;;
    *.wasm) type='application/wasm' ;;
    *.pbf) type='application/x-protobuf' ;;
    *.png) type='image/png' ;;
    *.woff2) type='font/woff2' ;;
    *.ico) type='image/x-icon' ;;
    *.txt) type='text/plain' ;;
    *) type='application/octet-stream' ;;
esac

file="$SITE$PATH_INFO"
if [ -f "$file.gz" ]; then
    printf 'Content-Type: %s\r\nCache-Control: %s\r\nVary: Accept-Encoding\r\n' "$type" "$cache"
    case "$HTTP_ACCEPT_ENCODING" in
        *gzip*)
            printf 'Content-Encoding: gzip\r\nContent-Length: %s\r\n\r\n' "$(wc -c < "$file.gz")"
            [ "$REQUEST_METHOD" = HEAD ] || cat "$file.gz"
            ;;
        *)
            printf '\r\n'
            [ "$REQUEST_METHOD" = HEAD ] || zcat "$file.gz"
            ;;
    esac
elif [ -f "$file" ]; then
    printf 'Content-Type: %s\r\nCache-Control: %s\r\nContent-Length: %s\r\n\r\n' "$type" "$cache" "$(wc -c < "$file")"
    [ "$REQUEST_METHOD" = HEAD ] || cat "$file"
else
    not_found
fi
