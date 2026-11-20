package stats

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pdrillin/stratdata/match"
	"github.com/pdrillin/stratdata/rec"
)

// Team 0 is "A" with players a1..a5, team 1 is "B" with b1..b5.
func roster() []rec.Player {
	var ps []rec.Player
	for _, n := range []string{"a1", "a2", "a3", "a4", "a5"} {
		ps = append(ps, rec.Player{Name: n, Team: 0, Side: rec.Attack})
	}
	for _, n := range []string{"b1", "b2", "b3", "b4", "b5"} {
		ps = append(ps, rec.Player{Name: n, Team: 1, Side: rec.Defense})
	}
	return ps
}

// kill builds a kill at the given second; team is read from the name.
func kill(sec int, killer, victim string) rec.Kill {
	team := func(n string) int {
		if n == "" {
			return -1
		}
		return int(n[0] - 'a')
	}
	return rec.Kill{
		Clock:  rec.Clock{Phase: rec.PhaseAction, ElapsedMs: sec * 1000, RemainingMs: (180 - sec) * 1000},
		Killer: killer, KillerTeam: team(killer),
		Victim: victim, VictimTeam: team(victim),
	}
}

func compute(rounds ...*rec.Round) *Report {
	m := &match.Match{Map: "Chalet", Teams: [2]string{"A", "B"}}
	for i, r := range rounds {
		r.Complete = true
		m.Rounds = append(m.Rounds, match.RoundFile{Header: rec.Header{RoundNumber: i, Players: roster()}, Round: r})
	}
	return Compute(Options{}, m)
}

