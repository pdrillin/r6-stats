import { NavLink, Route, Routes, useLocation } from "react-router-dom";
import { ComparePage } from "./pages/ComparePage";
import { HomePage } from "./pages/HomePage";
import { MatchPage } from "./pages/MatchPage";
import { MatchesPage } from "./pages/MatchesPage";
import { OverviewPage } from "./pages/OverviewPage";
import { PlayerPage } from "./pages/PlayerPage";
import { RoomsPage } from "./pages/RoomsPage";
import { ScoutPage } from "./pages/ScoutPage";
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
            Mon équipe
          </NavLink>
          <NavLink to={keep("/players")}>Joueurs</NavLink>
          <NavLink to={keep("/teams")}>Équipes</NavLink>
          <NavLink to={keep("/matches")}>Matchs</NavLink>
          <NavLink to={keep("/rooms")}>Pièces</NavLink>
          <NavLink to={keep("/compare")}>Comparer</NavLink>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/players" element={<OverviewPage />} />
          <Route path="/players/:id" element={<PlayerPage />} />
          <Route path="/teams" element={<TeamsPage />} />
          <Route path="/teams/:name" element={<TeamPage />} />
          <Route path="/teams/:name/scout" element={<ScoutPage />} />
          <Route path="/matches" element={<MatchesPage />} />
          <Route path="/matches/:id" element={<MatchPage />} />
          <Route path="/rooms" element={<RoomsPage />} />
          <Route path="/compare" element={<ComparePage />} />
          <Route path="*" element={<p className="error">Page introuvable.</p>} />
        </Routes>
      </main>
    </>
  );
}
