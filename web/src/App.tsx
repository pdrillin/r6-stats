import { Route, Routes } from "react-router-dom";
import { Shell } from "./components/Shell";
import { MembersPage } from "./pages/MembersPage";
import { UploadPage } from "./pages/UploadPage";
import { useWhere } from "./space";
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
  const { server } = useWhere();
  return (
    <Shell>
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
          {server && <Route path="/upload" element={<UploadPage />} />}
          {server && <Route path="/members" element={<MembersPage />} />}
          <Route path="*" element={<p className="error">Page introuvable.</p>} />
        </Routes>
      </main>
    </Shell>
  );
}
