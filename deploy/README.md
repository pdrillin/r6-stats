# Mise en ligne

Ce guide installe r6-stats sur un serveur loué (VPS) sous Debian ou Ubuntu, derrière Caddy pour le HTTPS, avec une sauvegarde quotidienne hors du serveur. Compter une heure la première fois.

```
Internet ──HTTPS──▶ Caddy (ports 80/443, certificat automatique)
                      │
                      ▼ 127.0.0.1:8642
                   r6server (service systemd, utilisateur r6-stats)
                      │
                      ▼
                   /var/lib/r6-stats : base SQLite + replays bruts
                      │ chaque nuit
                      ▼
                   sauvegarde chiffrée (restic) hors du serveur
```

## Ce qu'il faut avant de commencer

| Quoi | Exemple | Coût indicatif |
|---|---|---|
| Un VPS : 2 vCPU, 4 Go de mémoire, 40 Go de disque, Debian 12 ou Ubuntu 24.04 | Hetzner CX22, OVH VPS, Scaleway | 4 à 8 € par mois |
| Un nom de domaine (ou un sous-domaine d'un domaine que tu as déjà) | `stats.ma-team.fr` | environ 10 € par an |
| Un espace de sauvegarde hors du serveur | Hetzner Storage Box, Backblaze B2, stockage S3 | 1 à 4 € par mois |
| L'application Discord déjà créée | celle utilisée en local | gratuit |

Le décodage d'un round demande jusqu'à 150 Mo de mémoire, quatre à la fois pendant un envoi : 4 Go laissent de la marge. 40 Go de disque suffisent pour des centaines de matchs (environ 80 Mo de replays bruts par match).

Les prix changent : vérifie-les chez l'hébergeur.

## 1. Préparer le domaine et Discord

1. Chez ton registraire, crée un enregistrement DNS **A** (et **AAAA** si le VPS a une IPv6) : `stats.ma-team.fr` → adresse IP du VPS. Attends qu'il réponde : `ping stats.ma-team.fr`.
2. Dans l'application Discord (<https://discord.com/developers/applications>), onglet **OAuth2**, ajoute la redirection `https://stats.ma-team.fr/auth/callback`. Tu peux garder celle de `127.0.0.1` pour continuer à tester en local.

## 2. Préparer le serveur

Connecté en SSH, en root (ou avec `sudo`) :

```sh
apt update && apt upgrade -y
apt install -y curl restic ufw unattended-upgrades

# Pare-feu : SSH, HTTP (pour le certificat) et HTTPS seulement.
ufw allow OpenSSH && ufw allow 80/tcp && ufw allow 443/tcp && ufw enable

# Mises à jour de sécurité automatiques.
dpkg-reconfigure -plow unattended-upgrades

# Utilisateur sans droits qui fait tourner le serveur.
useradd --system --home /var/lib/r6-stats --shell /usr/sbin/nologin r6-stats
mkdir -p /etc/r6-stats /opt/r6-stats
```

Installe Caddy depuis son dépôt officiel (instructions à jour : <https://caddyserver.com/docs/install#debian-ubuntu-raspbian>).

## 3. Construire et envoyer une version

Sur ta machine, à la racine du dépôt :

```sh
deploy/build.sh                       # produit dist/r6-stats-<version>.tar.gz
scp dist/r6-stats-*.tar.gz root@stats.ma-team.fr:/root/
```

Pour un VPS ARM (Ampere, Graviton) : `GOARCH=arm64 deploy/build.sh`.

## 4. Configurer et lancer

Sur le serveur, en root :

```sh
cd /root
tar -xzf r6-stats-<version>.tar.gz r6-stats-<version>/update.sh --strip-components=1

# Réglages et secrets : modèle dans deploy/r6-stats.env.example.
nano /etc/r6-stats/r6-stats.env
chmod 600 /etc/r6-stats/r6-stats.env
```

Copie depuis le dépôt `deploy/r6-stats.service`, `deploy/r6-stats-backup.service` et `deploy/r6-stats-backup.timer` dans `/etc/systemd/system/`, puis `deploy/Caddyfile` dans `/etc/caddy/Caddyfile` en remplaçant `stats.example.org` par ton domaine. Enfin :

```sh
systemctl daemon-reload
systemctl enable r6-stats
./update.sh r6-stats-<version>.tar.gz     # installe, démarre, vérifie
systemctl reload caddy
```

Ouvre `https://stats.ma-team.fr` : la page de connexion s'affiche. Connecte-toi, crée l'espace de ton équipe et invite tes coéquipiers.

Pense à mettre ton identifiant Discord dans `R6_CREATORS` : sinon n'importe quelle personne connectée peut créer des espaces.

## 5. Sauvegardes

Sans sauvegarde hors du serveur, une panne de disque emporte tout. À faire dès le premier jour.

```sh
nano /etc/r6-stats/backup.env          # modèle : deploy/backup.env.example
chmod 600 /etc/r6-stats/backup.env
systemctl enable --now r6-stats-backup.timer

# Une première sauvegarde tout de suite, et vérifier qu'elle est passée :
systemctl start r6-stats-backup
journalctl -u r6-stats-backup -n 20
```

Chaque nuit, la sauvegarde :

- prend une copie cohérente de la base pendant que le serveur tourne, et garde les 7 dernières sur le serveur ;
- envoie cette copie et les replays bruts, chiffrés, vers le dépôt restic ;
- conserve 14 sauvegardes quotidiennes, 8 hebdomadaires et 12 mensuelles.

**Garde le mot de passe restic ailleurs que sur le serveur** : sans lui, les sauvegardes sont illisibles.

### Tester une restauration

Une sauvegarde jamais restaurée n'est pas une sauvegarde. Une fois par mois, sur le serveur ou sur ta machine :

```sh
export RESTIC_REPOSITORY=… RESTIC_PASSWORD=…
restic snapshots --tag r6-stats
restic restore latest --tag r6-stats --target /tmp/restauration
# La base restaurée s'ouvre comme une base normale :
mkdir /tmp/essai && cp /tmp/restauration/var/lib/r6-stats/backups/r6stats-*.db /tmp/essai/r6stats.db
/opt/r6-stats/current/r6server -data /tmp/essai spaces   # liste les espaces de la sauvegarde
```

### Restaurer pour de vrai

```sh
systemctl stop r6-stats
cd /var/lib/r6-stats
mv r6stats.db r6stats.db.abime && rm -f r6stats.db-wal r6stats.db-shm
cp backups/r6stats-<date>.db r6stats.db        # ou depuis restic, comme ci-dessus
restic restore latest --tag r6-stats --target / --include /var/lib/r6-stats/raw   # replays bruts
chown -R r6-stats:r6-stats /var/lib/r6-stats
systemctl start r6-stats
```

## Mettre à jour

Sur ta machine : `git pull`, `deploy/build.sh`, puis `scp` de l'archive. Sur le serveur :

```sh
./update.sh r6-stats-<nouvelle version>.tar.gz
```

Le script sauvegarde la base (`/var/lib/r6-stats/backups/before-<version>.db`), bascule sur la nouvelle version, redémarre le service et vérifie qu'il répond. Les anciennes versions restent dans `/opt/r6-stats/releases`.

Une nouvelle version du parser re-décode au démarrage les rounds dont le replay brut est encore là : le premier démarrage après une telle mise à jour peut prendre quelques minutes.

### Revenir en arrière

```sh
systemctl stop r6-stats
ln -sfn /opt/r6-stats/releases/<version précédente> /opt/r6-stats/current
# Si la nouvelle version a fait évoluer la base, l'ancienne refuse de l'ouvrir :
cp /var/lib/r6-stats/backups/before-<nouvelle version>.db /var/lib/r6-stats/r6stats.db
rm -f /var/lib/r6-stats/r6stats.db-wal /var/lib/r6-stats/r6stats.db-shm
chown r6-stats:r6-stats /var/lib/r6-stats/r6stats.db
systemctl start r6-stats
```

Les données envoyées entre la mise à jour et le retour en arrière sont alors perdues.

## Au quotidien

| Pour… | Commande |
|---|---|
| Voir si le service tourne | `systemctl status r6-stats` |
| Lire ses journaux | `journalctl -u r6-stats -f` |
| Vérifier qu'il répond | `curl http://127.0.0.1:8642/healthz` (renvoie `ok`) |
| Lister les espaces | `runuser -u r6-stats -- /opt/r6-stats/current/r6server -data /var/lib/r6-stats spaces` |
| Lien responsable d'un espace (si plus personne ne peut le gérer) | `runuser -u r6-stats -- /opt/r6-stats/current/r6server -data /var/lib/r6-stats invite -url https://stats.ma-team.fr <id>` |
| Place occupée | `du -sh /var/lib/r6-stats/*` |
| Dernières sauvegardes | `journalctl -u r6-stats-backup -n 20` |

Pour être prévenu d'une panne, un service de surveillance gratuit (UptimeRobot, Better Stack…) peut interroger `https://stats.ma-team.fr/healthz` toutes les 5 minutes.

## Sécurité : ce qui est en place

- **HTTPS partout.** Caddy obtient et renouvelle le certificat. Le serveur refuse d'écouter hors de la machine sans adresse `https`, et ses cookies de session ne circulent qu'en HTTPS.
- **Le serveur tourne sans privilèges**, sous l'utilisateur `r6-stats`, et ne peut écrire que dans `/var/lib/r6-stats` (durcissement systemd, exposition évaluée à 1,7 sur 10 par `systemd-analyze security`).
- **Les secrets** (Discord, restic) sont dans `/etc/r6-stats`, lisibles par root seulement.
- **En-têtes de sécurité** posés par Caddy : HSTS, CSP (scripts du site seulement, avatars depuis Discord), interdiction d'affichage dans un cadre.
- **La connexion de développement** (`-dev-login`) est refusée hors de la machine locale.
- **Pas de limitation du nombre de requêtes** pour l'instant : suffisant pour une bêta fermée, à ajouter avant une ouverture publique.

## Avant d'inviter d'autres équipes

- [ ] `R6_CREATORS` contient ton identifiant Discord
- [ ] La sauvegarde de la nuit est passée, et une restauration a été testée
- [ ] Une surveillance interroge `/healthz`
- [ ] `R6_OPERATOR` et `R6_CONTACT` sont renseignés : la page `/privacy` affiche qui joindre (sans eux, elle le signale en rouge)
- [ ] Tu as relu la page `/privacy` : elle doit rester vraie (durées de conservation, hébergeur)
