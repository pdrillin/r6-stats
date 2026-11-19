// Takes the screenshots of the README (docs/screenshots/).
//
// It needs an empty r6server started with the development sign-in, and the
// replay corpus:
//
//   go run ./cmd/r6server -data /tmp/r6-shots serve -dev-login -web web/dist
//   cd web && npm run screenshots
//
// The script signs in, creates a space, sends the corpus, follows FaZe, then
// photographs the pages. Environment: R6_URL (default
// http://127.0.0.1:8080), R6_REPLAYS (default ../Replays), CHROME (a
// Chrome or Chromium executable, default /usr/bin/google-chrome).

import { readdir, readFile, mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import { chromium } from "playwright-core";

const base = process.env.R6_URL ?? "http://127.0.0.1:8080";
const replays = resolve(process.env.R6_REPLAYS ?? "../Replays");
const out = resolve("../docs/screenshots");
const team = "FAZE";

async function recFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map((e) => (e.isDirectory() ? recFiles(join(dir, e.name)) : e.name.endsWith(".rec") ? [join(dir, e.name)] : [])),
  );
  return nested.flat();
}

const browser = await chromium.launch({ executablePath: process.env.CHROME ?? "/usr/bin/google-chrome" });
const desktop = await browser.newContext({ viewport: { width: 1366, height: 900 }, colorScheme: "dark", deviceScaleFactor: 1 });
const page = await desktop.newPage();

// Sign in, create the space, send the corpus through the API.
await page.goto(`${base}/auth/dev?name=Coach&next=/`);
const api = page.request;
const space = await (
  await api.post(`${base}/api/spaces`, { data: { name: "Ma team", kind: "team" }, headers: { Origin: base } })
).json();
if (!space.id) throw new Error("could not create the space: is the server empty and started with -dev-login?");
const files = await recFiles(replays);
for (let i = 0; i < files.length; i += 20) {
  const batch = files.slice(i, i + 20);
  const form = new FormData();
  for (const f of batch) form.append("files", new Blob([await readFile(f)]), f.split("/").pop());
  const res = await fetch(`${base}/api/spaces/${space.id}/replays`, {
    method: "POST",
    body: form,
    headers: {
      "X-R6-Upload": "1",
      Origin: base,
      Cookie: (await desktop.cookies()).map((c) => `${c.name}=${c.value}`).join("; "),
    },
  });
  if (!res.ok) throw new Error(`upload: ${res.status} ${await res.text()}`);
}
await api.put(`${base}/api/spaces/${space.id}/settings`, { data: { myTeam: team }, headers: { Origin: base } });
console.log(`${files.length} replays sent to space ${space.id}`);

// A second member, so the home page shows the dashboard and not the
// "getting started" steps.
const invitation = await (
  await api.post(`${base}/api/spaces/${space.id}/invitations`, { data: { role: "player" }, headers: { Origin: base } })
).json();
const mate = await browser.newContext();
const matePage = await mate.newPage();
await matePage.goto(`${base}/auth/dev?name=Joueur&next=/`);
await matePage.request.post(`${base}/api/invitations/${invitation.token}/accept`, { headers: { Origin: base } });
await mate.close();

await mkdir(out, { recursive: true });
const shot = async (p, name, opts = {}) => {
  await p.waitForTimeout(400); // charts measure their width once laid out
  await p.screenshot({ path: join(out, name), type: "jpeg", quality: 85, ...opts });
  console.log(name);
};
const spaceURL = (path = "") => `${base}/s/${space.id}/${path}`;

await page.goto(spaceURL());
await page.waitForSelector(".insights");
await shot(page, "accueil.jpg");

await page.goto(spaceURL("matches"));
await page.click(".match-card:has-text('Lair')");
await page.waitForSelector("figure.chart");
await shot(page, "match.jpg");

await page.goto(spaceURL("teams/FUR/scout?map=Bank"));
await page.waitForSelector(".rate-bars");
await shot(page, "fiche-adversaire.jpg");

await page.goto(spaceURL("players"));
await page.waitForSelector("table");
await shot(page, "joueurs.jpg");

await page.goto(spaceURL("upload"));
await page.waitForSelector(".dropzone");
await shot(page, "envoi.jpg", { clip: { x: 0, y: 0, width: 1366, height: 480 } });

// Phone: the home page, and the menu open.
const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, colorScheme: "dark", deviceScaleFactor: 2 });
await phone.addCookies(await desktop.cookies());
const mobile = await phone.newPage();
await mobile.goto(spaceURL());
await mobile.waitForSelector(".insights");
await shot(mobile, "mobile-accueil.jpg", { scale: "css" });
await mobile.click(".menu-button");
await shot(mobile, "mobile-menu.jpg", { scale: "css" });

await browser.close();
