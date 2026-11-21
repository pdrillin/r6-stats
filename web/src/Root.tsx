import { useEffect, useState } from "react";
import { Route, Routes } from "react-router-dom";
import { App } from "./App";
import { Footer } from "./components/Footer";
import { InvitePage } from "./pages/InvitePage";
import { LobbyPage } from "./pages/LobbyPage";
import { PrivacyPage } from "./pages/PrivacyPage";

type Mode = "probing" | "local" | "server";

/**
 * The start of the interface outside a space. stratrec serve has no accounts:
 * it answers 404 to /api/me, and the statistics pages run at the root.
 * stratdata knows /api/me: the start page lists the user's spaces.
 */
export function Root() {
  const [mode, setMode] = useState<Mode>("probing");
  useEffect(() => {
    fetch("/api/me").then(
      (res) => setMode(res.status === 404 ? "local" : "server"),
      () => setMode("server"),
    );
  }, []);
  if (mode === "probing") return null;
  if (mode === "local") return <App />;
  return (
    <div className="page-frame">
      <header className="topbar">
        <a href="/" className="brand">
          Stratdata
        </a>
      </header>
      <Routes>
        <Route path="/invite/:token" element={<InvitePage />} />
        <Route path="/privacy" element={<PrivacyPage />} />
        <Route path="*" element={<LobbyPage />} />
      </Routes>
      <Footer />
    </div>
  );
}

