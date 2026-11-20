# Mise en ligne

Ce guide installe stratdata sur un serveur loué (VPS) sous Debian ou Ubuntu, derrière Caddy pour le HTTPS, avec une sauvegarde quotidienne hors du serveur. Compter une heure la première fois.

```
Internet ──HTTPS──▶ Caddy (ports 80/443, certificat automatique)
                      │
                      ▼ 127.0.0.1:8642
                   stratdata (service systemd, utilisateur stratdata)
                      │
                      ▼
                   /var/lib/stratdata : base SQLite + replays bruts
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
useradd --system --home /var/lib/stratdata --shell /usr/sbin/nologin stratdata
mkdir -p /etc/stratdata /opt/stratdata
```

Installe Caddy depuis son dépôt officiel (instructions à jour : <https://caddyserver.com/docs/install#debian-ubuntu-raspbian>).

## 3. Construire et envoyer une version

Sur ta machine, à la racine du dépôt :

```sh
deploy/build.sh                       # produit dist/stratdata-<version>.tar.gz
scp dist/stratdata-*.tar.gz root@stats.ma-team.fr:/root/
```

Pour un VPS ARM (Ampere, Graviton) : `GOARCH=arm64 deploy/build.sh`.

## 4. Configurer et lancer

Sur le serveur, en root :

```sh
cd /root
tar -xzf stratdata-<version>.tar.gz stratdata-<version>/update.sh --strip-components=1

# Réglages et secrets : modèle dans deploy/stratdata.env.example.
nano /etc/stratdata/stratdata.env
chmod 600 /etc/stratdata/stratdata.env
```

Copie depuis le dépôt `deploy/stratdata.service`, `deploy/stratdata-backup.service` et `deploy/stratdata-backup.timer` dans `/etc/systemd/system/`, puis `deploy/Caddyfile` dans `/etc/caddy/Caddyfile` en remplaçant `stats.example.org` par ton domaine. Enfin :

```sh
systemctl daemon-reload
systemctl enable stratdata
./update.sh stratdata-<version>.tar.gz     # installe, démarre, vérifie
systemctl reload caddy
```

Ouvre `https://stats.ma-team.fr` : la page de connexion s'affiche. Connecte-toi, crée l'espace de ton équipe et invite tes coéquipiers.

Pense à mettre ton identifiant Discord dans `STRATDATA_CREATORS` : sinon n'importe quelle personne connectée peut créer des espaces.

## 5. Sauvegardes

Sans sauvegarde hors du serveur, une panne de disque emporte tout. À faire dès le premier jour.

```sh
nano /etc/stratdata/backup.env          # modèle : deploy/backup.env.example
chmod 600 /etc/stratdata/backup.env
systemctl enable --now stratdata-backup.timer

# Une première sauvegarde tout de suite, et vérifier qu'elle est passée :
systemctl start stratdata-backup
journalctl -u stratdata-backup -n 20
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
restic snapshots --tag stratdata
restic restore latest --tag stratdata --target /tmp/restauration
# La base restaurée s'ouvre comme une base normale :
mkdir /tmp/essai && cp /tmp/restauration/var/lib/stratdata/backups/stratdata-*.db /tmp/essai/stratdata.db
/opt/stratdata/current/stratdata -data /tmp/essai spaces   # liste les espaces de la sauvegarde
```

### Restaurer pour de vrai

```sh
systemctl stop stratdata
cd /var/lib/stratdata
mv stratdata.db stratdata.db.abime && rm -f stratdata.db-wal stratdata.db-shm
cp backups/stratdata-<date>.db stratdata.db        # ou depuis restic, comme ci-dessus
restic restore latest --tag stratdata --target / --include /var/lib/stratdata/raw   # replays bruts
chown -R stratdata:stratdata /var/lib/stratdata
systemctl start stratdata
```

## Mettre à jour

Sur ta machine : `git pull`, `deploy/build.sh`, puis `scp` de l'archive. Sur le serveur :

```sh
./update.sh stratdata-<nouvelle version>.tar.gz
```

Le script sauvegarde la base (`/var/lib/stratdata/backups/before-<version>.db`), bascule sur la nouvelle version, redémarre le service et vérifie qu'il répond. Les anciennes versions restent dans `/opt/stratdata/releases`.

Une nouvelle version du parser re-décode au démarrage les rounds dont le replay brut est encore là : le premier démarrage après une telle mise à jour peut prendre quelques minutes.

### Revenir en arrière

```sh
systemctl stop stratdata
ln -sfn /opt/stratdata/releases/<version précédente> /opt/stratdata/current
# Si la nouvelle version a fait évoluer la base, l'ancienne refuse de l'ouvrir :
cp /var/lib/stratdata/backups/before-<nouvelle version>.db /var/lib/stratdata/stratdata.db
rm -f /var/lib/stratdata/stratdata.db-wal /var/lib/stratdata/stratdata.db-shm
chown stratdata:stratdata /var/lib/stratdata/stratdata.db
systemctl start stratdata
```

Les données envoyées entre la mise à jour et le retour en arrière sont alors perdues.

## Au quotidien

| Pour… | Commande |
|---|---|
| Voir si le service tourne | `systemctl status stratdata` |
| Lire ses journaux | `journalctl -u stratdata -f` |
| Vérifier qu'il répond | `curl http://127.0.0.1:8642/healthz` (renvoie `ok`) |
| Lister les espaces | `runuser -u stratdata -- /opt/stratdata/current/stratdata -data /var/lib/stratdata spaces` |
| Lien responsable d'un espace (si plus personne ne peut le gérer) | `runuser -u stratdata -- /opt/stratdata/current/stratdata -data /var/lib/stratdata invite -url https://stats.ma-team.fr <id>` |
| Place occupée | `du -sh /var/lib/stratdata/*` |
| Dernières sauvegardes | `journalctl -u stratdata-backup -n 20` |

Pour être prévenu d'une panne, un service de surveillance gratuit (UptimeRobot, Better Stack…) peut interroger `https://stats.ma-team.fr/healthz` toutes les 5 minutes.

## Sécurité : ce qui est en place

- **HTTPS partout.** Caddy obtient et renouvelle le certificat. Le serveur refuse d'écouter hors de la machine sans adresse `https`, et ses cookies de session ne circulent qu'en HTTPS.
- **Le serveur tourne sans privilèges**, sous l'utilisateur `stratdata`, et ne peut écrire que dans `/var/lib/stratdata` (durcissement systemd, exposition évaluée à 1,7 sur 10 par `systemd-analyze security`).
- **Les secrets** (Discord, restic) sont dans `/etc/stratdata`, lisibles par root seulement.
- **En-têtes de sécurité** posés par Caddy : HSTS, CSP (scripts du site seulement, avatars depuis Discord), interdiction d'affichage dans un cadre.
- **La connexion de développement** (`-dev-login`) est refusée hors de la machine locale.
- **Pas de limitation du nombre de requêtes** pour l'instant : suffisant pour une bêta fermée, à ajouter avant une ouverture publique.

## Avant d'inviter d'autres équipes

- [ ] `STRATDATA_CREATORS` contient ton identifiant Discord
- [ ] La sauvegarde de la nuit est passée, et une restauration a été testée
- [ ] Une surveillance interroge `/healthz`
- [ ] `STRATDATA_OPERATOR` et `STRATDATA_CONTACT` sont renseignés : la page `/privacy` affiche qui joindre (sans eux, elle le signale en rouge)
- [ ] Tu as relu la page `/privacy` : elle doit rester vraie (durées de conservation, hébergeur)
