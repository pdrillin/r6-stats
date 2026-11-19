import { useState } from "react";
import { useParams } from "react-router-dom";
import { useApi, useSend } from "../api";
import { Loading } from "../components/Loading";
import { roleLabels, signIn, spacePath, type Role } from "../space";

interface InvitationInfo {
  spaceId: string;
  spaceName: string;
  spaceKind: "team" | "league";
  role: Role;
  expiresAt: string;
}

/** Where an invitation link leads: join a space, after signing in. */
export function InvitePage() {
  const { token = "" } = useParams();
  const info = useApi<InvitationInfo>("/invitations/" + encodeURIComponent(token));
  const send = useSend();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  if (info.status === 401) {
    return (
      <main className="narrow">
        <h1>Invitation</h1>
        <p>Connecte-toi pour rejoindre l'espace auquel ce lien t'invite.</p>
        <p>
          <button className="button primary" onClick={() => signIn()}>
            Se connecter avec Discord
          </button>
        </p>
      </main>
    );
  }
  if (info.status === 410) {
    return (
      <main className="narrow">
        <h1>Invitation expirée</h1>
        <p className="muted">Ce lien a expiré ou a été révoqué. Demande un nouveau lien à un responsable de l'espace.</p>
        <p>
          <a href="/">Mes espaces</a>
        </p>
      </main>
    );
  }

  const accept = async (spaceId: string) => {
    setBusy(true);
    setError(undefined);
    try {
      await send("POST", `/invitations/${encodeURIComponent(token)}/accept`);
      window.location.assign(spacePath(spaceId));
    } catch (err) {
      setError(String(err));
      setBusy(false);
    }
  };

  return (
    <main className="narrow">
      <Loading state={info}>
        {(inv) => (
          <>
            <h1>Rejoindre {inv.spaceName}</h1>
            <p>
              Tu es invité {inv.spaceKind === "league" ? "dans la ligue" : "dans l'équipe"} <b>{inv.spaceName}</b> en tant
              que <b>{roleLabels[inv.role].toLowerCase()}</b>.
            </p>
            <p className="muted">
              {inv.role === "manager"
                ? "Un responsable invite et retire des membres, étiquette les matchs et choisit l'équipe suivie."
                : "Un joueur envoie des replays et consulte toutes les stats de l'espace."}
            </p>
            <p>
              <button className="button primary" disabled={busy} onClick={() => accept(inv.spaceId)}>
                Rejoindre
              </button>
            </p>
            {error && <p className="error">{error}</p>}
          </>
        )}
      </Loading>
    </main>
  );
}
