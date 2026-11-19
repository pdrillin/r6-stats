import { NavLink, Route, Routes } from "react-router-dom";
import { MatchPage } from "./pages/MatchPage";
import { MatchesPage } from "./pages/MatchesPage";
import { OverviewPage } from "./pages/OverviewPage";
import { PlayerPage } from "./pages/PlayerPage";
import { RoomsPage } from "./pages/RoomsPage";
import { TeamPage } from "./pages/TeamPage";
import { TeamsPage } from "./pages/TeamsPage";

export function App() {
  return (
    <>
      <header className="topbar">
        <NavLink to="/" className="brand">
          r6-stats
        </NavLink>
        <nav>
          <NavLink to="/" end>
            Joueurs
          </NavLink>
          <NavLink to="/teams">Équipes</NavLink>
          <NavLink to="/matches">Matchs</NavLink>
          <NavLink to="/rooms">Pièces</NavLink>
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
