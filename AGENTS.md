# AGENTS.md

Guide pour les agents de code qui travaillent sur ce dépôt. À lire avant toute modification.

## Le projet

Statistiques de match avancées pour Rainbow Six Siege, calculées à partir des fichiers de replay du jeu (`.rec`). L'objectif est d'aller plus loin que les trackers publics (K/D, winrate) en exploitant l'ordre et le moment des événements de chaque round, pour les équipes et les ligues.

Produit prévu, pas encore construit :

- **Serveur partagé** (priorité) : site privé par espace (équipe, ligue), connexion Discord, envoi des matchs par le navigateur. Il décode les `.rec` et stocke les données extraites, qui suffisent à tout afficher. Voir « Hébergement et stockage ».
- **App Windows légère** (ensuite) : surveille le dossier `MatchReplay` du jeu, garde une archive des replays sur le PC du joueur (le jeu supprime les anciens) et les envoie automatiquement à l'espace. Elle n'embarque pas l'interface : les stats se consultent sur le site. Prévue avec Wails ou en simple programme Go.
- **Interface web** : SPA React + TypeScript + Vite.

L'app Windows locale complète (interface embarquée, sans serveur) a été écartée : elle ne servirait qu'à celui qui l'installe, alors que coach et coéquipiers doivent voir les stats.

Le parsing se fait côté serveur : un changement de saison ne demande qu'une mise à jour du serveur. Aujourd'hui existent le parser Go, le regroupement en matchs, les stats, un outil en ligne de commande avec une API JSON locale (`r6rec serve`, lit un dossier de replays), l'interface web, et le serveur partagé (`r6server` : espaces, envoi des replays, base SQLite, connexion Discord, rôles, invitations) avec son interface (accueil, invitations, envoi par glisser-déposer, membres). Reste la mise en ligne.

## Utilisations visées

Deux utilisations, un seul moteur (parser, regroupement en matchs, stats). Seuls changent l'ensemble des matchs pris en compte et qui peut les voir.

| | Équipe | Ligue ou tournoi |
|---|---|---|
| Matchs | praccs et officiels de l'équipe | tous les matchs de la compétition |
| Envoyés par | un membre de l'équipe (un seul suffit : chaque replay contient les 10 joueurs) | l'organisateur ou l'observateur, qui enregistre chaque match en spectateur (format du corpus) |
| Visibles par | l'équipe | les organisateurs, et le public si la ligue publie ses matchs |
| Vue principale | « nous » contre l'adversaire : maps, sites, ouvertures, revue des rounds, préparation contre un adversaire | classements (avec un minimum de rounds), leaders par stat, pages des équipes inscrites, pages des matchs |

Dans les deux cas la couverture est complète : une équipe a tous ses matchs, une ligue tous les siens, chacun alimenté par une seule personne. C'est ce qui rend les stats fiables, et la profondeur y répond à un vrai besoin (préparer un match, classer une compétition).

