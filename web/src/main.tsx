import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { Root } from "./Root";
import { SpaceShell } from "./SpaceShell";
import "./style.css";

// Inside a space (/s/<id>/…) the statistics pages run with the space's
// address as basename: their links and routes stay the same whether they
// are served by r6rec (no space) or r6server. Changing space reloads the page.
const inSpace = /^\/s\/([a-z0-9]+)(?=\/|$)/.exec(window.location.pathname);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {inSpace ? (
      <BrowserRouter basename={`/s/${inSpace[1]}`}>
        <SpaceShell id={inSpace[1]!} />
      </BrowserRouter>
    ) : (
      <BrowserRouter>
        <Root />
      </BrowserRouter>
    )}
  </StrictMode>,
);
