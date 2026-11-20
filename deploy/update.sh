#!/bin/sh
# Installe une version construite par build.sh, sur le serveur, en root :
#   ./update.sh stratdata-<version>.tar.gz
# La base est sauvegardée juste avant : une nouvelle version peut faire
# évoluer son schéma, et l'ancienne refuserait ensuite de l'ouvrir.
# Revenir en arrière : voir deploy/README.md.
set -eu
tarball=$1
name=$(basename "$tarball" .tar.gz)
releases=/opt/stratdata/releases
data=/var/lib/stratdata

mkdir -p "$releases"
tar -C "$releases" -xzf "$tarball"
chown -R root:root "$releases/$name"

if [ -x /opt/stratdata/current/stratdata ] && [ -f "$data/stratdata.db" ]; then
	mkdir -p "$data/backups"
	chown stratdata:stratdata "$data/backups"
	runuser -u stratdata -- /opt/stratdata/current/stratdata -data "$data" \
		backup "$data/backups/before-$name.db"
	echo "base sauvegardée : $data/backups/before-$name.db"
fi

ln -sfn "$releases/$name" /opt/stratdata/current
systemctl restart stratdata

# Le service doit répondre dans les secondes qui suivent.
for _ in 1 2 3 4 5 6 7 8 9 10; do
	if curl -fsS http://127.0.0.1:8642/healthz >/dev/null 2>&1; then
		echo "$name en service"
		exit 0
	fi
	sleep 1
done
echo "le service ne répond pas : journalctl -u stratdata -n 50" >&2
exit 1
