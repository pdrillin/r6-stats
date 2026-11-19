#!/bin/sh
# Sauvegarde quotidienne, lancée par r6-stats-backup.timer sous l'utilisateur
# r6-stats :
# 1. une copie cohérente de la base, prise pendant que le serveur tourne ;
# 2. si RESTIC_REPOSITORY est réglé, l'envoi chiffré de cette copie et des
#    replays bruts hors du serveur, avec une durée de conservation.
set -eu
data=/var/lib/r6-stats
dest="$data/backups"
mkdir -p "$dest"
stamp=$(date -u +%Y%m%d-%H%M%S)

/opt/r6-stats/current/r6server -data "$data" backup "$dest/r6stats-$stamp.db"

# Les 7 dernières copies restent sur le serveur, pour une restauration rapide.
ls -1t "$dest"/r6stats-*.db | tail -n +8 | xargs -r rm --

if [ -z "${RESTIC_REPOSITORY:-}" ]; then
	echo "RESTIC_REPOSITORY n'est pas réglé : les sauvegardes restent sur ce serveur" >&2
	exit 0
fi
restic snapshots >/dev/null 2>&1 || restic init
restic backup --tag r6-stats "$dest/r6stats-$stamp.db" "$data/raw"
restic forget --tag r6-stats --keep-daily 14 --keep-weekly 8 --keep-monthly 12 --prune
