package rec

import (
	"bytes"
	"encoding/binary"
	"sort"
	"strings"
)

// The body is a stream of object updates. An update is an object reference
// (tagObject, 8-byte object id) followed by properties: a 4-byte key, a 1-byte
// length and the value. Every property after the first is introduced by
// tagProp, or by tagIndexed when it carries a 4-byte array index before the
// length. Keys are opaque hashes; the ones below were identified by comparing
// rounds of game version Y11S2.
const (
	tagObject  = 0x23
	tagProp    = 0x22
	tagIndexed = 0x26
)

const (
	// Round state object.
	keyTimerSeconds = 0x1f07efc9 // u32, HUD timer in seconds; identifies the object
	keyTimerMillis  = 0x1837466c // u32, same timer in milliseconds
	keyTimerAlert   = 0x35f88fcf // bool, set when the defuser is planted or 10 s remain
	keyRoundState   = 0xbb094fd3 // u32, 3 once the round is over

	// Kill feed entry object.
	keyFeedEntry  = 0x5934e58b // u32, first property of every feed entry
	keyKiller     = 0xd9133cba // string
	keyKillerTeam = 0xc198de70 // u32, team index + 1
	keyVictim     = 0xac190f70 // string
	keyVictimTeam = 0x782e7b50 // u32, team index + 1
	keyHeadshot   = 0xc35ba44e // bool

	// HUD label objects. The same key holds room names (player locations,
	// spawn points) and bomb site labels.
	keyLabel = 0xaf9899ca // string
	// keySiteSlot is 1 on the label of the current round's bomb site. A
	// second label object can hold an older site with slot 2.
	keySiteSlot = 0x2ca42af6 // u32
)

const roundStateOver = 3

// Phase is a stage of a round.
type Phase string

const (
	PhasePrep      Phase = "prep"
	PhaseAction    Phase = "action"
	PhasePostPlant Phase = "postplant"
)

// WinReason says how a round was won.
type WinReason string

const (
	AttackersEliminated WinReason = "attackers_eliminated"
	DefendersEliminated WinReason = "defenders_eliminated"
	DefuserDetonated    WinReason = "defuser_detonated"
	DefuserDisabled     WinReason = "defuser_disabled"
	TimeExpired         WinReason = "time_expired"
)

// Clock locates an event in a round.
type Clock struct {
	Phase Phase `json:"phase"`
	// RemainingMs is the HUD timer: time left in the phase.
	RemainingMs int `json:"remainingMs"`
	// ElapsedMs is the time since the action phase started; negative during
	// the preparation phase. It is derived from the timer, which stops just
	// under one second when a plant in progress keeps the round alive, so
	// anything that happens during that extension reads as 2:59.
	ElapsedMs int `json:"elapsedMs"`
}

// Kill is one kill feed entry.
type Kill struct {
	Clock
	Killer     string `json:"killer"` // empty when no player is credited
	KillerTeam int    `json:"killerTeam"`
	Victim     string `json:"victim"`
	VictimTeam int    `json:"victimTeam"`
	Headshot   bool   `json:"headshot"`
	// KillerRoom and VictimRoom are where each body was at the time of the
	// kill. Empty when unknown.
	KillerRoom string `json:"killerRoom"`
	VictimRoom string `json:"victimRoom"`
}

// Round is what happened during one round file.
type Round struct {
	// Complete is false when the recording stops before the round ends, as
	// happens when a private match is rehosted.
	Complete bool   `json:"complete"`
	Kills    []Kill `json:"kills"`
	// Plant is when the defuser was planted, nil if it never was.
	Plant *Clock `json:"plant"`
	// Planter is the player credited with the plant on the scoreboard.
	Planter string `json:"planter,omitempty"`
	// Scoreboards holds, by player name, what each player's scoreboard
	// counters gained during the round. Assists are only known from here.
	Scoreboards map[string]Scoreboard `json:"scoreboards"`
	// End is the last timer value before the round was over. Zero value when
	// the round is not complete.
	End Clock `json:"end"`
	// Winner is the index into Header.Teams, or -1 when it cannot be told.
	Winner    int       `json:"winner"`
	WinReason WinReason `json:"winReason,omitempty"`
	// Site is the bomb site defended this round, as the two rooms the game
	// names it by, e.g. "2F Master Bedroom / 2F Office". Empty if not found.
	Site string `json:"site"`
	// Positions lists, per player name, every room the player's body
	// entered while alive and deployed.
	Positions map[string][]Position `json:"positions"`
}

