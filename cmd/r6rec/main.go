// Command r6rec inspects Rainbow Six Siege replay files.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
	"github.com/pdrillin/r6-stats/stats"
)

const usage = `usage:
  r6rec ls <dir>...             one line per round found under the folders
  r6rec matches <dir>...        rounds grouped into matches, with scoreboards
  r6rec stats [-json] <dir>...  opening duels, trades, clutches and man-advantage
                                conversion over every match under the folders
  r6rec rooms [-n N] <dir>...   kills and deaths by room, N busiest rooms per map
                                (default 15, 0 for all)
  r6rec header [-raw] <file>    header of one round as JSON (-raw: every property)
  r6rec round <file>            events of one round as JSON
  r6rec dump <file> <out>       write the decompressed body to <out>
`

func main() {
	if len(os.Args) < 3 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ls":
		err = list(os.Args[2:])
	case "matches":
		err = matches(os.Args[2:])
	case "stats":
		err = statistics(os.Args[2:])
	case "rooms":
		err = rooms(os.Args[2:])
	case "header":
		err = header(os.Args[2:])
	case "round":
		err = round(os.Args[2])
	case "dump":
		if len(os.Args) != 4 {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = dump(os.Args[2], os.Args[3])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "r6rec:", err)
		os.Exit(1)
	}
}

// find returns every .rec file under the roots, sorted by path.
func find(roots []string) ([]string, error) {
	var paths []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".rec") {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func list(roots []string) error {
	paths, err := find(roots)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "FILE\tVERSION\tMAP\tROUND\tSCORE\tPLAYERS\tCHUNKS\tBODY")
	failed := 0
	for _, p := range paths {
		f, err := rec.Open(p)
		if err != nil {
			failed++
			fmt.Fprintf(w, "%s\tERROR: %v\n", filepath.Base(p), err)
			continue
		}
		h := f.Header
		mapName := h.Map
		if mapName == "" {
			mapName = "#" + h.MapID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s %d-%d %s\t%d\t%d\t%.1f MB\n",
			filepath.Base(p), h.GameVersion, mapName, h.RoundNumber+1,
			h.Teams[0].Name, h.Teams[0].StartingScore, h.Teams[1].StartingScore, h.Teams[1].Name,
			len(h.Players), len(f.Chunks), float64(f.Size())/1e6)
	}
	if failed > 0 {
		w.Flush()
		return fmt.Errorf("%d of %d files could not be read", failed, len(paths))
	}
	return nil
}

func header(args []string) error {
	raw := false
	if args[0] == "-raw" {
		raw = true
		args = args[1:]
	}
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	f, err := rec.Open(args[0])
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if raw {
		return enc.Encode(f.Header.Properties)
	}
	return enc.Encode(f.Header)
}

