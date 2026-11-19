import { useState } from "react";
import { useApi, useSend } from "../api";
import { Loading } from "../components/Loading";
import { date } from "../format";
import { roleLabels, useWhere, type Role, type User } from "../space";

interface Member extends User {
  role: Role;
  joinedAt: string;
}

interface Invitation {
  id: string;
  role: Role;
  createdBy: string;
  createdAt: string;
  expiresAt: string;
}

export function MembersPage() {
  const { space, me, canEdit } = useWhere();
  const members = useApi<Member[]>("members");
  return (
    <>
      <h1>Membres de {space?.name}</h1>
      <p className="muted">
        Les responsables invitent, retirent des membres et étiquettent les matchs ; les joueurs envoient des replays et
        consultent les stats. Un espace garde toujours au moins un responsable.
      </p>
      <Loading state={members}>{(list) => <MemberList list={list} me={me} canEdit={canEdit} />}</Loading>
      {canEdit && <Invitations />}
      {canEdit && <DeleteSpace />}
    </>
  );
}

/** Deletes the whole space, for managers: typing its name confirms. */
function DeleteSpace() {
  const { space } = useWhere();
  const send = useSend();
  const [typed, setTyped] = useState("");
  const [error, setError] = useState<string>();
  if (!space) return null;
  const remove = async () => {
    try {
      await send("DELETE", "/spaces/" + space.id);
      window.location.assign("/");
    } catch (err) {
      setError(String(err));
    }
  };
  return (
    <section className="danger-zone">
      <h2>Supprimer l'espace</h2>
      <p className="muted">
        Supprime l'espace pour tous ses membres, avec ses matchs, ses statistiques et ses invitations. C'est définitif.
        Pour confirmer, écris son nom : <b>{space.name}</b>
      </p>
      <div className="filters">
        <input value={typed} onChange={(e) => setTyped(e.target.value)} aria-label="Nom de l'espace" />
        <button className="button danger" disabled={typed !== space.name} onClick={remove}>
          Supprimer définitivement
        </button>
      </div>
      {error && <p className="error">{error}</p>}
    </section>
  );
}

function Avatar({ u }: { u: User }) {
  // Accounts made by the development sign-in have no Discord avatar.
  if (u.avatar && !u.discordId.startsWith("dev:")) {
    return (
      <img
        className="avatar"
        src={`https://cdn.discordapp.com/avatars/${u.discordId}/${u.avatar}.png?size=64`}
        alt=""
        width={28}
        height={28}
      />
    );
  }
  return (
    <span className="avatar" aria-hidden="true">
      {u.name.slice(0, 1).toUpperCase()}
    </span>
  );
}

function MemberList({ list, me, canEdit }: { list: Member[]; me?: User; canEdit: boolean }) {
  const send = useSend();
  const [error, setError] = useState<string>();
  const act = async (f: () => Promise<unknown>, after?: () => void) => {
    setError(undefined);
    try {
      await f();
      after?.();
    } catch (err) {
      setError(String(err).includes("409") ? "Un espace garde toujours au moins un responsable." : String(err));
    }
  };
  return (
    <>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th className="text">Membre</th>
              <th className="text">Rôle</th>
              <th className="text">Depuis</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {list.map((m) => {
              const self = m.id === me?.id;
              return (
                <tr key={m.id}>
                  <td className="text">
                    <span className="member">
                      <Avatar u={m} /> {m.name}
                      {self && <span className="muted"> (toi)</span>}
                    </span>
                  </td>
                  <td className="text">
                    {canEdit ? (
                      <select
                        value={m.role}
                        aria-label={`Rôle de ${m.name}`}
                        onChange={(e) => act(() => send("PUT", `members/${m.id}`, { role: e.target.value }))}
                      >
                        {(Object.keys(roleLabels) as Role[]).map((r) => (
                          <option key={r} value={r}>
                            {roleLabels[r]}
                          </option>
                        ))}
                      </select>
                    ) : (
                      roleLabels[m.role]
                    )}
                  </td>
                  <td className="text muted">{date(m.joinedAt)}</td>
                  <td>
                    {self ? (
                      <button
                        className="link-button"
                        onClick={() =>
                          confirm("Quitter cet espace ? Il faudra une nouvelle invitation pour y revenir.") &&
                          act(
                            () => send("DELETE", `members/${m.id}`),
                            () => window.location.assign("/"),
                          )
                        }
                      >
                        Quitter
                      </button>
                    ) : (
                      canEdit && (
                        <button
                          className="link-button"
                          onClick={() =>
                            confirm(`Retirer ${m.name} de l'espace ?`) && act(() => send("DELETE", `members/${m.id}`))
                          }
                        >
                          Retirer
                        </button>
                      )
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {error && <p className="error">{error}</p>}
    </>
  );
}

function Invitations() {
  const list = useApi<Invitation[]>("invitations");
  const send = useSend();
  const [link, setLink] = useState<{ url: string; role: Role }>();
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string>();

  const create = async (role: Role) => {
    setError(undefined);
    setCopied(false);
    try {
      const inv = await send<{ url: string; role: Role }>("POST", "invitations", { role });
      setLink(inv);
    } catch (err) {
      setError(String(err));
    }
  };
  const copy = async () => {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link.url);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <>
      <h2>Inviter</h2>
      <p className="muted">
        Un lien d'invitation marche 7 jours, pour autant de personnes que tu veux : envoie-le sur le Discord de ton
        équipe. Chacun se connecte avec son compte Discord et rejoint l'espace.
      </p>
      <div className="filters">
        <button className="button" onClick={() => create("player")}>
          Créer un lien joueur
        </button>
        <button className="button" onClick={() => create("manager")}>
          Créer un lien responsable
        </button>
      </div>
      {link && (
        <div className="invite-link">
          <p>
            Lien {roleLabels[link.role].toLowerCase()} :{" "}
            <span className="muted">copie-le maintenant, il ne sera plus affiché.</span>
          </p>
          <div className="filters">
            <input readOnly value={link.url} onFocus={(e) => e.target.select()} aria-label="Lien d'invitation" />
            <button className="button" onClick={copy}>
              {copied ? "Copié" : "Copier"}
            </button>
          </div>
        </div>
      )}
      {error && <p className="error">{error}</p>}

      <h3>Liens en cours</h3>
      <Loading state={list}>
        {(invs) =>
          invs.length === 0 ? (
            <p className="muted">Aucun lien actif.</p>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th className="text">Rôle</th>
                    <th className="text">Créé par</th>
                    <th className="text">Expire</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {invs.map((i) => (
                    <tr key={i.id}>
                      <td className="text">{roleLabels[i.role]}</td>
                      <td className="text">{i.createdBy || <span className="muted">ligne de commande</span>}</td>
                      <td className="text muted">{date(i.expiresAt)}</td>
                      <td>
                        <button
                          className="link-button"
                          onClick={() => send("DELETE", `invitations/${i.id}`).catch((e: unknown) => setError(String(e)))}
                        >
                          Révoquer
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        }
      </Loading>
    </>
  );
}