// Body decompresses the whole replay body.
func (f *File) Body() ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(f.Size())
	if err := f.WriteBody(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Round decodes the events of the round. It decompresses the body.
func (f *File) Round() (*Round, error) {
	body, err := f.Body()
	if err != nil {
		return nil, err
	}
	return decodeRound(&f.Header, body), nil
}

// tick is one timer reading with its position in the body.
type tick struct {
	off int
	Clock
}

func decodeRound(h *Header, body []byte) *Round {
	r := &Round{Kills: []Kill{}, Winner: -1}
	ticks, endOff := readTimeline(body, r)

	// The body is chronological, so an event happened at the last timer
	// reading that precedes it.
	clockAt := func(off int) Clock {
		i := sort.Search(len(ticks), func(i int) bool { return ticks[i].off > off })
		if i == 0 {
			return Clock{}
		}
		return ticks[i-1].Clock
	}
	if r.Complete {
		r.End = clockAt(endOff)
	}
	kills, offs := readKills(body, clockAt)
	r.Kills = kills
	r.Site = readSite(body)

	players := readPlayerObjects(body)
	// The scoreboard credits the plant even when the round ends before the
	// timer switches to the defuser countdown; the plant then happened at
	// the credit.
	r.Scoreboards = readScoreboards(body, players)
	planter, plantOff := readPlanter(body, players)
	if planter != "" {
		r.Planter = planter
		if r.Plant == nil {
			c := clockAt(plantOff)
			r.Plant = &c
		}
	}
	tr := readTracks(body, players)
	r.Positions = map[string][]Position{}
	for _, p := range players {
		if pos := tr.positions(p.name, clockAt); len(pos) > 0 {
			r.Positions[p.name] = pos
		}
	}
	// Room updates sent in the same tick as a kill can arrive just after it.
	tickEnd := func(off int) int {
		i := sort.Search(len(ticks), func(i int) bool { return ticks[i].off > off })
		if i == len(ticks) {
			return len(body)
		}
		return ticks[i].off
	}
	for i := range r.Kills {
		k := &r.Kills[i]
		end := tickEnd(offs[i])
		if k.Killer != "" {
			k.KillerRoom = tr.roomAt(k.Killer, offs[i], end)
		}
		k.VictimRoom = tr.roomAt(k.Victim, offs[i], end)
	}
	r.Winner, r.WinReason = decideWinner(h, r)
	return r
}

// readTimeline walks the updates of the round state object. It returns the
// timer readings, fills r.Plant and r.Complete, and returns where the round
// ended.
func readTimeline(body []byte, r *Round) (ticks []tick, endOff int) {
	ref := roundObjectRef(body)
	if ref == nil {
		return nil, 0
	}
	var (
		phase       = PhasePrep
		prev        = -1 // previous timer reading, ms
		actionStart int  // timer value when the action phase began
		plantAt     int  // elapsed ms when the defuser was planted
		postStart   int  // timer value when the post-plant countdown began
	)
	for pos := 0; ; {
		i := bytes.Index(body[pos:], ref)
		if i < 0 {
			break
		}
		off := pos + i
		pos = off + len(ref)

		ms, alert := -1, false
		readProps(body, pos, func(key uint32, val []byte) {
			switch {
			case key == keyTimerMillis && len(val) == 4:
				ms = int(binary.LittleEndian.Uint32(val))
			case key == keyTimerAlert && len(val) == 1:
				alert = val[0] == 1
			case key == keyRoundState && len(val) == 4:
				// A recording made right after a rehost opens in the
				// "over" state of the lobby, then leaves it.
				over := binary.LittleEndian.Uint32(val) == roundStateOver
				if over && !r.Complete {
					endOff = off
				}
				r.Complete = over
			}
		})
		// The timer is reset to zero when the round is over; that reading
		// is not a moment of the round.
		if ms <= 0 || r.Complete {
			continue
		}

		jumpedUp := prev >= 0 && ms > prev+500
		switch phase {
		case PhasePrep:
			if jumpedUp {
				phase, actionStart = PhaseAction, ms
			}
		case PhaseAction:
			// Planting replaces the round timer with the defuser countdown.
			// The alert flag is raised in the same update; on its own it
			// also marks the last 10 seconds, hence the threshold.
			if jumpedUp || (alert && ms > 11500) {
				plantAt = actionStart - prev
				r.Plant = &Clock{Phase: PhaseAction, RemainingMs: prev, ElapsedMs: plantAt}
				phase, postStart = PhasePostPlant, ms
			}
		}
		c := Clock{Phase: phase, RemainingMs: ms}
		switch phase {
		case PhasePrep:
			c.ElapsedMs = -ms
		case PhaseAction:
			c.ElapsedMs = actionStart - ms
		case PhasePostPlant:
			c.ElapsedMs = plantAt + postStart - ms
		}
		ticks = append(ticks, tick{off, c})
		prev = ms
	}
	return ticks, endOff
}

// siteSeparator joins the rooms of a site label in the game's HUD markup.
const siteSeparator = "<br/>"

// readSite returns the label of the current bomb site. Site labels are only
// sent in the initial state of the recording and never change afterwards.
func readSite(body []byte) string {
	key := binary.BigEndian.AppendUint32(nil, keyLabel)
	for pos := 0; ; {
		i := bytes.Index(body[pos:], key)
		if i < 0 {
			return ""
		}
		at := pos + i
		pos = at + 1
		// A label object's first property is the label itself.
		if at < 9 || body[at-9] != tagObject {
			continue
		}
		var label string
		current := false
		readProps(body, at, func(key uint32, val []byte) {
			switch {
			case key == keyLabel && label == "":
				label = string(val)
			case key == keySiteSlot && len(val) == 4:
				current = binary.LittleEndian.Uint32(val) == 1
			}
		})
		if current && strings.Contains(label, siteSeparator) {
			return strings.Join(strings.Split(label, siteSeparator), " / ")
		}
	}
}

// roundObjectRef returns the object reference (tag and id) of the round state
// object, the one whose first property is the seconds timer.
func roundObjectRef(body []byte) []byte {
	key := binary.BigEndian.AppendUint32(nil, keyTimerSeconds)
	for pos := 0; ; {
		i := bytes.Index(body[pos:], key)
		if i < 0 {
			return nil
		}
		at := pos + i
		if at >= 9 && body[at-9] == tagObject {
			return body[at-9 : at]
		}
		pos = at + 1
	}
}

// readKills returns the kill feed and the body offset of each entry.
func readKills(body []byte, clockAt func(off int) Clock) ([]Kill, []int) {
	kills := []Kill{}
	var offs []int
	key := binary.BigEndian.AppendUint32(nil, keyFeedEntry)
	// The game sends an entry again whenever the feed is redrawn, so entries
	// are told apart by object id.
	seen := map[string]bool{}
	for pos := 0; ; {
		i := bytes.Index(body[pos:], key)
		if i < 0 {
			break
		}
		at := pos + i
		pos = at + 1
		if at < 9 || body[at-9] != tagObject {
			continue
		}
		id := string(body[at-8 : at])
		if seen[id] {
			continue
		}
		k := Kill{KillerTeam: -1, VictimTeam: -1}
		readProps(body, at, func(key uint32, val []byte) {
			switch {
			case key == keyKiller:
				k.Killer = string(val)
			case key == keyVictim:
				k.Victim = string(val)
			case key == keyKillerTeam && len(val) == 4:
				k.KillerTeam = int(binary.LittleEndian.Uint32(val)) - 1
			case key == keyVictimTeam && len(val) == 4:
				k.VictimTeam = int(binary.LittleEndian.Uint32(val)) - 1
			case key == keyHeadshot && len(val) == 1:
				k.Headshot = val[0] == 1
			}
		})
		// Feed entries without a victim are phase announcements.
		if k.Victim == "" {
			continue
		}
		seen[id] = true
		k.Clock = clockAt(at)
		kills = append(kills, k)
		offs = append(offs, at)
	}
	return kills, offs
}

// readProps calls fn for each property of the update whose first key starts
// at pos. Indexed properties are reported without their index.
func readProps(b []byte, pos int, fn func(key uint32, val []byte)) {
	for first := true; ; first = false {
		indexed := false
		if !first {
			if pos >= len(b) {
				return
			}
			switch b[pos] {
			case tagProp:
			case tagIndexed:
				indexed = true
			default:
				return
			}
			pos++
		}
		if pos+4 > len(b) {
			return
		}
		key := binary.BigEndian.Uint32(b[pos:])
		pos += 4
		if indexed {
			pos += 4
		}
		if pos >= len(b) {
			return
		}
		n := int(b[pos])
		pos++
		if pos+n > len(b) {
			return
		}
		fn(key, b[pos:pos+n])
		pos += n
	}
}

// decideWinner applies the bomb rules to what was observed. The replay does
// carry the outcome somewhere, but it is not located yet.
func decideWinner(h *Header, r *Round) (int, WinReason) {
	if !r.Complete {
		return -1, ""
	}
	atk := -1
	for _, p := range h.Players {
		switch p.Side {
		case Attack:
			atk = p.Team
		case Defense:
			atk = 1 - p.Team
		}
		if atk >= 0 {
			break
		}
	}
	if atk != 0 && atk != 1 {
		return -1, ""
	}
	def := 1 - atk

	dead := [2]map[string]bool{{}, {}}
	for _, k := range r.Kills {
		if k.VictimTeam == 0 || k.VictimTeam == 1 {
			dead[k.VictimTeam][k.Victim] = true
		}
	}
	wiped := func(team int) bool {
		size := 0
		for _, p := range h.Players {
			if p.Team == team {
				size++
			}
		}
		if size == 0 { // roster missing from the header
			size = h.MaxPlayersPerTeam
		}
		return size > 0 && len(dead[team]) >= size
	}
	// The timer is zeroed at the end of every round, so a round that ran
	// out of time is one whose last reading was within a second of zero.
	timedOut := r.End.RemainingMs < 1100

	switch {
	case wiped(def):
		return atk, DefendersEliminated
	case r.Plant != nil && timedOut:
		return atk, DefuserDetonated
	case r.Plant != nil:
		return def, DefuserDisabled
	case wiped(atk):
		return def, AttackersEliminated
	case timedOut:
		return def, TimeExpired
	}
	return -1, ""
}
