import { useEffect, useRef, useState, type DragEvent } from "react";
import { Link } from "react-router-dom";
import { invalidate } from "../api";
import { useWhere } from "../space";

interface Result {
  name: string;
  sha256?: string;
  status: "added" | "already" | "ignored" | "invalid";
  error?: string;
}

// A request carries at most this many files and bytes: progress stays
// visible, and a failure only costs one batch.
const BATCH_FILES = 20;
const BATCH_BYTES = 150 << 20;

/** Splits files into batches. */
function batches(files: File[]): File[][] {
  const out: File[][] = [];
  let cur: File[] = [];
  let size = 0;
  for (const f of files) {
    if (cur.length > 0 && (cur.length >= BATCH_FILES || size + f.size > BATCH_BYTES)) {
      out.push(cur);
      cur = [];
      size = 0;
    }
    cur.push(f);
    size += f.size;
  }
  if (cur.length > 0) out.push(cur);
  return out;
}

/** Reads every file of a dropped folder, however deep. */
async function readEntry(entry: FileSystemEntry): Promise<File[]> {
  if (entry.isFile) {
    return new Promise((resolve) => (entry as FileSystemFileEntry).file((f) => resolve([f]), () => resolve([])));
  }
  if (!entry.isDirectory) return [];
  const reader = (entry as FileSystemDirectoryEntry).createReader();
  const children: FileSystemEntry[] = [];
  // readEntries returns the content in chunks: call until it is empty.
  for (;;) {
    const chunk = await new Promise<FileSystemEntry[]>((resolve) => reader.readEntries(resolve, () => resolve([])));
    if (chunk.length === 0) break;
    children.push(...chunk);
  }
  const nested = await Promise.all(children.map(readEntry));
  return nested.flat();
}

/** Sends one batch, reporting the bytes sent. */
function sendBatch(url: string, files: File[], onProgress: (sent: number) => void): Promise<Result[]> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    for (const f of files) form.append("files", f, f.name);
    const xhr = new XMLHttpRequest();
    xhr.open("POST", url);
    // Required by the server: a form posted by another site cannot add it.
    xhr.setRequestHeader("X-R6-Upload", "1");
    xhr.upload.onprogress = (e) => onProgress(e.loaded);
    xhr.onload = () => {
      if (xhr.status === 200) resolve(JSON.parse(xhr.responseText) as Result[]);
      else reject(new Error(`${xhr.status} ${xhr.responseText.trim()}`));
    };
    xhr.onerror = () => reject(new Error("connexion au serveur perdue"));
    xhr.send(form);
  });
}

const isReplay = (f: File) => f.name.toLowerCase().endsWith(".rec");

type Phase = { kind: "idle" } | { kind: "sending"; sent: number; total: number } | { kind: "done" };

