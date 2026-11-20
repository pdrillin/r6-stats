import type { Player } from "../api";

/**
 * How many of the players' deaths have a known room. Replays recorded by a
 * player, unlike those recorded by a spectator, carry no player positions:
 * their kills have no room.
 */
export function roomCoverage(players: Player[]): {
  known: number;
  total: number;
} {
  let known = 0;
  let total = 0;
  for (const p of players) {
    total += p.deaths;
    for (const n of Object.values(p.deathRooms ?? {})) known += n;
  }
  return { known, total };
}

/**
 * Says when rooms are missing for some or all of the deaths, so that an empty
 * or thin table is not read as "nobody died there".
 */
export function RoomCoverageNote({ players }: { players: Player[] }) {
  const { known, total } = roomCoverage(players);
  if (total === 0 || known === total) return null;
  if (known === 0) {
    return (
      <p className="muted" role="note">
        Pièces non disponibles pour ces matchs : les replays enregistrés par un joueur ne contiennent pas la position
        des joueurs. Seuls ceux enregistrés en spectateur l'ont.
      </p>
    );
  }
  return (
    <p className="muted" role="note">
      Pièce connue pour {known} morts sur {total} : les replays enregistrés par un joueur ne contiennent pas la position
      des joueurs.
    </p>
  );
}
