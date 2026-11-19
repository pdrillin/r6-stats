import { NavLink, Route, Routes, useLocation } from "react-router-dom";
import { MatchPage } from "./pages/MatchPage";
import { MatchesPage } from "./pages/MatchesPage";
import { OverviewPage } from "./pages/OverviewPage";
import { PlayerPage } from "./pages/PlayerPage";
import { RoomsPage } from "./pages/RoomsPage";
import { TeamPage } from "./pages/TeamPage";
import { TeamsPage } from "./pages/TeamsPage";

export function App() {
  // The map filter follows the reader from page to page.
  const map = new URLSearchParams(useLocation().search).get("map");
  const keep = (path: string) => (map ? `${path}?map=${encodeURIComponent(map)}` : path);
  return (
    <>
      <header className="topbar">
        <NavLink to={keep("/")} className="brand">
          r6-stats
        </NavLink>
        <nav>
          <NavLink to={keep("/")} end>
            Joueurs
          </NavLink>
          <NavLink to={keep("/teams")}>Équipes</NavLink>
          <NavLink to={keep("/matches")}>Matchs</NavLink>
          <NavLink to={keep("/rooms")}>Pièces</NavLink>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/players/:id" element={<PlayerPage />} />
          <Route path="/teams" element={<TeamsPage />} />
          <Route path="/teams/:name" element={<TeamPage />} />
          <Route path="/matches" element={<MatchesPage />} />
          <Route path="/matches/:id" element={<MatchPage />} />
          <Route path="/rooms" element={<RoomsPage />} />
          <Route path="*" element={<p className="error">Page introuvable.</p>} />
        </Routes>
      </main>
    </>
  );
}
