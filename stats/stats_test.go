package stats

import (
	"reflect"
	"testing"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
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
	m := &match.Match{Teams: [2]string{"A", "B"}}
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

func TestUnknownWinnerSkipped(t *testing.T) {
	r := compute(&rec.Round{Winner: -1, Kills: []rec.Kill{kill(10, "a1", "b1")}})
	if r.Rounds != 0 || r.SkippedRounds != 1 || len(r.Players) != 0 {
		t.Errorf("rounds %d, skipped %d, players %d; want 0, 1, 0", r.Rounds, r.SkippedRounds, len(r.Players))
	}
}