export function UploadPage() {
  const { base } = useWhere();
  const [phase, setPhase] = useState<Phase>({ kind: "idle" });
  const [results, setResults] = useState<Result[]>([]);
  const [ignored, setIgnored] = useState(0);
  const [error, setError] = useState<string>();
  const [over, setOver] = useState(false);
  const folderInput = useRef<HTMLInputElement>(null);

  // The folder picker is not in React's attribute types.
  useEffect(() => {
    folderInput.current?.setAttribute("webkitdirectory", "");
  }, []);

  const send = async (all: File[]) => {
    const files = all.filter(isReplay);
    setIgnored(all.length - files.length);
    setResults([]);
    setError(undefined);
    if (files.length === 0) {
      setPhase({ kind: "done" });
      return;
    }
    const total = files.reduce((n, f) => n + f.size, 0);
    let done = 0;
    setPhase({ kind: "sending", sent: 0, total });
    try {
      for (const batch of batches(files)) {
        const res = await sendBatch(`/api/${base}replays`, batch, (sent) =>
          setPhase({ kind: "sending", sent: done + sent, total }),
        );
        done += batch.reduce((n, f) => n + f.size, 0);
        setResults((r) => [...r, ...res]);
      }
    } catch (err) {
      setError(String(err));
    }
    invalidate();
    setPhase({ kind: "done" });
  };

  const drop = async (e: DragEvent) => {
    e.preventDefault();
    setOver(false);
    if (phase.kind === "sending") return;
    const entries = [...e.dataTransfer.items].map((i) => i.webkitGetAsEntry()).filter((x): x is FileSystemEntry => !!x);
    const files = entries.length > 0 ? (await Promise.all(entries.map(readEntry))).flat() : [...e.dataTransfer.files];
    send(files);
  };

  const count = (s: Result["status"]) => results.filter((r) => r.status === s).length;
  const sending = phase.kind === "sending";

  return (
    <>
      <h1>Envoyer des replays</h1>
      <p className="muted">
        Les replays sont dans <code>Documents\My Games\Rainbow Six - Siege\&lt;ton identifiant&gt;\MatchReplay</code>, un
        dossier par match. Glisse un ou plusieurs dossiers de match, ou tout le dossier <code>MatchReplay</code> : les
        matchs déjà envoyés sont reconnus et ignorés. Seules les parties personnalisées (praccs, matchs officiels) sont
        gardées : les ranked, parties rapides et arcades sont écartées sans être stockées.
      </p>

      <div
        className={over ? "dropzone over" : "dropzone"}
        onDragOver={(e) => {
          e.preventDefault();
          setOver(true);
        }}
        onDragLeave={() => setOver(false)}
        onDrop={drop}
      >
        <p>
          <b>Dépose ici des dossiers ou des fichiers .rec</b>
        </p>
        <p className="muted">ou</p>
        <div className="filters">
          <label className="button">
            Choisir un dossier
            <input
              ref={folderInput}
              type="file"
              multiple
              hidden
              disabled={sending}
              onChange={(e) => e.target.files && send([...e.target.files])}
            />
          </label>
          <label className="button">
            Choisir des fichiers
            <input
              type="file"
              multiple
              accept=".rec"
              hidden
              disabled={sending}
              onChange={(e) => e.target.files && send([...e.target.files])}
            />
          </label>
        </div>
      </div>

      {phase.kind === "sending" && (
        <div className="upload-progress" role="status">
          <div className="rate-track">
            <span className="rate-fill" style={{ width: `${(100 * phase.sent) / Math.max(1, phase.total)}%` }} />
          </div>
          <span className="muted">
            Envoi et analyse : {Math.round(phase.sent / 1e6)} / {Math.round(phase.total / 1e6)} Mo
          </span>
        </div>
      )}

      {phase.kind === "done" && (
        <div className="upload-result" role="status">
          {results.length === 0 && !error && <p className="muted">Aucun fichier .rec trouvé.</p>}
          {results.length > 0 && (
            <p>
              <b>{count("added")}</b> round{count("added") > 1 ? "s" : ""} ajouté{count("added") > 1 ? "s" : ""}
              {count("already") > 0 && <>, {count("already")} déjà présent{count("already") > 1 ? "s" : ""}</>}
              {count("ignored") > 0 && (
                <>
                  , {count("ignored")} écarté{count("ignored") > 1 ? "s" : ""} (ranked, partie rapide ou arcade)
                </>
              )}
              {count("invalid") > 0 && <>, {count("invalid")} illisible{count("invalid") > 1 ? "s" : ""}</>}
              {ignored > 0 && (
                <span className="muted">
                  {" "}
                  ({ignored} autre{ignored > 1 ? "s" : ""} fichier{ignored > 1 ? "s" : ""} ignoré{ignored > 1 ? "s" : ""})
                </span>
              )}
              .
            </p>
          )}
          {count("invalid") > 0 && (
            <ul className="muted">
              {results
                .filter((r) => r.status === "invalid")
                .map((r, i) => (
                  <li key={i}>
                    {r.name} : {r.error}
                  </li>
                ))}
            </ul>
          )}
          {error && <p className="error">L'envoi s'est arrêté : {error}. Les fichiers déjà envoyés sont gardés.</p>}
          {count("added") > 0 && (
            <p>
              <Link to="/matches">Voir les matchs</Link>
            </p>
          )}
        </div>
      )}
    </>
  );
}