func player(t *testing.T, r *Report, name string) *Player {
	t.Helper()
	for _, p := range r.Players {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("player %s missing from report", name)
	return nil
}

func TestOpening(t *testing.T) {
	r := compute(
		&rec.Round{Winner: 0, Kills: []rec.Kill{kill(10, "a1", "b1"), kill(20, "b2", "a2")}},
		&rec.Round{Winner: 1, Kills: []rec.Kill{kill(10, "a1", "b1")}},
		&rec.Round{Winner: 1, Kills: []rec.Kill{kill(10, "b1", "a1")}},
		&rec.Round{Winner: 0}, // nobody dies
	)
	if got, want := player(t, r, "a1").OpeningKills, (Record{2, 1}); got != want {
		t.Errorf("a1 opening kills = %+v, want %+v", got, want)
	}
	if got, want := player(t, r, "a1").OpeningDeaths, (Record{1, 0}); got != want {
		t.Errorf("a1 opening deaths = %+v, want %+v", got, want)
	}
	if got, want := player(t, r, "b1").OpeningDeaths, (Record{2, 1}); got != want {
		t.Errorf("b1 opening deaths = %+v, want %+v", got, want)
	}
	if got := player(t, r, "b2").OpeningKills; got != (Record{}) {
		t.Errorf("b2 got the second kill, opening kills = %+v", got)
	}
	a, b := r.Teams[0], r.Teams[1]
	if a.OpeningKills != (Record{2, 1}) || a.OpeningDeaths != (Record{1, 0}) || b.OpeningKills != (Record{1, 1}) {
		t.Errorf("team openings: A %+v/%+v, B %+v", a.OpeningKills, a.OpeningDeaths, b.OpeningKills)
	}
	if a.Rounds != (Record{4, 2}) || r.Rounds != 4 {
		t.Errorf("A rounds = %+v of %d", a.Rounds, r.Rounds)
	}
}

func TestOpeningTeamKill(t *testing.T) {
	// a1 kills a teammate first: an opening death for A, no opening kill
	// for anyone, and not a kill for a1.
	r := compute(&rec.Round{Winner: 1, Kills: []rec.Kill{kill(5, "a1", "a2"), kill(9, "b1", "a1")}})
	if p := player(t, r, "a1"); p.Kills != 0 || p.OpeningKills.Count != 0 {
		t.Errorf("a1 credited for a team kill: %+v", p)
	}
	if got := player(t, r, "a2").OpeningDeaths; got != (Record{1, 0}) {
		t.Errorf("a2 opening deaths = %+v", got)
	}
	if got := player(t, r, "b1").OpeningKills; got.Count != 0 {
		t.Errorf("b1 opening kills = %+v, the first kill was not theirs", got)
	}
	if r.Teams[1].OpeningKills.Count != 0 || r.Teams[0].OpeningDeaths != (Record{1, 0}) {
		t.Errorf("team openings: B kills %+v, A deaths %+v", r.Teams[1].OpeningKills, r.Teams[0].OpeningDeaths)
	}
}

func TestTrades(t *testing.T) {
	r := compute(&rec.Round{Winner: 0, Kills: []rec.Kill{
		kill(10, "b1", "a1"), // traded: b1 dies 3 s later to a2
		kill(11, "b1", "a3"), // traded by the same kill
		kill(13, "a2", "b1"), // one trade kill, although it avenges two
		kill(30, "b2", "a2"), // not traded: b2 dies 6 s later
		kill(36, "a4", "b2"),
		kill(50, "b3", "a4"), // not traded: b3 is killed by nobody
		kill(52, "", "b3"),
	}})
	for name, want := range map[string]int{"a1": 1, "a3": 1, "a2": 0, "a4": 0, "b1": 0, "b2": 0} {
		if got := player(t, r, name).TradedDeaths; got != want {
			t.Errorf("%s traded deaths = %d, want %d", name, got, want)
		}
	}
	for name, want := range map[string]int{"a2": 1, "a4": 0} {
		if got := player(t, r, name).TradeKills; got != want {
			t.Errorf("%s trade kills = %d, want %d", name, got, want)
		}
	}
	if a := r.Teams[0]; a.Deaths != 4 || a.TradedDeaths != 2 {
		t.Errorf("team A: %d deaths, %d traded, want 4 and 2", a.Deaths, a.TradedDeaths)
	}
}

func TestTradeWindowOption(t *testing.T) {
	m := &match.Match{Teams: [2]string{"A", "B"}, Rounds: []match.RoundFile{{
		Header: rec.Header{Players: roster()},
		Round:  &rec.Round{Winner: 0, Kills: []rec.Kill{kill(30, "b2", "a2"), kill(36, "a4", "b2")}},
	}}}
	if got := player(t, Compute(Options{TradeWindowMs: 10000}, m), "a2").TradedDeaths; got != 1 {
		t.Errorf("with a 10 s window, a2 traded deaths = %d, want 1", got)
	}
}

func TestClutches(t *testing.T) {
	r := compute(
		// a5 is left alone against three and wins. b5 ends up alone
		// against a5 and loses that 1v1.
		&rec.Round{Winner: 0, Kills: []rec.Kill{
			kill(10, "b1", "a1"), kill(20, "b1", "a2"), kill(30, "a3", "b1"), kill(40, "b2", "a3"),
			kill(50, "a4", "b2"), kill(60, "b3", "a4"),
			kill(70, "a5", "b3"), kill(80, "a5", "b4"), kill(90, "a5", "b5"),
		}},
		// a5 alone against five, loses on time without dying.
		&rec.Round{Winner: 1, Kills: []rec.Kill{
			kill(10, "b1", "a1"), kill(20, "b1", "a2"), kill(30, "b1", "a3"), kill(40, "b1", "a4"),
		}},
	)
	want := map[int]*Record{3: {1, 1}, 5: {1, 0}}
	if got := player(t, r, "a5").Clutches; !reflect.DeepEqual(got, want) {
		t.Errorf("a5 clutches = %v, want 1v3 won and 1v5 lost", fmtClutches(got))
	}
	if got := player(t, r, "b5").Clutches; !reflect.DeepEqual(got, map[int]*Record{1: {1, 0}}) {
		t.Errorf("b5 clutches = %v, want one lost 1v1", fmtClutches(got))
	}
	if got := player(t, r, "a4").Clutches; len(got) != 0 {
		t.Errorf("a4 was never alone, clutches = %v", fmtClutches(got))
	}
}

func fmtClutches(m map[int]*Record) map[int]Record {
	out := map[int]Record{}
	for k, v := range m {
		out[k] = *v
	}
	return out
}

func TestSituations(t *testing.T) {
	r := compute(
		&rec.Round{Winner: 0, Kills: []rec.Kill{kill(10, "a1", "b1"), kill(20, "a1", "b2")}},
		&rec.Round{Winner: 1, Kills: []rec.Kill{kill(10, "a1", "b1"), kill(20, "b2", "a1")}},
	)
	a, b := r.Teams[0], r.Teams[1]
	for s, want := range map[Situation]Record{{5, 4}: {2, 1}, {5, 3}: {1, 1}, {4, 4}: {1, 0}} {
		if got := a.Situations[s]; got == nil || *got != want {
			t.Errorf("A in %dv%d = %+v, want %+v", s.Own, s.Enemy, got, want)
		}
	}
	if got := b.Situations[Situation{4, 5}]; got == nil || *got != (Record{2, 1}) {
		t.Errorf("B in 4v5 = %+v, want {2 1}", got)
	}
	if len(a.Situations) != 3 {
		t.Errorf("A went through %d situations, want 3", len(a.Situations))
	}
}

func TestSituationsJSON(t *testing.T) {
	r := compute(&rec.Round{Winner: 0, Kills: []rec.Kill{kill(10, "a1", "b1"), kill(20, "b2", "a1")}})
	b, err := json.Marshal(r.Teams[0])
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Name       string `json:"name"`
		Situations []struct {
			Own, Enemy, Count, Won int
		} `json:"situations"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	// 5v4 then 4v4, biggest advantage first.
	want := []struct{ Own, Enemy, Count, Won int }{{5, 4, 1, 1}, {4, 4, 1, 1}}
	if got.Name != "A" || !reflect.DeepEqual(got.Situations, want) {
		t.Errorf("got %s %+v, want A %+v", got.Name, got.Situations, want)
	}
}

func TestTiming(t *testing.T) {
	prep := kill(0, "a2", "b2")
	prep.Phase, prep.ElapsedMs = rec.PhasePrep, -5000
	late := kill(200, "b3", "a3") // after a plant
	r := compute(&rec.Round{Winner: 0, Kills: []rec.Kill{prep, kill(10, "a1", "b1"), kill(45, "a1", "b4"), late}})
	for _, c := range []struct {
		name          string
		kills, deaths []int
	}{
		{"a1", []int{0, 1, 1, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0, 0, 0}},
		{"a2", []int{1, 0, 0, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0, 0, 0}},
		{"a3", []int{0, 0, 0, 0, 0, 0, 0, 0}, []int{0, 0, 0, 0, 0, 0, 0, 1}},
		{"b4", []int{0, 0, 0, 0, 0, 0, 0, 0}, []int{0, 0, 1, 0, 0, 0, 0, 0}},
	} {
		p := player(t, r, c.name)
		if !reflect.DeepEqual(p.KillTiming, c.kills) || !reflect.DeepEqual(p.DeathTiming, c.deaths) {
			t.Errorf("%s timing = %v / %v, want %v / %v", c.name, p.KillTiming, p.DeathTiming, c.kills, c.deaths)
		}
	}
}

func TestUnknownWinnerSkipped(t *testing.T) {
	r := compute(&rec.Round{Winner: -1, Kills: []rec.Kill{kill(10, "a1", "b1")}})
	if r.Rounds != 0 || r.SkippedRounds != 1 || len(r.Players) != 0 {
		t.Errorf("rounds %d, skipped %d, players %d; want 0, 1, 0", r.Rounds, r.SkippedRounds, len(r.Players))
	}
}

func TestSites(t *testing.T) {
	const office, bar = "2F Master Bedroom / 2F Office", "1F Bar / 1F Gaming Room"
	plant := &rec.Clock{Phase: rec.PhaseAction, ElapsedMs: 90000}
	r := compute(
		// A attacks (see roster) in every round.
		&rec.Round{Winner: 0, Site: office, Plant: plant, Planter: "a2"},
		&rec.Round{Winner: 1, Site: office, Plant: plant, Planter: "a2"},
		&rec.Round{Winner: 1, Site: office},
		&rec.Round{Winner: 0, Site: bar},
		&rec.Round{Winner: 0}, // site unknown: counted in rounds, not in sites
	)
	a, b := r.Teams[0], r.Teams[1]
	if len(a.Sites) != 2 || len(b.Sites) != 2 {
		t.Fatalf("sites: A %d, B %d, want 2 each", len(a.Sites), len(b.Sites))
	}
	// Sorted by site name: bar first.
	if s := a.Sites[1]; s.Site != office || s.Attack != (Record{3, 1}) || s.Plants != (Record{2, 1}) || s.Defense != (Record{}) {
		t.Errorf("A on %s: %+v", office, *s)
	}
	if s := b.Sites[1]; s.Defense != (Record{3, 2}) || s.Attack != (Record{}) || s.Plants != (Record{}) {
		t.Errorf("B on %s: %+v", office, *s)
	}
	if s := a.Sites[0]; s.Site != bar || s.Map != "Chalet" || s.Attack != (Record{1, 1}) {
		t.Errorf("A on %s: %+v", bar, *s)
	}
	if got := player(t, r, "a2").Plants; got != (Record{2, 1}) {
		t.Errorf("a2 plants = %+v, want 2 plants, 1 won", got)
	}
	if a.Rounds.Count != 5 {
		t.Errorf("A rounds = %d, want 5", a.Rounds.Count)
	}
}

func TestRooms(t *testing.T) {
	at := func(k rec.Kill, killerRoom, victimRoom string) rec.Kill {
		k.KillerRoom, k.VictimRoom = killerRoom, victimRoom
		return k
	}
	r := compute(
		&rec.Round{Winner: 0, Kills: []rec.Kill{
			at(kill(10, "a1", "b1"), "EXT Front Yard", "2F Office"),
			at(kill(20, "b2", "a2"), "2F Office", "2F Office"),
			at(kill(30, "a1", "a3"), "2F Office", "2F Piano Room"), // team kill: a death, no kill
			at(kill(40, "", "b3"), "", "2F Office"),
			kill(50, "a1", "b4"), // rooms unknown
		}},
	)
	rooms := map[string]Room{}
	for _, rm := range r.Rooms {
		if rm.Map != "Chalet" {
			t.Errorf("room %s on map %q", rm.Room, rm.Map)
		}
		rooms[rm.Room] = *rm
	}
	want := map[string]Room{
		"2F Office":      {Map: "Chalet", Room: "2F Office", Attack: RoomCount{Deaths: 1}, Defense: RoomCount{Kills: 1, Deaths: 2}},
		"EXT Front Yard": {Map: "Chalet", Room: "EXT Front Yard", Attack: RoomCount{Kills: 1}},
		"2F Piano Room":  {Map: "Chalet", Room: "2F Piano Room", Attack: RoomCount{Deaths: 1}},
	}
	if !reflect.DeepEqual(rooms, want) {
		t.Errorf("rooms = %+v\nwant %+v", rooms, want)
	}
	if r.Rooms[0].Room != "2F Office" {
		t.Errorf("rooms not sorted by deaths: first is %s", r.Rooms[0].Room)
	}
	if a1 := player(t, r, "a1"); !reflect.DeepEqual(a1.KillRooms, map[string]int{"EXT Front Yard": 1}) {
		t.Errorf("a1 kill rooms = %v", a1.KillRooms)
	}
	if b1 := player(t, r, "b1"); !reflect.DeepEqual(b1.DeathRooms, map[string]int{"2F Office": 1}) {
		t.Errorf("b1 death rooms = %v", b1.DeathRooms)
	}
}

func TestKOST(t *testing.T) {
	r := compute(
		// a1 kills, a2 dies but is traded by a3, a4 plants then dies
		// untraded, a5 survives without doing anything. b1 kills a4 then
		// dies to a team kill, b2 kills a2 then dies, b3 dies without a
		// kill or a trade, b4 and b5 survive.
		&rec.Round{Winner: 0, Planter: "a4", Plant: &rec.Clock{}, Kills: []rec.Kill{
			kill(10, "a1", "b3"),
			kill(20, "b2", "a2"),
			kill(22, "a3", "b2"),
			kill(30, "b1", "a4"), // b1 kills a4, nobody avenges within 5 s
			kill(60, "b4", "b1"), // team kill
		}},
	)
	want := map[string]int{"a1": 1, "a2": 1, "a3": 1, "a4": 1, "a5": 1, "b1": 1, "b2": 1, "b3": 0, "b4": 1, "b5": 1}
	for name, k := range want {
		if got := player(t, r, name).KOST; got != k {
			t.Errorf("%s KOST = %d, want %d", name, got, k)
		}
	}
	// b1 is in KOST for the kill on a4, not for surviving.
	for name, sv := range map[string]int{"a1": 1, "a2": 0, "a4": 0, "a5": 1, "b1": 0, "b4": 1} {
		if got := player(t, r, name).Survived; got != sv {
			t.Errorf("%s survived = %d, want %d", name, got, sv)
		}
	}

	// A defuse counts as the objective, even for a player who dies after
	// it in the same tick and is not traded.
	r = compute(&rec.Round{Winner: 1, Defuser: "b1", Plant: &rec.Clock{}, Planter: "a1",
		Kills: []rec.Kill{kill(170, "a2", "b1")}})
	if p := player(t, r, "b1"); p.KOST != 1 || p.Defuses != 1 {
		t.Errorf("defuser b1: KOST %d, defuses %d, want 1 and 1", p.KOST, p.Defuses)
	}

	// A round where a player only dies untraded does not count.
	r = compute(&rec.Round{Winner: 1, Kills: []rec.Kill{kill(10, "b1", "a1")}})
	if got := player(t, r, "a1").KOST; got != 0 {
		t.Errorf("untraded death without kill: KOST = %d, want 0", got)
	}
}

func TestAssists(t *testing.T) {
	r := compute(
		&rec.Round{Winner: 0, Scoreboards: map[string]rec.Scoreboard{"a1": {Assists: 2}, "b1": {Assists: 1}}},
		&rec.Round{Winner: 0, Scoreboards: map[string]rec.Scoreboard{"a1": {Assists: 1}}},
		&rec.Round{Winner: 1}, // no scoreboard decoded
	)
	for name, want := range map[string]int{"a1": 3, "b1": 1, "a2": 0} {
		if got := player(t, r, name).Assists; got != want {
			t.Errorf("%s assists = %d, want %d", name, got, want)
		}
	}
}

func TestBreakdowns(t *testing.T) {
	// Round 1: A attacks, a1 plays Ash and kills b1. Round 2: sides
	// swapped, a1 plays Mute and dies. Round 3 is on another map.
	swap := func(ps []rec.Player, op string) []rec.Player {
		out := append([]rec.Player(nil), ps...)
		for i := range out {
			if out[i].Side == rec.Attack {
				out[i].Side = rec.Defense
			} else {
				out[i].Side = rec.Attack
			}
			if out[i].Name == "a1" {
				out[i].Operator = op
			}
		}
		return out
	}
	r1 := roster()
	r1[0].Operator = "ASH"
	chalet := &match.Match{Map: "Chalet", Teams: [2]string{"A", "B"}, Rounds: []match.RoundFile{
		{Header: rec.Header{RoundNumber: 0, Players: r1}, Round: &rec.Round{Winner: 0, Kills: []rec.Kill{kill(10, "a1", "b1")}}},
		{Header: rec.Header{RoundNumber: 1, Players: swap(roster(), "MUTE")}, Round: &rec.Round{Winner: 1, Kills: []rec.Kill{kill(10, "b1", "a1")}}},
	}}
	bank := &match.Match{Map: "Bank", Teams: [2]string{"A", "B"}, Rounds: []match.RoundFile{
		{Header: rec.Header{RoundNumber: 0, Players: r1}, Round: &rec.Round{Winner: 0}},
	}}
	r := Compute(Options{}, chalet, bank)
	a1 := player(t, r, "a1")

	want := map[string]Line{
		"attack":  {Rounds: 2, Won: 2, Kills: 1, KOST: 2, Survived: 2, OpeningKills: 1},
		"defense": {Rounds: 1, Won: 0, Deaths: 1, OpeningDeaths: 1},
	}
	for side, w := range want {
		if got := a1.BySide[side]; got == nil || *got != w {
			t.Errorf("a1 on %s = %+v, want %+v", side, got, w)
		}
	}
	if got := a1.ByMap["Chalet"]; got == nil || got.Rounds != 2 || got.Won != 1 || got.Kills != 1 || got.Deaths != 1 {
		t.Errorf("a1 on Chalet = %+v", got)
	}
	if got := a1.ByMap["Bank"]; got == nil || got.Rounds != 1 || got.Won != 1 {
		t.Errorf("a1 on Bank = %+v", got)
	}
	if got := a1.ByOperator["ASH"]; got == nil || got.Rounds != 2 || got.Kills != 1 {
		t.Errorf("a1 on Ash = %+v", got)
	}
	if got := a1.ByOperator["MUTE"]; got == nil || got.Rounds != 1 || got.Deaths != 1 {
		t.Errorf("a1 on Mute = %+v", got)
	}
	if len(a1.ByOperator) != 2 {
		t.Errorf("a1 operators = %v, want Ash and Mute only", a1.ByOperator)
	}
	if b1 := player(t, r, "b1"); len(b1.ByOperator) != 0 {
		t.Errorf("b1 has no known operator, got %v", b1.ByOperator)
	}

	a, b := r.Teams[0], r.Teams[1]
	if got := a.BySide["attack"]; got == nil || got.Rounds != (Record{2, 2}) || got.OpeningKills != (Record{1, 1}) {
		t.Errorf("A on attack = %+v", got)
	}
	if got := b.BySide["attack"]; got == nil || got.Rounds != (Record{1, 1}) || got.OpeningKills != (Record{1, 1}) {
		t.Errorf("B on attack = %+v", got)
	}
	if got := a.ByMap["Bank"]; got == nil || got.Rounds != (Record{1, 1}) {
		t.Errorf("A on Bank = %+v", got)
	}
}
