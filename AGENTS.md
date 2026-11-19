# AGENTS.md

Guide pour les agents de code qui travaillent sur ce dépôt. À lire avant toute modification.

## Le projet

Statistiques de match avancées pour Rainbow Six Siege, calculées à partir des fichiers de replay du jeu (`.rec`). L'objectif est d'aller plus loin que les trackers publics (K/D, winrate) en exploitant l'ordre et le moment des événements de chaque round.

Produit prévu, pas encore construit :

- **App Windows** (les joueurs sont sous Windows) : surveille le dossier `MatchReplay` du jeu et envoie les nouveaux matchs avant que le jeu ne les supprime. Prévue avec Wails (backend Go + interface web dans WebView2).
- **Serveur** : reçoit les `.rec` bruts, les parse, stocke les stats (Postgres) et conserve les fichiers bruts (stockage objet) pour pouvoir les re-parser quand le parser s'améliore.
- **Interface web** : SPA React + TypeScript + Vite, partagée entre le site et l'app Windows.

Le parsing se fait côté serveur : un changement de saison ne demande qu'une mise à jour du serveur. Aujourd'hui existent le parser Go, le regroupement en matchs, les stats et un outil en ligne de commande.

## Travailler avec l'utilisateur

- L'utilisateur écrit en français : répondre en français. Le code, les identifiants et les commentaires de code restent en anglais. La documentation (README, ce fichier) est en français.
- Le développement se fait sous Linux, les utilisateurs cibles sont sous Windows. Compilation croisée : `GOOS=windows GOARCH=amd64 go build ./cmd/r6rec`.
- Les anciennes saisons sont explicitement hors périmètre pour l'instant : viser la saison en cours (Y11S2) et les suivantes.

## Commandes

```sh
go vet ./...
go test ./...                        # tests unitaires + tests sur le corpus si Replays/ existe
go test -short ./...                 # saute les tests qui décompressent tout le corpus
go run ./cmd/r6rec ls Replays        # une ligne par round
go run ./cmd/r6rec matches Replays   # matchs reconstitués, round par round
go run ./cmd/r6rec stats Replays     # rapport de stats (-json pour du JSON)
go run ./cmd/r6rec rooms Replays     # kills et morts par pièce (-n N pièces par map)
go run ./cmd/r6rec round <fichier.rec>  # événements décodés d'un round
go run ./cmd/r6rec header -raw <fichier.rec>
go run ./cmd/r6rec dump <fichier.rec> <sortie.bin>  # corps décompressé, pour la rétro-ingénierie
```

La CI (`.github/workflows/test.yml`) lance gofmt, vet, les tests et la compilation Windows. `gofmt -l .` doit rester vide.

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
| `match/` | Regroupe les rounds en matchs, gère les rehosts ; `Roster` complète une équipe manquante |
| `stats/` | Duels d'ouverture, trades, clutches, conversion des avantages numériques |
| `cmd/r6rec/` | Outil en ligne de commande |

## Format de fichier (rétro-ingénierie, Y11S2)

Entiers en little-endian sauf mention contraire.

**Conteneur**
- Signature `dissect\0`, puis un préambule (u32, chaîne, u32, u32), puis un u64 donnant le nombre de propriétés et autant de paires (clé, valeur) en chaînes. Une chaîne = longueur u64 + octets.
- Ensuite une zone d'index non décodée, puis les blocs : balise `200VRPMC`, u32 taille décompressée, u32 taille compressée, une trame zstd. Les blocs concaténés forment le corps (40 à 90 Mo par round).

