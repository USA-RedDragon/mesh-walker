#!/bin/sh
# Serve the walker's map.json and node detail files gzip-compressed.
# uhttpd has no Content-Encoding support for static files, so this sends the
# .gz files the walker writes as they are, and unpacks them for clients that
# do not accept gzip.
#
#   /cgi-bin/meshmap-data/map.json
#   /cgi-bin/meshmap-data/details-<stamp>/<shard>.json

DATA=/meshmap/data

not_found() {
    printf 'Status: 404 Not Found\r\nContent-Type: text/plain\r\nAccess-Control-Allow-Origin: *\r\n\r\nNot found\n'
    exit 0
}

case "$PATH_INFO" in
    /map.json)
        cache='no-cache'
        ;;
    /details-[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]T[0-9][0-9][0-9][0-9][0-9][0-9]Z/[0-9a-f][0-9a-f].json)
        cache='public, max-age=31536000, immutable'
        ;;
    *)
        not_found
        ;;
esac

file="$DATA$PATH_INFO.gz"
[ -f "$file" ] || not_found

printf 'Content-Type: application/json\r\nCache-Control: %s\r\nAccess-Control-Allow-Origin: *\r\nVary: Accept-Encoding\r\n' "$cache"
case "$HTTP_ACCEPT_ENCODING" in
    *gzip*)
        printf 'Content-Encoding: gzip\r\nContent-Length: %s\r\n\r\n' "$(wc -c < "$file")"
        [ "$REQUEST_METHOD" = HEAD ] || cat "$file"
        ;;
    *)
        printf '\r\n'
        [ "$REQUEST_METHOD" = HEAD ] || zcat "$file"
        ;;
esac
