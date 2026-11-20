#!/bin/sh
# Sauvegarde quotidienne, lancée par stratdata-backup.timer sous l'utilisateur
# stratdata :
# 1. une copie cohérente de la base, prise pendant que le serveur tourne ;
# 2. si RESTIC_REPOSITORY est réglé, l'envoi chiffré de cette copie et des
#    replays bruts hors du serveur, avec une durée de conservation.
set -eu
data=/var/lib/stratdata
dest="$data/backups"
mkdir -p "$dest"
stamp=$(date -u +%Y%m%d-%H%M%S)

/opt/stratdata/current/stratdata -data "$data" backup "$dest/stratdata-$stamp.db"

# Les 7 dernières copies restent sur le serveur, pour une restauration rapide.
ls -1t "$dest"/stratdata-*.db | tail -n +8 | xargs -r rm --

if [ -z "${RESTIC_REPOSITORY:-}" ]; then
	echo "RESTIC_REPOSITORY n'est pas réglé : les sauvegardes restent sur ce serveur" >&2
	exit 0
fi
restic snapshots >/dev/null 2>&1 || restic init
restic backup --tag stratdata "$dest/stratdata-$stamp.db" "$data/raw"
restic forget --tag stratdata --keep-daily 14 --keep-weekly 8 --keep-monthly 12 --prune
