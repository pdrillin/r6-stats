# r6-stats

Statistiques avancées de matchs Rainbow Six Siege, calculées à partir des fichiers de replay `.rec` du jeu.

Le parser est écrit de zéro en Go. Il est validé sur des replays de la saison Y11S2 ; les autres saisons ne sont pas prises en charge pour l'instant.

## Ce qui est décodé

- **En-tête** : map, équipes, score avant le round, joueurs avec opérateur et côté (attaque/défense), version du jeu.
- **Événements** : kills horodatés (tueur, victime, headshot), pose du défuseur, poseur et désamorceur, site de bombe, vainqueur du round et raison.
- **Positions** : pièce où se trouve chaque joueur pendant le round, et pièce du tueur et de la victime pour chaque kill.
- **Matchs** : regroupement des rounds en matchs, y compris les parties privées rehostées (les enregistrements de rounds interrompus sont écartés).

## Statistiques

- Assists, lus dans le tableau des scores du jeu.
- KOST : part des rounds avec un kill, une pose ou un désamorçage, une survie ou une mort tradée, et taux de survie.
- Duels d'ouverture : premier kill et première mort, avec le taux de victoire du round qui suit.
- Trades : morts vengées dans les 5 secondes, kills de vengeance.
- Clutches : situations 1vX rencontrées et gagnées.
- Avantages numériques : taux de victoire par situation (5v4, 3v2, 1v3…).
- Sites de bombe : taux de victoire en attaque et en défense, et taux de pose, par site.
- Poses et désamorçages par joueur, avec les rounds gagnés après pose.
- Pièces : kills et morts par pièce et par côté, et par joueur.
- Ventilation des stats des joueurs par côté, map et opérateur, et des équipes par côté et map.
- Moment du round : kills et morts par tranche de 30 secondes.

## Utilisation

Prérequis : Go 1.25 ou plus récent.

```sh
go run ./cmd/r6rec ls <dossier>        # liste les rounds trouvés
go run ./cmd/r6rec matches <dossier>   # matchs reconstitués, round par round
go run ./cmd/r6rec stats <dossier>     # statistiques (ajouter -json pour la sortie JSON)
go run ./cmd/r6rec rooms <dossier>     # kills et morts par pièce
go run ./cmd/r6rec split side <dossier>  # stats par côté (aussi : map, operator)
go run ./cmd/r6rec round <fichier.rec> # événements d'un round en JSON
```

### Interface web

```sh
go run ./cmd/r6rec serve Replays          # API sur http://127.0.0.1:8080
cd web && npm install && npm run dev      # interface sur http://localhost:5173
```

Pour servir l'interface construite directement depuis l'outil (Node n'est alors nécessaire que pour la construire) :

```sh
cd web && npm run build && cd ..
go run ./cmd/r6rec serve -web web/dist Replays   # tout sur http://127.0.0.1:8080
```

Sous Windows, les replays sont dans le dossier `MatchReplay` du jeu, avec un sous-dossier par match.

Pour installer l'outil sans cloner le dépôt :

```sh
go install github.com/pdrillin/r6-stats/cmd/r6rec@latest
```

Pour produire l'exécutable Windows depuis n'importe quel système :

```sh
GOOS=windows GOARCH=amd64 go build -o r6rec.exe ./cmd/r6rec
```

## Organisation du code

| Dossier | Rôle |
|---|---|
| `rec/` | Lecture des fichiers `.rec` : en-tête, décompression, événements du round |
| `match/` | Regroupement des rounds en matchs, gestion des rehosts |
| `stats/` | Calcul des statistiques |
| `api/` | API JSON servie à l'interface web |
| `cmd/r6rec/` | Outil en ligne de commande |
| `web/` | Interface web (React, TypeScript, Vite) |

## Tests

```sh
go test ./...
```

Une partie des tests tourne sur de vrais replays placés dans `Replays/` à la racine. Ce dossier n'est pas versionné (taille et droits) : sans lui, ces tests sont ignorés et seuls les tests unitaires s'exécutent.