**Pas de tracker public** (décision de l'utilisateur, novembre 2026) : un tracker ne vaut que par sa couverture, et des replays envoyés à la main ne couvrent que les parties des utilisateurs de l'outil. La page de la plupart des joueurs serait vide ou incomplète, là où R6 Tracker a toutes leurs parties via Ubisoft. Ne pas y revenir sauf si l'outil devient assez répandu pour que la couverture soit réelle. Cela évite aussi de publier les stats de joueurs qui n'ont rien demandé.

Modèle prévu :

- **Espace** : un ensemble de matchs, des membres, une visibilité (privé ou public). Une équipe est un espace privé qui sait laquelle des deux équipes est « nous » (à reconnaître par ses joueurs plutôt que par son nom). Une ligue est un espace à plusieurs équipes, qui peut être rendu public. Les stats d'un espace sont `stats.Compute` sur ses matchs, comme les filtres par map et par type aujourd'hui ; l'interface actuelle sert telle quelle dans un espace.
- **Joueur** : identifié par son profile id Ubisoft. Son profil n'existe qu'à l'intérieur d'un espace (ses stats dans l'équipe ou dans la ligue) ; aucune page ne le suit d'un espace à l'autre.

Confidentialité et accès :

- **Un espace équipe est privé par défaut** : seuls ses membres invités voient ses matchs et ses stats. Rien n'est visible ni indexé de l'extérieur.
- **Rôles** :
  - **responsable** (capitaine ou coach) : invite et retire des membres, étiquette les matchs (pracc ou officiel), supprime un match ;
  - **joueur** : envoie des replays et consulte tout ;
  - plus tard, **lecture seule** (analyste, remplaçant).
- **Un membre n'apparaît pas forcément dans les replays** (le coach, par exemple) : l'accès dépend d'un compte sur l'outil, pas de l'identité en jeu. À côté, chaque joueur de l'équipe est relié à son profil Ubisoft pour reconnaître « nous » dans les replays.
- **Les données de l'adversaire restent dans l'espace** : l'équipe affrontée en pracc ne voit rien, et ces matchs n'alimentent jamais une page publique. Les seules données publiques sont celles d'une ligue qui choisit de publier ses matchs officiels.
- **RGPD** (fait) : page publique `/privacy` (`web/src/pages/PrivacyPage.tsx`), exploitant et contact réglés sur le serveur (`R6_OPERATOR`, `R6_CONTACT`). Droits réels, pas seulement annoncés : export JSON (`GET /api/me/export`), suppression du compte (`DELETE /api/me`, refusée tant que l'utilisateur est seul responsable d'un espace qui a d'autres membres ; ses espaces sans autre membre partent avec lui), suppression d'un match ou d'un espace par un responsable. Un replay n'est effacé (base et fichier brut) que quand plus aucun espace ne le tient (`server/privacy.go`, `collect`). **Toute modification de ce qui est stocké ou de sa durée doit être reportée sur la page de confidentialité.**
- **Connexion** : comptes via Discord (OAuth), invitations par lien. Prévu avec le serveur partagé, conçu pour accueillir ensuite les ligues. En attendant, chaque page de l'interface doit être pensée comme appartenant à un espace.

Types de match, à ne jamais mélanger dans une même stat par défaut :

- **Ranked** (et autres parties en ligne : non classée, partie rapide) : coéquipiers souvent inconnus, pas de stratégie d'équipe. **Secondaire** : pourrait servir à un joueur de l'équipe pour suivre son jeu individuel, mais n'est pas une priorité.
- **Pracc** (scrim, entraînement) : partie personnalisée, l'équipe travaille des stratégies et peut en tester. C'est la matière principale de l'espace équipe.
- **Match officiel** (ligue, tournoi) : partie personnalisée, ce qui compte vraiment. Matière de l'espace ligue, et vue à part dans l'espace équipe.

Détection :

- Ranked ou partie personnalisée : à lire dans l'en-tête (`matchtype`, `gamemodeid`, éventuellement les `gmsetting`). Valeurs de `matchtype` vues : 3 = partie personnalisée enregistrée en spectateur (corpus), 4 = partie personnalisée enregistrée en joueur (praccs de l'utilisateur, 20 rounds max, pas de `playlistcategory`), 2 = ranked ou non classée (6 rounds + 3 de prolongation), 1 = partie rapide (4 + 1), 13 = arcade (match à mort : un seul « round », plus de 130 kills, ne pas traiter comme une partie à bombe). `matchresult` = 7 sur des parties annulées au premier round (à 0-0). Seuls 3 et 4 sont gardés (`rec.Header.CustomGame`). Un rehost n'existe qu'en partie personnalisée.
- Pracc ou officiel : même format de fichier, impossible à distinguer automatiquement. C'est une étiquette posée par l'utilisateur, modifiable par match (fait : `store.Kind`, sélecteur sur la page d'un match). Un match non étiqueté est un pracc (`store.DefaultKind`) ; dans un futur espace ligue, ce sera officiel.
- Dans l'interface : filtre de type de match (`?kind=`, fait pour pracc et officiel). Si le ranked est pris en charge un jour, il est exclu par défaut des vues d'équipe et séparé dans le profil d'un joueur.

Ordre retenu :

1. **Équipe d'abord.** C'est le format validé (parties privées, rehosts), c'est là que les stats avancées apportent le plus, l'infrastructure est minime, et l'utilisateur et son équipe sont les premiers utilisateurs.
2. **Ligue ensuite** : même format de replay (enregistrements en spectateur, comme le corpus) ; il faut surtout des vues de classement, la gestion des équipes inscrites et un accès public en lecture si la ligue le souhaite.

## Hébergement et stockage

**Hébergement et monétisation** : bêta fermée et gratuite d'abord (l'équipe de l'utilisateur, puis 2 ou 3 équipes ou une petite ligue), pour valider l'utilité avant toute monétisation. Coût visé : moins de 20 € par mois (petit VPS, domaine). Prévoir dès le départ un plan par espace (quotas d'historique et de stockage, durée de conservation des bruts) pour permettre plus tard un modèle freemium par équipe ou une offre par saison pour les ligues. Pas de publicité. Avant une ouverture publique : une marque propre (pas « R6 » ni les marques de Ubisoft), statut, CGU, RGPD.

**Les données extraites suffisent à tout afficher** : rien de ce que voit l'utilisateur ne dépend des replays bruts. Les bruts ne servent qu'à une nouvelle analyse (nouvelle information décodée, correction d'un bug de décodage) et aux contestations en ligue.

**Conservation des bruts** : tout garder pendant la bêta (peu de volume, parser qui progresse vite). Ensuite, conservation limitée (30 à 90 jours, plus longue pour les matchs officiels d'une ligue), puis suppression. L'archive locale de l'app Windows sert de filet de sécurité : elle peut renvoyer d'anciens fichiers à ré-analyser.

**Stockage** (une seule base, SQLite pour commencer, Postgres si une grosse ligue l'exige) :

- **Tables classiques** pour ce qu'on liste et filtre : espaces, membres, invitations, fichiers reçus, liens fichier ↔ espace, types de match, joueurs qui forment « nous ».
- **Un bloc par round** pour le détail : le JSON que produit le décodage (en-tête complet, propriétés brutes comprises, et round), compressé en zstd, avec la version du parser qui l'a produit (`rec.Version`). Environ 5 Ko par round. Pas de requête SQL dedans : le serveur charge les rounds d'un espace et calcule en Go. Pas de tables par événement (kills, positions…) : chaque amélioration du parser imposerait une migration.
- **Les stats ne sont jamais stockées** : calculées en mémoire à partir des rounds de l'espace (quelques millisecondes), gardées en mémoire, oubliées à chaque changement. Toute évolution d'une définition s'applique ainsi à tout l'historique.
- **Bruts** : rangés par SHA-256 (`raw/3f/a2/3fa2….rec`), sur disque puis stockage objet. Un fichier envoyé par plusieurs personnes ou les deux équipes d'un pracc n'est stocké et décodé qu'une fois ; chaque espace n'en voit que ses propres liens.
- **Conséquences quand les bruts sont supprimés** : les données extraites deviennent la seule copie, donc sauvegarde continue de la base hors du serveur et restauration testée. Une information décodée plus tard n'existera que pour les matchs envoyés après : l'interface affiche « non disponible pour ce match », jamais un zéro trompeur.
- **Nouvelle version du parser** : les rounds dont le brut existe encore sont re-décodés en tâche de fond ; les autres gardent leur analyse.

## Travailler avec l'utilisateur

- L'utilisateur écrit en français : répondre en français. Le code, les identifiants et les commentaires de code restent en anglais. La documentation (README, ce fichier) est en français.
- Le développement se fait sous Linux, les utilisateurs cibles sont sous Windows. Compilation croisée : `GOOS=windows GOARCH=amd64 go build ./cmd/r6rec`.
- Les anciennes saisons sont explicitement hors périmètre pour l'instant : viser la saison en cours (Y11S3, le corpus pro est en Y11S2) et les suivantes.

## Commandes

```sh
go vet ./...
go test ./...                        # tests unitaires + tests sur le corpus si Replays/ existe
go test -short ./...                 # saute les tests qui décompressent tout le corpus
go run ./cmd/r6rec ls Replays        # une ligne par round
go run ./cmd/r6rec matches Replays   # matchs reconstitués, round par round
go run ./cmd/r6rec stats Replays     # rapport de stats (-json pour du JSON)
go run ./cmd/r6rec rooms Replays     # kills et morts par pièce (-n N pièces par map)
go run ./cmd/r6rec split side Replays  # stats par côté (aussi : map, operator)
go run ./cmd/r6rec round <fichier.rec>  # événements décodés d'un round
go run ./cmd/r6rec header -raw <fichier.rec>
go run ./cmd/r6rec dump <fichier.rec> <sortie.bin>  # corps décompressé, pour la rétro-ingénierie
go run ./cmd/r6rec serve Replays     # API JSON sur 127.0.0.1:8642 (-addr, -web web/dist, -data fichier)
go run ./cmd/r6rec cache info        # emplacement et taille du cache (aussi : clear)

go run ./cmd/r6server create "Ma team" team     # crée un espace, affiche son id (aussi : league)
go run ./cmd/r6server import <id> Replays       # envoie tous les .rec d'un dossier dans l'espace
go run ./cmd/r6server spaces                    # liste les espaces
go run ./cmd/r6server invite <id> [manager|player]  # lien d'invitation (premier responsable d'un espace créé ici)
go run ./cmd/r6server delete <id>               # supprime un espace (demande de taper son nom)
go run ./cmd/r6server serve                     # 127.0.0.1:8642 (-addr, -url, -web web/dist, -dev-login)
go run ./cmd/r6server backup <fichier>          # copie cohérente de la base (VACUUM INTO), serveur en marche
deploy/build.sh                                 # version à installer : dist/r6-stats-<version>.tar.gz
# -data <dossier> avant la sous-commande : base et bruts (défaut r6-data)
```

Connexion : variables `R6_DISCORD_CLIENT_ID`, `R6_DISCORD_CLIENT_SECRET` (application OAuth2 Discord, redirection `<url>/auth/callback`) et `R6_CREATORS` (identifiants Discord autorisés à créer des espaces, séparés par des virgules ; vide = tout le monde). `R6_OPERATOR` et `R6_CONTACT` : exploitant et contact affichés sur la page de confidentialité. `-dev-login` permet de se connecter par un simple nom (`/auth/dev?name=…`), sans Discord : refusé hors de la machine locale. Hors de la machine locale, `serve` exige une `-url` en https. Ouvrir le site à l'adresse exacte de `-url` : les modifications venant d'une autre origine sont refusées.

Les rounds décodés sont mis en cache (dossier de cache de l'utilisateur : `~/.cache/r6-stats`, `%LocalAppData%\r6-stats` sous Windows). `R6STATS_CACHE=<dossier>` en choisit un autre, `R6STATS_CACHE=off` le coupe. Corpus : environ 5 s sans cache (décodage en parallèle), 0,2 s avec, 1,7 Mo de cache.

Interface web (Node 24, dossier `web/`) :

```sh
cd web && npm install
npm run dev     # http://localhost:5173, /api relayé vers r6rec serve (port 8642, ou R6_API)
npm run build   # vérifie les types puis produit web/dist
```

En développement, lancer `r6rec serve Replays` (ou `r6server serve -dev-login -url http://localhost:5173`, pour que la connexion revienne par Vite) d'un côté et `npm run dev` de l'autre. Pour servir l'interface construite sans Node : `r6rec serve -web web/dist Replays` ou `r6server serve -web web/dist`.

Captures du README (`docs/screenshots/`, JPEG) : `web/scripts/screenshots.mjs` (`npm run screenshots`) les refait sur un `r6server` vide lancé avec `-dev-login` : il crée un espace, envoie le corpus, suit FaZe, invite un second membre (pour que l'accueil montre le tableau de bord), puis photographie les pages. À relancer après un changement visible de l'interface.

La CI (`.github/workflows/test.yml`) lance gofmt, vet, les tests et la compilation Windows, et un job séparé construit l'interface web. `gofmt -l .` doit rester vide.

## Corpus de replays

`Replays/` est ignoré par git : environ 580 Mo de replays esport pro (EWC 2026, 15 août 2026, version du jeu `Y11S2_Alpha04`), non redistribuables. Sur une nouvelle machine, le copier à la main à la racine du dépôt. Sans lui, les tests sur le corpus sont ignorés et seuls les tests unitaires tournent.

Organisation : `Replays/<série>/<n> - <map>/Match-<date>-<id>/Match-...-RNN.rec`, un fichier par round. Deux séries :

- DarkZero vs Geekay : Nighthaven Labs 3-7, Kafe 8-7, Chalet 7-0.
- FURIA vs FaZe (grande finale) : Bank 3-7, Border 5-7, Fortress 8-6, Lair 5-7. Ces quatre scores correspondent aux résultats publics.

`match/match_test.go` fige ces scores ; `rec/rec_test.go` compare le vainqueur de chaque round au score de départ du round suivant (72/72).

## Organisation du code

| Package | Rôle |
|---|---|
| `rec/` | Format de fichier : en-tête (`rec.go`), identifiants de maps (`maps.go`), événements du corps et vainqueur du round (`round.go`) |
| `cache/` | Cache des rounds décodés, un fichier JSON par replay, clé = SHA-256 du fichier ; décodage en parallèle (6 au plus : chacun tient un corps décompressé) |
| `store/` | Données saisies par l'utilisateur (type de chaque match, « mon équipe ») : un fichier JSON réécrit en entier, de façon atomique, à chaque changement. `%AppData%\r6-stats\data.json` sous Windows, `~/.config/r6-stats/data.json` sous Linux. Distinct du cache, qu'on peut vider sans rien perdre |
| `match/` | Regroupe les rounds en matchs (parties personnalisées seulement), gère les rehosts ; `NameTeams` nomme les équipes (voir « Noms d'équipe ») ; un match enregistré par plusieurs joueurs ne compte qu'une fois (`dedupe` : les rounds du joueur qui en a le plus de complets, complétés par ceux d'un coéquipier, jamais par ceux d'un adversaire dont les noms d'équipe sont inversés) ; `Roster` complète une équipe manquante |
| `stats/` | Duels d'ouverture, trades, clutches, conversion des avantages numériques |
| `api/` | API JSON pour l'interface, celle de `r6rec serve` et celle de chaque espace du serveur (`api.Store` : fichier local ou base du serveur). Lecture : `/api/overview` (`?map=`, `?kind=` ; calculée à la demande, gardée en mémoire, oubliée quand un type de match change), `/api/matches`, `/api/matches/{id}`, `/api/settings`. Écriture : `PUT /api/matches/{id}/kind`, `PUT /api/settings`, en JSON uniquement (un autre site ne peut pas envoyer de JSON au serveur local via le navigateur sans préflight, jamais accepté) ; sert aussi l'interface construite (repli sur `index.html` pour les routes de la SPA) |
| `server/` | Serveur partagé. `auth.go` : connexion Discord (OAuth2, permission `identify` seule, jeton Discord jamais gardé, `state` lié au navigateur par un cookie), session = cookie aléatoire `HttpOnly` `SameSite=Lax` dont la base ne garde que le SHA-256, refus des modifications venant d'un autre site (`Sec-Fetch-Site`, `Origin`). `members.go` : utilisateurs, rôles (`manager` : invite, retire, change les rôles, étiquette les matchs, choisit « mon équipe » ; `player` : envoie et consulte), un espace garde toujours un responsable, invitations par lien (7 jours, plusieurs usages, révocables, seule l'empreinte est gardée). `http.go` : toutes les règles d'accès ; un non-membre reçoit 404 pour ne pas révéler l'existence de l'espace. `home.go` : page d'accueil minimale servie à `/` quand `-web` n'est pas donné (utilisateur connecté, ses espaces, connexion et déconnexion), pour ne jamais atterrir sur un 404 après la connexion Discord. Espaces, réception des replays (`Add` : décodé une fois, lié à l'espace, doublons repérés par SHA-256), base SQLite (`db.go`, migrations par `PRAGMA user_version` : ne jamais modifier une migration publiée, en ajouter une), bruts dans `raw/aa/bb/<sha256>.rec`, un bloc zstd JSON par round (en-tête complet compris), vue de chaque espace = un `api.Server` construit à la demande et oublié quand l'espace change, `Redecode` au démarrage pour les rounds d'un ancien parser. Routes : voir le commentaire de `Server.Handler` (connexion, `/api/me`, espaces, invitations, membres, envoi en multipart avec l'en-tête `X-R6-Upload: 1`, API de l'espace sous `/api/spaces/{id}/…`) |
| `cmd/r6rec/` | Outil en ligne de commande |
| `cmd/r6server/` | Le serveur partagé : `serve` (délais HTTP, arrêt propre sur SIGTERM, `/healthz`), `create`, `invite`, `delete`, `import`, `spaces`, `backup` |
| `deploy/` | Mise en ligne : guide (`README.md`), service systemd durci, Caddyfile (HTTPS, en-têtes de sécurité, CSP vérifiée sur toutes les pages), `build.sh`, `update.sh` (sauvegarde avant bascule, vérification de `/healthz`), sauvegarde quotidienne (`backup.sh`, timer, restic hors du serveur) |
| `web/` | Interface React + TypeScript + Vite. `src/api.ts` reprend à la main les types JSON de Go : un champ ajouté côté Go doit y être ajouté aussi |

Interface web :

- **Deux modes, une seule application.** Au démarrage hors espace, `Root` appelle `/api/me` : `r6rec serve` répond 404 (pas de comptes, les pages de stats tournent à la racine, `LOCAL` dans `space.ts`) ; `r6server` répond 200 ou 401 (accueil avec connexion et liste des espaces `LobbyPage`, invitation `/invite/<lien>` `InvitePage`).
- **Dans un espace** (`/s/<id>/…`), `main.tsx` lance l'application avec `/s/<id>` comme `basename` : les liens et routes des pages de stats restent identiques dans les deux modes, et changer d'espace recharge la page. `SpaceShell` vérifie la session (sinon retour à la connexion Discord) et l'appartenance, puis fournit `WhereContext` : préfixe de l'API (`spaces/<id>/`), espace, rôle, utilisateur. `useApi(path)` et `useSend()` résolvent les chemins relativement à l'espace ; un chemin qui commence par `/` est global (`/me`, `/spaces`, `/invitations/…`).
- **Navigation** (`components/Shell.tsx`) : menu latéral groupé (Équipe : Accueil, Matchs, Équipes ; Analyse : Joueurs, Pièces, Comparer ; Espace : Membres), bouton « Envoyer des replays » en tête, espace courant et compte. Sur téléphone (< 900 px), une barre en haut avec un bouton menu ouvre le même contenu en tiroir (fermé par Échap, par un clic à côté ou après un choix). Les liens gardent les filtres.
- **Accueil** (`pages/HomePage.tsx`) : en-tête (équipe, matchs, rounds, filtres), étapes « Pour bien démarrer » tant que l'espace n'a pas de matchs, d'équipe choisie ou d'autre membre (mode serveur), quatre chiffres clés, « Ce qui ressort », derniers matchs, accès aux fiches adversaires, rounds par map, joueurs. Les constats viennent de `insights.ts` : une phrase par constat, toujours avec son échantillon, jamais sous 5 cas ni sous 15 points d'écart, 5 au plus, les plus marqués d'abord ; ▲ point fort, ▼ à travailler, ● à savoir (forme et libellé accessible, pas la couleur seule).
- **Page d'un match** (`pages/MatchPage.tsx`) : en-tête (map, score en grand, date, type de match) et menu « ⋯ » pour les actions rares (suppression, responsables du serveur seulement). Trois onglets (`components/Tabs.tsx`, onglet dans l'URL `?vue=`, flèches, Début et Fin au clavier) : Résumé (carrés de rounds qui ouvrent le round dans l'onglet Rounds, comparaison des deux équipes, chronologie des kills), Rounds (dépliables, « Tout déplier »), Joueurs (tableau et situations).
- **Rôle** : `canEdit` (responsable, ou mode local) affiche le choix de l'équipe et le sélecteur de type de match ; un joueur voit le type sans pouvoir le changer, et on lui indique qu'un responsable doit choisir l'équipe.
- **Envoyer** (`/upload`, r6server) : glisser-déposer de fichiers ou de dossiers entiers (lecture récursive via `webkitGetAsEntry`), ou choix d'un dossier ; seuls les `.rec` partent, par lots de 20 fichiers ou 150 Mo, avec progression ; compte rendu ajoutés / déjà présents / écartés / illisibles. Le serveur écarte sans rien stocker tout replay qui n'est pas une partie personnalisée (`rec.Header.CustomGame`, statut `ignored`). **Membres** (`/members`) : rôles, retrait, départ, liens d'invitation (affichés une seule fois, copiables), révocation, et pour un responsable la suppression de l'espace (confirmée en tapant son nom, ancre `#supprimer`). La liste des espaces (`LobbyPage`) a un bouton « Supprimer » pour chaque espace dont on est responsable, qui mène à ce formulaire.
- Pages : « Mon équipe » (`/`, tableau de bord de l'équipe choisie : bilan, derniers matchs, adversaires, rounds par map, points forts et sites à travailler, joueurs), fiche adversaire (`/teams/{nom}/scout` : maps, choix de site en défense, attaque par site, joueurs à surveiller, opérateurs par côté, rythme, pièces si une map est choisie, situations), joueurs (`/players`), équipes, détail d'une équipe (côtés, maps, sites, situations), matchs, détail d'un match (rounds dépliables avec kill feed et opérateurs), détail d'un joueur (cartes, ventilation par côté/map/opérateur, pièces), pièces par map, comparaison de deux joueurs (`/compare?a=…&b=…`).
- Comparaison : stats ramenées au nombre de rounds de chacun (ou en pourcentage), valeur la plus favorable en gras (`lowerIsBetter` pour les morts et la première mort), barres normalisées ligne par ligne, graphiques « moment du round » sur la même échelle. Calculée côté client à partir de `/api/overview`, donc le filtre par map s'applique.
- Textes en français. Thème clair ou sombre : Auto (selon le système), Clair ou Sombre au choix (`ThemePicker`, en bas du menu et dans le pied des pages hors espace). `theme.ts` pose `data-theme` sur `<html>` avant le premier rendu (depuis `main.tsx`, la CSP interdisant les scripts en ligne) et garde le choix dans le `localStorage` ; `style.css` met les couleurs claires sous `:root[data-theme="light"]`. Pas de bibliothèque d'UI : `src/style.css` et un composant `Table` triable.
- **Style « broadcast »** (choisi en novembre 2026 parmi quatre maquettes) : fond presque noir, angles droits (`--radius`), Barlow Condensed en capitales (`--font-display`) pour les titres, scores, chiffres clés, en-têtes de tableau et le menu, Barlow (`--font`) pour le reste et les nombres des tableaux. Jaune vif (`--accent`, texte posé dessus `--on-accent`) pour la marque, le bouton d'envoi, l'onglet et le choix courants ; `--accent-text` pour le texte et les contours de focus (plus foncé en thème clair). Le bleu et l'orange restent réservés aux équipes : jamais d'accent orange. En-têtes de l'accueil et d'un match en bandeau (`.page-head`, `.match-head`). Polices embarquées via `@fontsource` (la CSP n'autorise que `'self'`, et aucune adresse de lecteur ne part chez Google).
- **Tableaux** (`components/Table.tsx`) : en-têtes triables au clavier (boutons) ; `title` d'une colonne = explication derrière un bouton ⓘ qui s'ouvre au clic ou au toucher (bulle dessinée hors du tableau, fermée par Échap ou un clic ailleurs) ; `extra: true` = colonne secondaire, masquée jusqu'à « Plus de colonnes » (choix gardé par tableau avec `id`) ; sous 700 px, les lignes deviennent des fiches (titre = première colonne affichable, `noCard` pour ne pas répéter la colonne de regroupement, « Trier par » à partir de 4 fiches). Intitulés en clair (« Premier kill », « Morts vengées »), pas d'abréviations.
- Pièces : les replays enregistrés en joueur n'en ont pas. `components/RoomCoverage.tsx` compare les morts des joueurs à celles dont la pièce est connue ; les pages Pièces, Joueur et fiche adversaire disent « non disponible » (ou « pièce connue pour X morts sur Y ») au lieu d'un tableau vide ou partiel.
- Graphiques (`src/components/charts.tsx`, SVG fait main, pas de bibliothèque) : chronologie des kills d'un match, kills et morts par moment du round (joueur, équipe), morts par pièce. Chacun a un tooltip au survol et au clavier, et ses valeurs restent lisibles sans survol (tableau à côté ou « Voir en tableau »).
- Couleurs : variables de `style.css` (`--t0`/`--t1` équipes, `--attack`/`--defense`, `--kills`/`--deaths`), validées pour le daltonisme et le contraste en clair et en sombre. Le texte n'est jamais écrit dans une couleur de série : une pastille (`tk0`, `tk1`) porte l'identité de l'équipe.
- Un pourcentage s'affiche toujours avec son échantillon (`Ratio` : « 35 % 8/23 »), grisé sous 10 cas (`SMALL_SAMPLE`). Pour les ouvertures, la fréquence (« 1er kill ») et la conversion (« Gagnés après ») sont dans deux colonnes séparées.
- Filtres map et type de match : dans l'URL (`?map=Bank&kind=official`), lus par `useMapFilter` et `useKindFilter`. Il s'applique à toutes les pages de stats, suit la navigation (`FLink`, liens du menu) et se partage avec le lien. La page Pièces n'a pas de vue « toutes les maps » : ses onglets règlent le filtre. Les réponses de l'API sont mises en cache côté client ; pendant un rechargement, l'ancien rendu reste affiché, grisé.
- « Mon équipe » est enregistrée par le serveur (`/api/settings`). Un choix gardé dans le `localStorage` par une version précédente est envoyé une fois au serveur puis effacé (`src/team.ts`). Après une écriture, `put` vide le cache client et les pages chargent à nouveau. Le côté de chaque opérateur vient de `Overview.operatorSides` (un opérateur ne joue qu'un côté). En défense, c'est l'équipe qui choisit le site : la part de chaque site en défense mesure ses préférences.
- Identifiant d'un match : `id` d'en-tête du premier enregistrement. Identifiant d'un joueur : `Player.ID` (profile id, ou `name:` + pseudo).

## Format de fichier (rétro-ingénierie, Y11S2)

Entiers en little-endian sauf mention contraire.

**Conteneur**
- Signature `dissect\0`, puis un préambule (u32, chaîne, u32, u32), puis un u64 donnant le nombre de propriétés et autant de paires (clé, valeur) en chaînes. Une chaîne = longueur u64 + octets.
- Ensuite une zone d'index non décodée, puis les blocs : balise `200VRPMC`, u32 taille décompressée, u32 taille compressée, une trame zstd. Les blocs concaténés forment le corps (40 à 90 Mo par round).

**Propriétés de l'en-tête** (les clés se répètent, l'ordre compte)
- Les blocs joueur commencent à `playerid` : `profileid`, `playername`, `team` (indice d'équipe), `heroname`, `alliance`, `roleimage`, `rolename` (nom affiché de l'opérateur), `roleportrait`.
- `alliance` : 3 = attaque, 4 = défense (vérifié avec les opérateurs choisis sur les deux mi-temps).
- Match : `version`, `code`, `datetime`, `id` (identifiant de match, change à chaque rehost), `matchtype` (voir « Détection »), `worldid` (identifiant de map, voir `rec/maps.go`), `gamemodeid`, `roundnumber` (à partir de 0), `teamname0/1`, `startingteamscore0/1` (score *avant* le round), `teamscore0/1` (score *après* le round, `Team.Score`), `matchresult`, `recordingprofileid`, `maxnbplayersperteam`, de nombreux `gmsetting`.
- Le site de bombe n'est pas dans l'en-tête (il est dans le corps, voir plus bas).
- Replays de tournoi (spectateur) : `profileid` à zéro, `isspectator` = 1, vrais noms d'équipe.
- **Replays enregistrés en joueur** (Y11S3, dossier de l'utilisateur) : `profileid` remplis, `recordingprofileid` = le joueur qui enregistre, équipes toujours nommées « YOUR TEAM » (la sienne) et « ENEMY TEAM », dans un ordre quelconque : l'indice 0 n'est pas forcément « YOUR TEAM ». Les pseudos portent souvent le tag du clan (`Pepsi.RCK`, `Finn.OGLA`). Deux joueurs qui enregistrent le même match produisent des fichiers différents, avec des noms d'équipe inversés s'ils sont adverses.

**Corps**
- Un flux de mises à jour d'objets : `0x23` + identifiant d'objet sur 8 octets, puis des propriétés. Chaque propriété = clé de 4 octets (un hash, lu en big-endian dans le code), longueur sur 1 octet, valeur. Les propriétés après la première sont préfixées par `0x22`, ou par `0x26` quand un indice de tableau sur 4 octets précède la longueur.
- Objet d'état du round (repéré par sa première clé `0x1f07efc9`) : `0x1f07efc9` chrono en secondes, `0x1837466c` chrono en ms, `0x35f88fcf` drapeau d'alerte (pose, ou 10 dernières secondes), `0xbb094fd3` état du round (3 = terminé).
- Entrée du kill feed (première clé `0x5934e58b`) : `0xd9133cba` tueur, `0xac190f70` victime, `0xc198de70` / `0x782e7b50` équipe du tueur / de la victime + 1, `0xc35ba44e` headshot. Ces équipes suivent l'en-tête dans un replay en spectateur, mais celui qui enregistre en joueur a toujours l'équipe 0 : le parser les ramène aux indices de l'en-tête d'après les joueurs nommés (`alignFeedTeams`). Les entrées sont renvoyées à chaque redessin du feed : dédoublonner par identifiant d'objet. Les entrées sans victime sont des annonces de phase.
- Objets « étiquette » (clé `0xaf9899ca`), présents dans l'état initial du corps :
  - **Site de bombe** : valeur non indexée de la forme `"2F Master Bedroom<br/>2F Office"`, avec `0x2ca42af6` = 1. Un second objet peut porter un ancien site avec `0x2ca42af6` = 2 (par exemple le dernier site de la map précédente) : l'ignorer. L'étiquette n'est jamais mise à jour pendant le round. Vérifié sur les 84 fichiers : une seule étiquette active par round.
  - **Points d'apparition** : valeur non indexée (`"Campfire"`, `"Lakeside"`) suivie de `0x76fee40a` = 4. **Non fiable** : souvent un reste d'un round précédent, présent aussi chez les défenseurs. Non utilisé.
  - **Positions des joueurs** : `0xaf9899ca` = `ffffffffffffffff`, puis l'élément indexé `[0]` = nom de la pièce (vide si mort ou pas encore déployé), `[1]` = un octet.
- **Rattachement des positions aux joueurs** (`rec/position.go`) : dans l'état initial, chaque joueur a un bloc qui commence par son nom (clé `0x07949bdc`) et se termine au nom suivant. Le bloc contient deux objets de position : le **premier suit la vue** (drone en préparation, caméras), le **second suit le corps**. Validé sur le corpus : le corps passe à vide au moment de la mort annoncée par le kill feed (532 cas sur 556 ; les autres ne sont pas remis à zéro mais la dernière pièce connue reste juste), et les défenseurs démarrent sur leur site dans 74 rounds sur 79. **Absent des replays enregistrés en joueur** (Y11S3) : les objets de position existent mais restent vides, aucun nom de pièce n'est envoyé (seule l'étiquette du site existe). Pas de positions ni de pièce des kills pour ces matchs ; les stats par pièce les ignorent. Reste à savoir si un replay Y11S3 en spectateur les a encore.
- **Tableau des scores** : un objet par joueur, envoyé juste avant l'objet qui porte son nom dans l'état initial. Compteurs cumulés sur la session d'enregistrement (u32) : `0x1cd2b19d` kills, `0xcd9c5d72` morts, `0x4d737f9e` assists, `0x31a3d4d2` poses de défuseur, `0xecda4f80` score, `0x8310f4f4` jamais mis à jour dans le corpus. Aucun compteur de désamorçages trouvé.
- **Gains par round** (`Round.Scoreboards`) : dernière valeur vue moins la première, pour chaque compteur. Les morts concordent avec le kill feed pour les 800 joueur-rounds du corpus et les kills par équipe pour tous les rounds. Les kills par joueur diffèrent dans 24 cas : joueur mis à terre par l'un et achevé par l'autre, le feed nomme celui qui achève, le tableau des scores peut créditer le kill à l'autre et l'assist à celui qui achève. Le parser prend les kills du feed et les assists du tableau des scores.
- **Poseur** : le joueur dont le compteur `0x31a3d4d2` augmente pendant le round (la première valeur vue sert de référence). Les 21 poses du corpus sont attribuées à un attaquant. Quand le round se termine dans le même tick que la pose (Lair, round 2), le chrono ne passe jamais au compte à rebours : la pose est alors datée par ce compteur.
- **Désamorceur** : aucun compteur dédié, mais le désamorçage rapporte des points. Au dernier tick du round, chaque vainqueur gagne le même bonus (100, ou bien plus en fin de match) et le désamorceur gagne exactement 100 de plus (`defuseBonus`). On compare les gains de score (`0xecda4f80`) des défenseurs entre le début du dernier tick et la fin de l'enregistrement ; sans écart d'exactement 100, personne n'est nommé. Les 7 désamorçages du corpus sont attribués ; 6 désamorceurs sont dans la pièce même de la pose, le 7e sur le seuil (le défuseur peut être posé en bordure de site, par exemple en 2F Mask Room sur Lair).
- La pièce d'un kill est la dernière pièce non vide du corps avant le kill, en comptant les mises à jour du même tick de chrono (elles arrivent parfois juste après l'entrée du feed). Connue pour les 555 kills du corpus.
- Les noms de pièces peuvent différer légèrement des étiquettes de site (`"1F  Kitchen Cooking"` avec deux espaces sur Kafe) : le parser normalise les espaces.
- Sur Kafe, les défenseurs du site Fireplace Hall / Mining Room démarrent dans le Train Museum voisin.
- Un événement prend l'heure de la dernière mise à jour du chrono qui le précède dans le corps (le corps est chronologique).

**Version du décodeur** : `rec.Version`. L'incrémenter à chaque modification qui change ce que `Header` ou `Round` contiennent pour un fichier, sinon le cache resservira d'anciens résultats. `Header.Properties` n'est pas mis en cache.

**Pièges connus**
- Le vainqueur du round est déduit des règles du jeu (élimination, pose + temps écoulé, pose sans temps écoulé = désamorçage, temps écoulé). Quand l'en-tête donne `teamscore0/1` et qu'un seul score a gagné un point, c'est lui qui fait foi (forfait, enregistrement coupé), sans raison de victoire. Vérifié sur les replays de l'utilisateur : 145 rounds sur 145.
- Une pose est détectée par le saut du chrono vers le haut (compte à rebours du défuseur), par le drapeau d'alerte avec plus de 11,5 s restantes, ou à défaut par le compteur de poses du tableau des scores.
- Quand une pose en cours prolonge le round au-delà de 0:00, le chrono se fige sous 1 s : les événements tardifs s'affichent à 2:59.
- Un enregistrement fait juste après un rehost commence dans l'état « terminé », puis en sort.
- Rehosts : même map et mêmes équipes ; un numéro de round répété remplace l'enregistrement précédent (incomplet). Certains fichiers d'après rehost ne listent qu'une équipe dans l'en-tête ; `match.Roster` reprend l'autre équipe d'un round voisin.
- Les clés et la structure changeront d'une saison à l'autre. Plan : aiguiller selon `version`/`code`, garder un corpus de test par saison, signaler les matchs qui échouent aux contrôles de cohérence plutôt que d'afficher des stats fausses.

## Noms d'équipe (`match/naming.go`)

Les replays enregistrés en joueur appellent les équipes « YOUR TEAM » et « ENEMY TEAM » : sans traitement, tous les adversaires seraient une seule équipe. `NameTeams`, appliqué par l'API avant tout calcul :

- **Reconnaître** : deux équipes de deux matchs qui ont au moins `SameTeam` (3) joueurs en commun (profile id, `PlayerID`) sont la même. Les noms écrits par un replay spectateur (vrais noms) regroupent par nom, sans regarder les joueurs.
- **Nommer**, dans l'ordre : un nom donné par un responsable (`Alias` : nom + joueurs, appliqué à toute équipe qui en partage 3, y compris dans les matchs futurs) ; le nom du replay s'il n'est pas générique ; le tag de clan évident (au moins 3 pseudos `xxx.TAG` avec le même tag, sans égalité) ; sinon le nom générique et le joueur le plus présent (« ENEMY TEAM · salva.Alone »). `Overview.teamNamedBy` dit d'où vient chaque nom.
- **Renommer** : `PUT /api/teams/name` `{"team": nom actuel, "name": nouveau}` (responsables), nom vide = retour au nom automatique. « Mon équipe » suit le renommage. Stocké dans `data.json` (`teamNames`) ou la colonne `spaces.team_names` (JSON). Interface : page d'une équipe (`components/TeamName.tsx`).
- Limite : un adversaire qui change 3 joueurs d'un match à l'autre devient une autre équipe.

## Définitions des stats (`stats/`)

- Ouverture : premier kill du round. Un team kill en premier compte comme première mort pour cette équipe et comme premier kill pour personne.
- Trade : le tueur meurt, tué par l'équipe de la victime, dans les `TradeWindowMs` (5000 par défaut). Un kill qui venge plusieurs morts compte pour un seul kill de trade.
- Clutch : joueur resté seul dans son équipe ; taille = nombre d'ennemis en vie à ce moment ; gagné si l'équipe gagne le round.
- Situations : chaque état NvM compté une fois par round, avec le nombre de rounds gagnés.
- KOST : rounds où le joueur a fait un kill (ennemi), posé ou désamorcé le défuseur, survécu ou vu sa mort tradée. `Survived` = rounds vivant à la fin.
- Ventilations : `BySide`, `ByMap`, `ByOperator` (joueurs) et `BySide`, `ByMap` (équipes). Calculées par différence des totaux avant et après chaque round, donc toujours cohérentes avec eux : un nouveau compteur doit être ajouté à `Line`/`TeamLine` pour apparaître dans les ventilations. L'opérateur vient de l'en-tête ; il manque pour l'équipe absente d'un en-tête à une seule équipe (ces rounds sont exclus de `ByOperator`).
- Moment du round (`KillTiming`, `DeathTiming`) : tranche 0 = préparation, tranche i = [(i-1)×30 s, i×30 s) de la phase d'action, la dernière tout ce qui suit 3:00 (seule une pose y mène). Les sommes égalent `Kills` et `Deaths`, vérifié sur le corpus.
- Pièces : kills comptés dans la pièce du tueur, morts dans celle de la victime, séparés par côté (`Report.Rooms`), et par joueur (`KillRooms`, `DeathRooms`).
- Sites : par équipe, map et site, rounds joués et gagnés en attaque et en défense, et rounds d'attaque avec pose (et gagnés). Les rounds sans site connu ne comptent pas dans cette ventilation.
- Les joueurs sont identifiés par leur profile id quand il existe, par leur nom sinon. Les rounds sans vainqueur connu sont écartés et comptés dans `SkippedRounds`.

## Prochaines étapes

1. **Replays de l'équipe de l'utilisateur** (examinés en novembre 2026, Y11S3 : 4 praccs et beaucoup de ranked, voir « Format de fichier »). Restent :
   - fait : nommer l'adversaire (voir « Noms d'équipe ») ;
   - reconnaître « nous » par les profile ids des joueurs de l'équipe ;
   - faits : seules les parties personnalisées sont gardées, et un match envoyé par plusieurs membres ne compte qu'une fois (`match.dedupe`, testé sur des rounds synthétiques faute de deux enregistrements réels du même match).
2. **Fait** : cache des rounds décodés et stockage local (`r6rec serve`), qui servent de base au serveur.
3. **Vues « équipe »** (faits : accueil d'équipe, fiche adversaire, type pracc ou officiel et son filtre) : à compléter avec la reconnaissance de « nous » par les joueurs plutôt que par le nom, la revue d'un round (positions au fil du temps), et un minimum de rounds sur la page Joueurs.
4. **Serveur partagé**, avant l'app Windows (voir « Hébergement et stockage ») :
   1. **fait** : cœur testable en local (`server/`, `cmd/r6server`) : base SQLite, stockage des bruts, envoi et décodage des matchs, espaces, API propre à chaque espace. Corpus : 84 rounds en 12 s, base de 508 Ko contre 578 Mo de bruts, stats identiques à `r6rec` ;
   2. **fait** : connexion Discord, sessions, rôles, invitations, contrôle d'accès, bêta fermée (`R6_CREATORS`), connexion de développement. Testé avec un faux Discord ; reste à l'essayer avec une vraie application Discord ;
   3. **fait** : interface : connexion, liste et création des espaces, invitations, envoi par glisser-déposer (dossiers compris), membres et rôles ; les pages de stats tournent dans l'espace ;
   4. mise en ligne : configuration prête dans `deploy/` (VPS Debian ou Ubuntu, Caddy, systemd, restic). Reste à l'utilisateur de choisir l'hébergeur, le domaine et l'espace de sauvegarde, puis de suivre `deploy/README.md`. Non testé sur un vrai VPS.
5. **App Windows légère** : surveillance de `MatchReplay`, archive locale, envoi automatique à l'espace.
6. **Ligue** : espace à plusieurs équipes, classements, publication facultative.

Le filtre par période attend d'avoir plusieurs semaines de parties. Pas de tracker public (voir « Utilisations visées »).

Toujours afficher la taille de l'échantillon à côté d'un pourcentage : les clutches et les situations rares reposent sur peu de cas.
