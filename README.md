# r6-stats

Statistiques avancées de matchs Rainbow Six Siege, calculées à partir des fichiers de replay `.rec` du jeu.

Le parser est écrit de zéro en Go. Il est validé sur des replays de la saison Y11S2 ; les autres saisons ne sont pas prises en charge pour l'instant.

## Ce qui est décodé

- **En-tête** : map, équipes, score avant le round, joueurs avec opérateur et côté (attaque/défense), version du jeu.
- **Événements** : kills horodatés (tueur, victime, headshot), pose du défuseur, vainqueur du round et raison.
- **Matchs** : regroupement des rounds en matchs, y compris les parties privées rehostées (les enregistrements de rounds interrompus sont écartés).

## Statistiques

- Duels d'ouverture : premier kill et première mort, avec le taux de victoire du round qui suit.
- Trades : morts vengées dans les 5 secondes, kills de vengeance.
- Clutches : situations 1vX rencontrées et gagnées.
- Avantages numériques : taux de victoire par situation (5v4, 3v2, 1v3…).

## Utilisation

Prérequis : Go 1.25 ou plus récent.

```sh
go run ./cmd/r6rec ls <dossier>        # liste les rounds trouvés
go run ./cmd/r6rec matches <dossier>   # matchs reconstitués, round par round
go run ./cmd/r6rec stats <dossier>     # statistiques (ajouter -json pour la sortie JSON)
go run ./cmd/r6rec round <fichier.rec> # événements d'un round en JSON
```

Sous Windows, les replays sont dans le dossier `MatchReplay` du jeu, avec un sous-dossier par match.

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
| `cmd/r6rec/` | Outil en ligne de commande |

## Tests

```sh
go test ./...
```

Une partie des tests tourne sur de vrais replays placés dans `Replays/` à la racine. Ce dossier n'est pas versionné (taille et droits) : sans lui, ces tests sont ignorés et seuls les tests unitaires s'exécutent.
