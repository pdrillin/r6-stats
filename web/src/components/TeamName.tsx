import { useState, type FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useSend, type TeamNamedBy } from "../api";
import { useWhere } from "../space";
import { teamPath } from "./PlayerTable";

const MAX = 40;

const origins: Partial<Record<TeamNamedBy, string>> = {
  tag: "Nom proposé d'après le tag que portent la plupart de ses joueurs.",
  player:
    "Nom provisoire : le replay appelle chaque adversaire « ENEMY TEAM ». L'équipe est reconnue à ses joueurs (3 en commun d'un match à l'autre).",
  manager: "Nom choisi par un responsable. Il suit ses joueurs dans les prochains matchs.",
};

/**
 * Where a team name comes from and, for a manager, a form to rename it. The
 * name sticks to the team's players: it applies to every match they play
 * together, past and future.
 */
export function TeamName({ name, by }: { name: string; by?: TeamNamedBy }) {
  const { canEdit } = useWhere();
  const send = useSend();
  const navigate = useNavigate();
  const { search } = useLocation();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const save = async (next: string) => {
    setBusy(true);
    setError("");
    try {
      const res = await send<{ name: string }>("PUT", "teams/name", { team: name, name: next });
      setEditing(false);
      navigate(teamPath(res.name) + search, { replace: true });
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save(value);
  };

  const origin = by && origins[by];
  if (!origin && !canEdit) return null;
  return (
    <div className="team-name">
      {origin && <p className="muted">{origin}</p>}
      {canEdit && !editing && (
        <p>
          <button
            className="button small"
            onClick={() => {
              setValue(name);
              setEditing(true);
            }}
          >
            Renommer
          </button>
          {by === "manager" && (
            <button className="link-button" disabled={busy} onClick={() => save("")}>
              Revenir au nom automatique
            </button>
          )}
        </p>
      )}
      {editing && (
        <form className="filters" onSubmit={submit}>
          <label>
            <span className="muted">Nouveau nom</span>
            <input value={value} onChange={(e) => setValue(e.target.value)} maxLength={MAX} required autoFocus />
          </label>
          <button className="button primary" disabled={busy || !value.trim() || value.trim() === name}>
            Enregistrer
          </button>
          <button type="button" className="button" onClick={() => setEditing(false)}>
            Annuler
          </button>
        </form>
      )}
      {error && <p className="error">{error}</p>}
    </div>
  );
}