**Propriétés de l'en-tête** (les clés se répètent, l'ordre compte)
- Les blocs joueur commencent à `playerid` : `profileid`, `playername`, `team` (indice d'équipe), `heroname`, `alliance`, `roleimage`, `rolename` (nom affiché de l'opérateur), `roleportrait`.
- `alliance` : 3 = attaque, 4 = défense (vérifié avec les opérateurs choisis sur les deux mi-temps).
- Match : `version`, `code`, `datetime`, `id` (identifiant de match, change à chaque rehost), `matchtype` (3 dans tout le corpus ; valeur en ranked inconnue), `worldid` (identifiant de map, voir `rec/maps.go`), `gamemodeid`, `roundnumber` (à partir de 0), `teamname0/1`, `startingteamscore0/1` (score *avant* le round), `maxnbplayersperteam`, de nombreux `gmsetting`.
- L'en-tête ne contient ni le résultat du round, ni le site de bombe (le site est dans le corps, voir plus bas).
- Replays de tournoi : `profileid` à zéro, `isspectator` = 1. Les replays de ranked seront différents ; aucun n'a encore été examiné.

**Corps**
- Un flux de mises à jour d'objets : `0x23` + identifiant d'objet sur 8 octets, puis des propriétés. Chaque propriété = clé de 4 octets (un hash, lu en big-endian dans le code), longueur sur 1 octet, valeur. Les propriétés après la première sont préfixées par `0x22`, ou par `0x26` quand un indice de tableau sur 4 octets précède la longueur.
- Objet d'état du round (repéré par sa première clé `0x1f07efc9`) : `0x1f07efc9` chrono en secondes, `0x1837466c` chrono en ms, `0x35f88fcf` drapeau d'alerte (pose, ou 10 dernières secondes), `0xbb094fd3` état du round (3 = terminé).
- Entrée du kill feed (première clé `0x5934e58b`) : `0xd9133cba` tueur, `0xac190f70` victime, `0xc198de70` / `0x782e7b50` équipe du tueur / de la victime + 1, `0xc35ba44e` headshot. Les entrées sont renvoyées à chaque redessin du feed : dédoublonner par identifiant d'objet. Les entrées sans victime sont des annonces de phase.
- Objets « étiquette » (clé `0xaf9899ca`), présents dans l'état initial du corps :
  - **Site de bombe** : valeur non indexée de la forme `"2F Master Bedroom<br/>2F Office"`, avec `0x2ca42af6` = 1. Un second objet peut porter un ancien site avec `0x2ca42af6` = 2 (par exemple le dernier site de la map précédente) : l'ignorer. L'étiquette n'est jamais mise à jour pendant le round. Vérifié sur les 84 fichiers : une seule étiquette active par round.
  - **Points d'apparition** : valeur non indexée (`"Campfire"`, `"Lakeside"`) suivie de `0x76fee40a` = 4. **Non fiable** : souvent un reste d'un round précédent, présent aussi chez les défenseurs. Non utilisé.
  - **Positions des joueurs** : `0xaf9899ca` = `ffffffffffffffff`, puis l'élément indexé `[0]` = nom de la pièce (vide si mort ou pas encore déployé), `[1]` = un octet.
- **Rattachement des positions aux joueurs** (`rec/position.go`) : dans l'état initial, chaque joueur a un bloc qui commence par son nom (clé `0x07949bdc`) et se termine au nom suivant. Le bloc contient deux objets de position : le **premier suit la vue** (drone en préparation, caméras), le **second suit le corps**. Validé sur le corpus : le corps passe à vide au moment de la mort annoncée par le kill feed (532 cas sur 556 ; les autres ne sont pas remis à zéro mais la dernière pièce connue reste juste), et les défenseurs démarrent sur leur site dans 74 rounds sur 79.
- **Tableau des scores** : un objet par joueur, envoyé juste avant l'objet qui porte son nom dans l'état initial. Compteurs cumulés sur la session d'enregistrement (u32) : `0x1cd2b19d` kills, `0xcd9c5d72` morts, `0x4d737f9e` assists, `0x31a3d4d2` poses de défuseur, `0xecda4f80` score, `0x8310f4f4` jamais mis à jour dans le corpus. Aucun compteur de désamorçages trouvé.
- **Poseur** : le joueur dont le compteur `0x31a3d4d2` augmente pendant le round (la première valeur vue sert de référence). Les 21 poses du corpus sont attribuées à un attaquant. Quand le round se termine dans le même tick que la pose (Lair, round 2), le chrono ne passe jamais au compte à rebours : la pose est alors datée par ce compteur.
- La pièce d'un kill est la dernière pièce non vide du corps avant le kill, en comptant les mises à jour du même tick de chrono (elles arrivent parfois juste après l'entrée du feed). Connue pour les 555 kills du corpus.
- Les noms de pièces peuvent différer légèrement des étiquettes de site (`"1F  Kitchen Cooking"` avec deux espaces sur Kafe) : le parser normalise les espaces.
- Sur Kafe, les défenseurs du site Fireplace Hall / Mining Room démarrent dans le Train Museum voisin.
- Un événement prend l'heure de la dernière mise à jour du chrono qui le précède dans le corps (le corps est chronologique).

