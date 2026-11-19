import { useApi, type Overview, type Room } from "../api";
import { RoomBars } from "../components/charts";
import { Loading } from "../components/Loading";
import { useMapFilter } from "../components/MapFilter";
import { Table } from "../components/Table";

export function RoomsPage() {
  const state = useApi<Overview>("overview");
  return <Loading state={state}>{(o) => <Rooms rooms={o.stats.rooms ?? []} />}</Loading>;
}

function Rooms({ rooms }: { rooms: Room[] }) {
  const maps = [...new Set(rooms.map((r) => r.map))].sort();
  // Rooms only make sense on one map: the tabs set the map filter, and
  // without one the first map is shown.
  const [filter, setMap] = useMapFilter();
  const map = maps.includes(filter) ? filter : (maps[0] ?? "");
  const rows = rooms.filter((r) => r.map === map);
  return (
    <>
      <h1>Pièces</h1>
      <p className="muted">
        Kills comptés dans la pièce du tueur, morts dans celle de la victime, séparés selon le côté du joueur.
      </p>
      <div className="tabs">
        {maps.map((m) => (
          <button key={m} className={m === map ? "active" : undefined} onClick={() => setMap(m)}>
            {m}
          </button>
        ))}
      </div>
      <h2>Où l'on meurt le plus</h2>
      <RoomBars key={map} rooms={rows} />
      <h2>Toutes les pièces</h2>
      <Table
        key={map}
        rows={rows}
        rowKey={(r) => r.room}
        initialSort={5}
        columns={[
          { label: "Pièce", text: true, sort: (r) => r.room, render: (r) => r.room },
          { label: "Kills att.", sort: (r) => r.attack.kills, render: (r) => r.attack.kills || "–" },
          { label: "Morts att.", sort: (r) => r.attack.deaths, render: (r) => r.attack.deaths || "–" },
          { label: "Kills déf.", sort: (r) => r.defense.kills, render: (r) => r.defense.kills || "–" },
          { label: "Morts déf.", sort: (r) => r.defense.deaths, render: (r) => r.defense.deaths || "–" },
          {
            label: "Morts",
            title: "Toutes les morts dans la pièce",
            sort: (r) => r.attack.deaths + r.defense.deaths,
            render: (r) => r.attack.deaths + r.defense.deaths,
          },
        ]}
      />
    </>
  );
}
