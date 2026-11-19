import { useState, type FormEvent } from "react";
import { useApi, useSend } from "../api";
import { Loading } from "../components/Loading";
import { roleLabels, signIn, signOut, spacePath, type Space, type User } from "../space";

type Me = User & { canCreate: boolean };

/** The start page with r6server: sign in, then the user's spaces. */
export function LobbyPage() {
  const me = useApi<Me>("/me");
  if (me.status === 401) return <SignIn />;
  return (
    <main className="narrow">
      <Loading state={me}>{(user) => <Spaces me={user} />}</Loading>
    </main>
  );
}

function SignIn() {
  return (
    <main className="narrow">
      <h1>r6-stats</h1>
      <p>
        Les statistiques avancées des matchs de ton équipe, tirées des replays du jeu. Chaque espace est privé : seuls
        ses membres y ont accès.
      </p>
      <p>
        <button className="button primary" onClick={() => signIn("/")}>
          Se connecter avec Discord
        </button>
      </p>
      <p className="muted">Seuls ton pseudo et ton avatar Discord sont lus.</p>
    </main>
  );
}

function Spaces({ me }: { me: Me }) {
  const spaces = useApi<Space[]>("/spaces");
  return (
    <>
      <div className="lobby-head">
        <h1>Mes espaces</h1>
        <span className="muted">
          {me.name} ·{" "}
          <button className="link-button" onClick={signOut}>
            Déconnexion
          </button>
        </span>
      </div>
      <Loading state={spaces}>
        {(list) => (
          <>
            {list.length === 0 && (
              <p className="muted">
                Tu n'es membre d'aucun espace.{" "}
                {me.canCreate
                  ? "Crée celui de ton équipe, ou rejoins-en un avec un lien d'invitation."
                  : "Pendant la bêta, on rejoint un espace avec le lien d'invitation d'un responsable."}
              </p>
            )}
            <ul className="space-list">
              {list.map((s) => (
                <li key={s.id}>
                  <a href={spacePath(s.id)} className="space-card">
                    <b>{s.name}</b>
                    <span className="muted">
                      {s.kind === "league" ? "Ligue" : "Équipe"} · {roleLabels[s.role]} · {s.files} round
                      {s.files > 1 ? "s" : ""}
                    </span>
                  </a>
                </li>
              ))}
            </ul>
          </>
        )}
      </Loading>
      {me.canCreate && <CreateSpace />}
    </>
  );
}

function CreateSpace() {
  const send = useSend();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<"team" | "league">("team");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      const sp = await send<Space>("POST", "/spaces", { name, kind });
      // A new space starts empty: first thing to do is send replays.
      window.location.assign(spacePath(sp.id, "upload"));
    } catch (err) {
      setError(String(err));
      setBusy(false);
    }
  };
  return (
    <form className="create-space" onSubmit={submit}>
      <h2>Créer un espace</h2>
      <div className="filters">
        <label>
          <span className="muted">Nom</span>
          <input value={name} onChange={(e) => setName(e.target.value)} maxLength={80} required placeholder="Ma team" />
        </label>
        <label>
          <span className="muted">Type</span>
          <select value={kind} onChange={(e) => setKind(e.target.value as "team" | "league")}>
            <option value="team">Équipe</option>
            <option value="league">Ligue ou tournoi</option>
          </select>
        </label>
        <button className="button primary" disabled={busy || !name.trim()}>
          Créer
        </button>
      </div>
      <p className="muted">Tu en seras responsable : tu pourras inviter ton équipe et ton coach.</p>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
