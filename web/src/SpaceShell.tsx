import { useEffect } from "react";
import { App } from "./App";
import { useApi } from "./api";
import { Loading } from "./components/Loading";
import { WhereContext, signIn, type Space, type User } from "./space";

/** Runs the statistics pages inside one space, for its members. */
export function SpaceShell({ id }: { id: string }) {
  const me = useApi<User>("/me");
  const spaces = useApi<Space[]>("/spaces");
  // The session ended: back to Discord, then here.
  useEffect(() => {
    if (me.status === 401) signIn();
  }, [me.status]);
  if (me.status === 401) return null;
  return (
    <Loading state={me}>
      {(user) => (
        <Loading state={spaces}>
          {(list) => {
            const space = list.find((s) => s.id === id);
            if (!space) {
              return (
                <main>
                  <h1>Espace introuvable</h1>
                  <p className="muted">
                    Cet espace n'existe pas, ou tu n'en es pas membre. Pour le rejoindre, demande un lien d'invitation à
                    un responsable.
                  </p>
                  <p>
                    <a href="/">Mes espaces</a>
                  </p>
                </main>
              );
            }
            return (
              <WhereContext.Provider
                value={{ base: `spaces/${id}/`, space, canEdit: space.role === "manager", server: true, me: user }}
              >
                <App />
              </WhereContext.Provider>
            );
          }}
        </Loading>
      )}
    </Loading>
  );
}
