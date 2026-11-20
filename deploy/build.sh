#!/bin/sh
# Construit une version à installer sur le serveur : le programme pour Linux
# et l'interface web, dans dist/stratdata-<version>.tar.gz.
# À lancer sur ta machine, à la racine du dépôt ou d'ici.
#   GOARCH=arm64 deploy/build.sh   pour un serveur ARM
set -eu
cd "$(dirname "$0")/.."

version=$(git describe --tags --always --dirty)
name="stratdata-$version"
out="dist/$name"
rm -rf "$out" "$out.tar.gz"
mkdir -p "$out"

(cd web && npm ci --no-audit --no-fund && npm run build)
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-amd64}" \
	go build -trimpath -ldflags "-s -w" -o "$out/stratdata" ./cmd/stratdata
cp -r web/dist "$out/web"
cp deploy/backup.sh deploy/update.sh "$out/"

tar -C dist -czf "$out.tar.gz" "$name"
echo "$out.tar.gz"
