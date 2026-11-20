import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import { roleLabels, signOut, useWhere } from "../space";
import { filterSearch } from "./MapFilter";
import { ThemePicker } from "./ThemePicker";

/**
 * The frame of every statistics page: a side menu grouped by purpose on wide
 * screens, a drawer behind a menu button on phones.
 */
export function Shell({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const { pathname, search } = useLocation();
  const { server, space } = useWhere();
  const menuButton = useRef<HTMLButtonElement>(null);
  const firstLink = useRef<HTMLAnchorElement>(null);

  // The drawer closes once a page is chosen, and with Escape.
  useEffect(() => setOpen(false), [pathname]);
  useEffect(() => {
    if (!open) return;
    firstLink.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        menuButton.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  // The map and kind filters follow the reader from page to page.
  const keep = (path: string) => path + filterSearch(search);

  return (
    <div className="layout">
      <header className="mobilebar">
        <button
          ref={menuButton}
          className="menu-button"
          aria-label="Menu"
          aria-expanded={open}
          aria-controls="sidebar"
          onClick={() => setOpen(!open)}
        >
          <svg width="20" height="20" viewBox="0 0 20 20" aria-hidden="true">
            <path d="M3 5h14M3 10h14M3 15h14" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
          </svg>
        </button>
        <span className="mobilebar-title">{space?.name ?? "Stratdata"}</span>
        {server && (
          <Link to="/upload" className="button primary small">
            Envoyer
          </Link>
        )}
      </header>

      <aside id="sidebar" className={open ? "sidebar open" : "sidebar"}>
        <Link to={keep("/")} className="brand">
          Stratdata
        </Link>
        {server && space && <SpaceBlock />}
        {server && (
          <NavLink to="/upload" className="button primary upload-button">
            Envoyer des replays
          </NavLink>
        )}
        <nav aria-label="Navigation">
          <Group title="Équipe">
            <NavLink ref={firstLink} to={keep("/")} end>
              Accueil
            </NavLink>
            <NavLink to={keep("/matches")}>Matchs</NavLink>
            <NavLink to={keep("/teams")}>Équipes</NavLink>
          </Group>
          <Group title="Analyse">
            <NavLink to={keep("/players")}>Joueurs</NavLink>
            <NavLink to={keep("/rooms")}>Pièces</NavLink>
            <NavLink to={keep("/compare")}>Comparer</NavLink>
          </Group>
          {server && (
            <Group title="Espace">
              <NavLink to="/members">Membres</NavLink>
            </Group>
          )}
        </nav>
        {server ? (
          <AccountBlock />
        ) : (
          <div className="account-block">
            <ThemePicker />
          </div>
        )}
      </aside>
      {open && <div className="scrim" onClick={() => setOpen(false)} aria-hidden="true" />}

      <div className="content">{children}</div>
    </div>
  );
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="nav-group">
      <div className="nav-title">{title}</div>
      {children}
    </div>
  );
}

function SpaceBlock() {
  const { space } = useWhere();
  if (!space) return null;
  return (
    <div className="space-block">
      <div className="space-name">{space.name}</div>
      <div className="muted">
        {space.kind === "league" ? "Ligue" : "Équipe"} · {roleLabels[space.role].toLowerCase()}
      </div>
      <a href="/" className="small-link">
        Changer d'espace
      </a>
    </div>
  );
}

function AccountBlock() {
  const { me } = useWhere();
  return (
    <div className="account-block">
      {me && <div className="muted">{me.name}</div>}
      <div className="account-links">
        <button className="link-button" onClick={signOut}>
          Déconnexion
        </button>
        <a href="/privacy" className="small-link">
          Confidentialité
        </a>
      </div>
      <ThemePicker />
    </div>
  );
}
