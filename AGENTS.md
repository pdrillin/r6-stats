# AGENTS.md

Guide for coding agents working on this repository. Read it before changing anything.

## Project

Advanced match statistics for Rainbow Six Siege, computed from the game's replay files (`.rec`). The goal is to go beyond public trackers (K/D, win rate) by using the order and timing of events in each round.

Planned product, not built yet:

- **Windows app** (players are on Windows): watches the game's `MatchReplay` folder and uploads new matches before the game purges them. Planned with Wails (Go backend + web UI in WebView2).
- **Server**: receives raw `.rec` files, parses them, stores stats (Postgres) and keeps the raw files (object storage) so they can be re-parsed when the parser improves.
- **Web UI**: React + TypeScript + Vite SPA, shared between the website and the Windows app.

Parsing happens server-side so a season change only needs a server update. What exists today is the Go parser, match assembly, stats and a CLI.

## Working with the user

- The user writes in French. Answer in French. Code, identifiers and code comments stay in English.
- Development happens on Linux; the target users are on Windows. Cross-compile with `GOOS=windows GOARCH=amd64 go build ./cmd/r6rec`.
- Older seasons are explicitly out of scope for now: target the current season (Y11S2) and the following ones.

## Commands

```sh
go vet ./...
go test ./...                        # unit tests + corpus tests if Replays/ exists
go test -short ./...                 # skips tests that decompress the whole corpus
go run ./cmd/r6rec ls Replays        # one line per round
go run ./cmd/r6rec matches Replays   # assembled matches, round by round
go run ./cmd/r6rec stats Replays     # stats report (-json for JSON)
go run ./cmd/r6rec round <file.rec>  # decoded events of one round
go run ./cmd/r6rec header -raw <file.rec>
go run ./cmd/r6rec dump <file.rec> <out.bin>  # decompressed body, for reverse engineering
```

CI (`.github/workflows/test.yml`) runs gofmt, vet, tests and the Windows build. Keep `gofmt -l .` empty.

## Replay corpus

`Replays/` is git-ignored: ~580 MB of pro esports replays (EWC 2026, 15 Aug 2026, game version `Y11S2_Alpha04`), not redistributable. On a new machine, copy it over by hand to the repo root. Without it, corpus tests skip and only unit tests run.

Layout: `Replays/<series>/<n> - <map>/Match-<date>-<id>/Match-...-RNN.rec`, one file per round. Two series:

- DarkZero vs Geekay: Nighthaven Labs 3-7, Kafe 8-7, Chalet 7-0.
- FURIA vs FaZe (grand final): Bank 3-7, Border 5-7, Fortress 8-6, Lair 5-7. These four scores match public results.

`match/match_test.go` pins those scores; `rec/rec_test.go` checks every round winner against the next round's starting score (72/72).

## Code layout

| Package | Role |
|---|---|
| `rec/` | File format: header (`rec.go`), map ids (`maps.go`), body events and round winner (`round.go`) |
| `match/` | Groups round files into matches, handles rehosts, `Roster` fills a missing team |
| `stats/` | Opening duels, trades, clutches, man-advantage conversion |
| `cmd/r6rec/` | CLI |

## File format (reverse engineered, Y11S2)

All integers little-endian unless noted.

**Container**
- Magic `dissect\0`, then a preamble (u32, string, u32, u32), then u64 property count and that many (string key, string value) pairs. Strings are u64 length + bytes.
- Then an undecoded index region, then chunks: tag `200VRPMC`, u32 decompressed size, u32 compressed size, one zstd frame. Concatenated chunks form the body (40–90 MB per round).

**Header properties** (keys repeat; order matters)
- Player blocks start at `playerid`: `profileid`, `playername`, `team` (index into teams), `heroname`, `alliance`, `roleimage`, `rolename` (operator display name), `roleportrait`.
- `alliance`: 3 = attack, 4 = defense (verified against operator picks on both halves).
- Match: `version`, `code`, `datetime`, `id` (match id, changes on rehost), `matchtype` (3 in all corpus files; ranked value unknown), `worldid` (map id, see `rec/maps.go`), `gamemodeid`, `roundnumber` (0-based), `teamname0/1`, `startingteamscore0/1` (score *before* the round), `maxnbplayersperteam`, many `gmsetting`.
- The header does not contain the round result, nor the bomb site.
- Tournament replays: `profileid` is zeroed, `isspectator` = 1. Ranked replays will differ; none have been examined yet.

**Body**
- A stream of object updates: `0x23` + 8-byte object id, then properties. Each property is a 4-byte key (hash, read big-endian in code), a 1-byte length, the value. Properties after the first are prefixed `0x22`, or `0x26` when followed by a 4-byte array index before the length.
- Round state object (found via its first key `0x1f07efc9`): `0x1f07efc9` timer seconds, `0x1837466c` timer ms, `0x35f88fcf` alert flag (plant, or last 10 s), `0xbb094fd3` round state (3 = over).
- Kill feed entry (first key `0x5934e58b`): `0xd9133cba` killer, `0xac190f70` victim, `0xc198de70` / `0x782e7b50` killer/victim team + 1, `0xc35ba44e` headshot. Entries are re-sent when the feed redraws: dedupe by object id. Entries without a victim are phase announcements.
- Room names (`"2F Library Stairs"`, key `0xaf9899ca` indexed) appear in the body; not yet linked to players.
- Events get their time from the last timer update preceding them in the body (the body is chronological).

**Known pitfalls**
- The round winner is inferred from rules (wipe, plant + timeout, plant + no timeout = defuse, timeout), not read from the file.
- A plant is detected by the timer jumping up (defuser countdown) or the alert flag with more than 11.5 s left.
- When a plant in progress extends the round past 0:00, the timer freezes under 1 s, so late events read as 2:59.
- A recording made right after a rehost starts in the "over" state, then leaves it.
- Rehosts: same map + teams; a repeated round number replaces the earlier (incomplete) recording. Some post-rehost files list only one team in the header; `match.Roster` borrows the other team from a neighbouring round.
- Expect keys and layouts to change between seasons. Plan: dispatch on `version`/`code`, keep a test corpus per season, flag matches that fail consistency checks instead of showing wrong stats.

## Stats definitions (`stats/`)

- Opening: first kill of the round. A team kill first is an opening death for that team and nobody's opening kill.
- Trade: the killer dies to the victim's team within `TradeWindowMs` (default 5000). One kill avenging several deaths is one trade kill.
- Clutch: player left alone on their team; size = enemies alive at that moment; won if the team wins the round.
- Situations: each NvM state counted once per round, with rounds won.
- Players are keyed by profile id when present, by name otherwise. Rounds with unknown winner are skipped and counted in `SkippedRounds`.

## Next steps

1. Decode the bomb site, then the planter/defuser (unlocks per-site stats and full KOST).
2. Per-side, per-map and per-operator breakdowns of the existing stats.
3. Get ranked replays from the user's PC to check `matchtype`, profile ids and differences from tournament files.
4. Then the server, web UI and Windows app.

Always show sample sizes next to percentages: clutches and rare situations rest on few cases.