**Pièges connus**
- Le vainqueur du round est déduit des règles du jeu (élimination, pose + temps écoulé, pose sans temps écoulé = désamorçage, temps écoulé), pas lu dans le fichier.
- Une pose est détectée par le saut du chrono vers le haut (compte à rebours du défuseur), par le drapeau d'alerte avec plus de 11,5 s restantes, ou à défaut par le compteur de poses du tableau des scores.
- Quand une pose en cours prolonge le round au-delà de 0:00, le chrono se fige sous 1 s : les événements tardifs s'affichent à 2:59.
- Un enregistrement fait juste après un rehost commence dans l'état « terminé », puis en sort.
- Rehosts : même map et mêmes équipes ; un numéro de round répété remplace l'enregistrement précédent (incomplet). Certains fichiers d'après rehost ne listent qu'une équipe dans l'en-tête ; `match.Roster` reprend l'autre équipe d'un round voisin.
- Les clés et la structure changeront d'une saison à l'autre. Plan : aiguiller selon `version`/`code`, garder un corpus de test par saison, signaler les matchs qui échouent aux contrôles de cohérence plutôt que d'afficher des stats fausses.

## Définitions des stats (`stats/`)

- Ouverture : premier kill du round. Un team kill en premier compte comme première mort pour cette équipe et comme premier kill pour personne.
- Trade : le tueur meurt, tué par l'équipe de la victime, dans les `TradeWindowMs` (5000 par défaut). Un kill qui venge plusieurs morts compte pour un seul kill de trade.
- Clutch : joueur resté seul dans son équipe ; taille = nombre d'ennemis en vie à ce moment ; gagné si l'équipe gagne le round.
- Situations : chaque état NvM compté une fois par round, avec le nombre de rounds gagnés.
- Pièces : kills comptés dans la pièce du tueur, morts dans celle de la victime, séparés par côté (`Report.Rooms`), et par joueur (`KillRooms`, `DeathRooms`).
- Sites : par équipe, map et site, rounds joués et gagnés en attaque et en défense, et rounds d'attaque avec pose (et gagnés). Les rounds sans site connu ne comptent pas dans cette ventilation.
- Les joueurs sont identifiés par leur profile id quand il existe, par leur nom sinon. Les rounds sans vainqueur connu sont écartés et comptés dans `SkippedRounds`.

## Prochaines étapes

1. Calculer le KOST (kill, objectif, survie, trade) et exploiter les assists du tableau des scores. Le désamorceur reste inconnu : piste, le défenseur présent dans une pièce du site au moment du désamorçage. Site, positions et poseur sont faits.
2. Ventiler les stats existantes par côté, map et opérateur.
3. Récupérer des replays de ranked sur le PC de l'utilisateur pour vérifier `matchtype`, les profile ids et les différences avec les fichiers de tournoi.
4. Ensuite le serveur, l'interface web et l'app Windows.

Toujours afficher la taille de l'échantillon à côté d'un pourcentage : les clutches et les situations rares reposent sur peu de cas.