func dump(in, out string) error {
	f, err := rec.Open(in)
	if err != nil {
		return err
	}
	o, err := os.Create(out)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(o, 1<<20)
	if err := f.WriteBody(w); err != nil {
		o.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

func round(path string) error {
	f, err := rec.Open(path)
	if err != nil {
		return err
	}
	r, err := f.Round()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// load decodes every round under the roots and groups them into matches.
func load(roots []string) ([]*match.Match, error) {
	paths, err := find(roots)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .rec file under %s", strings.Join(roots, ", "))
	}
	var files []match.RoundFile
	for _, p := range paths {
		f, err := rec.Open(p)
		if err != nil {
			return nil, err
		}
		r, err := f.Round()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		files = append(files, match.RoundFile{Path: p, Header: f.Header, Round: r})
	}
	return match.Assemble(files), nil
}

func matches(roots []string) error {
	ms, err := load(roots)
	if err != nil {
		return err
	}
	for i, m := range ms {
		if i > 0 {
			fmt.Println()
		}
		mapName := m.Map
		if mapName == "" {
			mapName = "#" + m.MapID
		}
		score := m.Score()
		fmt.Printf("%s  %s %d-%d %s  (%d rounds, %d superseded recordings)\n",
			mapName, m.Teams[0], score[0], score[1], m.Teams[1], len(m.Rounds), len(m.Superseded))

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  ROUND\tATTACK\tSITE\tWINNER\tHOW\tKILLS\tPLANT\tLENGTH")
		for _, rf := range m.Rounds {
			h, r := rf.Header, rf.Round
			atk, winner, plant := "?", "?", "-"
			for _, p := range h.Players {
				if p.Side == rec.Attack {
					atk = m.Teams[p.Team]
				} else if p.Side == rec.Defense {
					atk = m.Teams[1-p.Team]
				}
			}
			if r.Winner >= 0 {
				winner = m.Teams[r.Winner]
			}
			if r.Plant != nil {
				plant = clock(r.Plant.ElapsedMs)
			}
			site := r.Site
			if site == "" {
				site = "?"
			}
			fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
				h.RoundNumber+1, atk, site, winner, r.WinReason, len(r.Kills), plant, clock(r.End.ElapsedMs))
		}
		w.Flush()

		fmt.Fprintln(w, "  PLAYER\tTEAM\tK\tD\tHS")
		for _, p := range stats.Compute(stats.Options{}, m).Players {
			fmt.Fprintf(w, "  %s\t%s\t%d\t%d\t%d\n", p.Name, p.Team, p.Kills, p.Deaths, p.Headshots)
		}
		w.Flush()
	}
	return nil
}

func statistics(args []string) error {
	asJSON := false
	if args[0] == "-json" {
		asJSON = true
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ms, err := load(args)
	if err != nil {
		return err
	}
	rep := stats.Compute(stats.Options{}, ms...)
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	fmt.Printf("%d matches, %d rounds", len(ms), rep.Rounds)
	if rep.SkippedRounds > 0 {
		fmt.Printf(" (%d more skipped: winner unknown)", rep.SkippedRounds)
	}
	fmt.Printf(". Trade window: %d s.\n", stats.DefaultTradeWindowMs/1000)
	fmt.Println("x/y (z%) reads: y occurrences, x of them in rounds the team won.")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nPLAYER\tTEAM\tRNDS\tK\tD\tOPENING KILLS\tOPENING DEATHS\tTRADE KILLS\tDEATHS TRADED\tCLUTCHES")
	for _, p := range rep.Players {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%s\t%s\t%d\t%s\t%s\n",
			p.Name, p.Team, p.Rounds, p.Kills, p.Deaths,
			record(p.OpeningKills), record(p.OpeningDeaths),
			p.TradeKills, ratio(p.TradedDeaths, p.Deaths), clutches(p.Clutches))
	}
	w.Flush()

	fmt.Fprintln(w, "\nTEAM\tROUNDS WON\tFIRST KILL FOR\tFIRST KILL AGAINST\tDEATHS TRADED")
	for _, t := range rep.Teams {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Name,
			record(t.Rounds), record(t.OpeningKills), record(t.OpeningDeaths), ratio(t.TradedDeaths, t.Deaths))
	}
	w.Flush()

	fmt.Println("\nBOMB SITES (rounds won; PLANTS: attacking rounds with a plant, and won)")
	for _, t := range rep.Teams {
		fmt.Fprintln(w, t.Name+"\tMAP\tSITE\tATTACK\tDEFENSE\tPLANTS")
		for _, s := range t.Sites {
			fmt.Fprintf(w, "\t%s\t%s\t%s\t%s\t%s\n", s.Map, s.Site, record(s.Attack), record(s.Defense), record(s.Plants))
		}
	}
	w.Flush()

	fmt.Println("\nROUNDS WON BY PLAYERS ALIVE (own v enemy)")
	var all []stats.Situation
	seen := map[stats.Situation]bool{}
	for _, t := range rep.Teams {
		for s := range t.Situations {
			if !seen[s] {
				seen[s] = true
				all = append(all, s)
			}
		}
	}
	// Biggest advantage first, then most players.
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Own-a.Enemy != b.Own-b.Enemy {
			return a.Own-a.Enemy > b.Own-b.Enemy
		}
		return a.Own > b.Own
	})
	fmt.Fprint(w, "TEAM")
	for _, s := range all {
		fmt.Fprintf(w, "\t%dv%d", s.Own, s.Enemy)
	}
	fmt.Fprintln(w)
	for _, t := range rep.Teams {
		fmt.Fprint(w, t.Name)
		for _, s := range all {
			if r := t.Situations[s]; r != nil {
				fmt.Fprintf(w, "\t%d/%d", r.Won, r.Count)
			} else {
				fmt.Fprint(w, "\t-")
			}
		}
		fmt.Fprintln(w)
	}
	return w.Flush()
}

// record formats a Record as "won/count (pct%)".
func record(r stats.Record) string { return ratio(r.Won, r.Count) }

func ratio(n, of int) string {
	if of == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d (%d%%)", n, of, (100*n+of/2)/of)
}

// clutches formats clutch records as "1v2 1/3", smallest first.
func clutches(m map[int]*stats.Record) string {
	if len(m) == 0 {
		return "-"
	}
	sizes := make([]int, 0, len(m))
	for x := range m {
		sizes = append(sizes, x)
	}
	sort.Ints(sizes)
	parts := make([]string, len(sizes))
	for i, x := range sizes {
		parts[i] = fmt.Sprintf("1v%d %d/%d", x, m[x].Won, m[x].Count)
	}
	return strings.Join(parts, ", ")
}

// clock formats a duration in milliseconds as m:ss.
func clock(ms int) string {
	s := ms / 1000
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func rooms(args []string) error {
	limit := 15
	if len(args) >= 2 && args[0] == "-n" {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 0 {
			return fmt.Errorf("-n wants a number, got %q", args[1])
		}
		limit, args = n, args[2:]
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ms, err := load(args)
	if err != nil {
		return err
	}
	rep := stats.Compute(stats.Options{}, ms...)
	fmt.Println("Kills are counted in the killer's room, deaths in the victim's.")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	shown, cur := 0, ""
	for _, r := range rep.Rooms {
		if r.Map != cur {
			cur, shown = r.Map, 0
			fmt.Fprintf(w, "\n%s\tATK KILLS\tATK DEATHS\tDEF KILLS\tDEF DEATHS\n", r.Map)
		}
		if limit > 0 && shown == limit {
			continue
		}
		shown++
		fmt.Fprintf(w, "  %s\t%d\t%d\t%d\t%d\n", r.Room, r.Attack.Kills, r.Attack.Deaths, r.Defense.Kills, r.Defense.Deaths)
	}
	return w.Flush()
}
