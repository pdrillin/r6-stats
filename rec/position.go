package rec

import (
	"bytes"
	"encoding/binary"
	"sort"
	"strings"
)

// keyPlayerName is the player name in the state object that opens each
// player's block of the initial state.
const keyPlayerName = 0x07949bdc

// Position is a room a player's body entered.
type Position struct {
	Clock
	Room string `json:"room"`
}

// Scoreboard object: one per player, sent just before the player's name.
// Counters are cumulative over the recording session.
const (
	keyScoreKills   = 0x1cd2b19d // u32
	keyScoreDeaths  = 0xcd9c5d72 // u32
	keyScoreAssists = 0x4d737f9e // u32
	keyScorePlants  = 0x31a3d4d2 // u32, defuser plants
	keyScorePoints  = 0xecda4f80 // u32, in-game score
)

// playerObjects is what the initial state tells about one player.
type playerObjects struct {
	name  string
	view  string // label object following what the player looks through (drone, camera)
	body  string // label object following the player's body
	score string // scoreboard object
}

// readPlayerObjects walks the initial state. Each player has a block that
// starts with their name and holds two room labels: the first follows the
// player's view, the second their body. Blocks are found by position, so only
// the first occurrence of each name counts.
//
// The block also holds a plain label naming a spawn point, but it is often
// left over from an earlier round (defenders carry one too), so it is not
// used.
func readPlayerObjects(body []byte) []playerObjects {
	key := binary.BigEndian.AppendUint32(nil, keyPlayerName)
	type start struct {
		off  int
		name string
	}
	var starts []start
	seen := map[string]bool{}
	for pos := 0; ; {
		i := bytes.Index(body[pos:], key)
		if i < 0 {
			break
		}
		at := pos + i
		pos = at + 1
		if !isPropStart(body, at) || at+5 > len(body) {
			continue
		}
		n := int(body[at+4])
		if at+5+n > len(body) || n == 0 {
			continue
		}
		name := string(body[at+5 : at+5+n])
		if seen[name] {
			break // the initial state is over
		}
		seen[name] = true
		starts = append(starts, start{at, name})
	}

	// The scoreboard update opens with the kill counter and comes just
	// before the update that holds the name.
	scoreKey := binary.BigEndian.AppendUint32(nil, keyScoreKills)
	type board struct {
		off int
		id  string
	}
	var boards []board
	if len(starts) > 0 {
		limit := starts[len(starts)-1].off
		for pos := 0; pos < limit; {
			i := bytes.Index(body[pos:limit], scoreKey)
			if i < 0 {
				break
			}
			at := pos + i
			pos = at + 1
			if at >= 9 && body[at-9] == tagObject {
				boards = append(boards, board{at, string(body[at-8 : at])})
			}
		}
	}

	players := make([]playerObjects, len(starts))
	for i, s := range starts {
		end := len(body)
		if i+1 < len(starts) {
			end = starts[i+1].off
		}
		p := playerObjects{name: s.name}
		prev := 0
		if i > 0 {
			prev = starts[i-1].off
		}
		for _, b := range boards {
			if b.off > prev && b.off < s.off {
				p.score = b.id
			}
		}
		forEachLabel(body[:end], s.off, func(_ int, id string, _ string, room bool) bool {
			switch {
			case room && p.view == "":
				p.view = id
			case room && id != p.view && p.body == "":
				p.body = id
			}
			return p.body == ""
		})
		players[i] = p
	}
	return players
}

// isPropStart reports whether a property key starts at off: either the first
// property of an update or one introduced by tagProp.
func isPropStart(b []byte, off int) bool {
	return (off >= 9 && b[off-9] == tagObject) || (off >= 1 && b[off-1] == tagProp)
}

// forEachLabel calls fn for each label update in b[from:], in order. Room
// labels carry an 8-byte placeholder then the room as indexed element 0;
// plain labels carry the text directly. fn returns false to stop.
func forEachLabel(b []byte, from int, fn func(off int, id, label string, room bool) bool) {
	key := binary.BigEndian.AppendUint32(nil, keyLabel)
	for pos := from; ; {
		i := bytes.Index(b[pos:], key)
		if i < 0 {
			return
		}
		at := pos + i
		pos = at + 1
		if at < 9 || b[at-9] != tagObject {
			continue
		}
		id := string(b[at-8 : at])
		var (
			plain  string
			room   string
			isRoom bool
			first  = true
		)
		readProps(b, at, func(k uint32, val []byte) {
			switch {
			case k == keyLabel && first:
				first = false
				if len(val) == 8 && bytes.Equal(val, placeholder) {
					isRoom = true
				} else {
					plain = string(val)
				}
			case k == keyLabel && isRoom && room == "":
				// Element 0 is the room; element 1 is a one-byte flag.
				if len(val) != 1 || val[0] > 1 {
					room = string(val)
				}
			}
		})
		label := plain
		if isRoom {
			label = room
		}
		if !fn(at, id, label, isRoom) {
			return
		}
	}
}

var placeholder = bytes.Repeat([]byte{0xff}, 8)

// normalizeRoom collapses repeated spaces: some rooms are named with two
// ("1F  Kitchen Cooking") while the site label of the same room has one.
func normalizeRoom(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// tracks is the decoded body positions of every player.
type tracks struct {
	byName map[string][]trackPoint
}

type trackPoint struct {
	off  int
	room string // empty once the player is dead or not deployed
}

// readTracks follows the body label of each player through the round.
func readTracks(body []byte, players []playerObjects) tracks {
	owner := map[string]string{} // body object id -> name
	for _, p := range players {
		if p.body != "" {
			owner[p.body] = p.name
		}
	}
	t := tracks{byName: map[string][]trackPoint{}}
	forEachLabel(body, 0, func(off int, id, label string, room bool) bool {
		if name, ok := owner[id]; ok && room {
			t.byName[name] = append(t.byName[name], trackPoint{off, normalizeRoom(label)})
		}
		return true
	})
	return t
}

// roomAt returns the room the player's body was last seen in before off,
// counting updates sent in the same timer tick as off. Empty when unknown.
func (t tracks) roomAt(name string, off, tickEnd int) string {
	pts := t.byName[name]
	i := sort.Search(len(pts), func(i int) bool { return pts[i].off >= tickEnd })
	for i--; i >= 0; i-- {
		if pts[i].room != "" {
			return pts[i].room
		}
	}
	return ""
}

// positions converts a track into room changes with their time, leaving out
// the time the player is dead or not deployed.
func (t tracks) positions(name string, clockAt func(off int) Clock) []Position {
	var out []Position
	last := ""
	for _, p := range t.byName[name] {
		if p.room == "" || p.room == last {
			last = p.room
			continue
		}
		out = append(out, Position{Clock: clockAt(p.off), Room: p.room})
		last = p.room
	}
	return out
}

// readPlanter returns the player whose plant counter goes up during the
// recording, and the body offset where it does. The counter is cumulative,
// so the first value seen for each player is the baseline.
func readPlanter(body []byte, players []playerObjects) (string, int) {
	planter, at := "", -1
	for _, p := range players {
		if p.score == "" {
			continue
		}
		ref := append([]byte{tagObject}, p.score...)
		base := -1
		for pos := 0; ; {
			i := bytes.Index(body[pos:], ref)
			if i < 0 {
				break
			}
			off := pos + i
			pos = off + len(ref)
			plants := -1
			readProps(body, pos, func(k uint32, val []byte) {
				if k == keyScorePlants && len(val) == 4 {
					plants = int(binary.LittleEndian.Uint32(val))
				}
			})
			switch {
			case plants < 0:
			case base < 0:
				base = plants
			case plants > base:
				if at < 0 || off < at {
					planter, at = p.name, off
				}
				pos = len(body) // first increase only
			}
		}
	}
	return planter, at
}

// Scoreboard is what a player's scoreboard counters gained during the
// recording: last value seen minus first value seen.
type Scoreboard struct {
	Kills   int `json:"kills"`
	Deaths  int `json:"deaths"`
	Assists int `json:"assists"`
	Plants  int `json:"plants"`
	Points  int `json:"points"`
}

// readScoreboards returns the counters each player gained, by name.
func readScoreboards(body []byte, players []playerObjects) map[string]Scoreboard {
	out := map[string]Scoreboard{}
	for _, p := range players {
		if p.score == "" {
			continue
		}
		first := map[uint32]int{}
		last := map[uint32]int{}
		ref := append([]byte{tagObject}, p.score...)
		for pos := 0; ; {
			i := bytes.Index(body[pos:], ref)
			if i < 0 {
				break
			}
			pos += i + len(ref)
			readProps(body, pos, func(k uint32, val []byte) {
				switch k {
				case keyScoreKills, keyScoreDeaths, keyScoreAssists, keyScorePlants, keyScorePoints:
				default:
					return
				}
				if len(val) != 4 {
					return
				}
				v := int(binary.LittleEndian.Uint32(val))
				if _, ok := first[k]; !ok {
					first[k] = v
				}
				last[k] = v
			})
		}
		gain := func(k uint32) int { return last[k] - first[k] }
		out[p.name] = Scoreboard{
			Kills:   gain(keyScoreKills),
			Deaths:  gain(keyScoreDeaths),
			Assists: gain(keyScoreAssists),
			Plants:  gain(keyScorePlants),
			Points:  gain(keyScorePoints),
		}
	}
	return out
}

// defuseBonus is the score a defender earns for disabling the defuser, on top
// of the round win bonus every winner gets at the same moment.
const defuseBonus = 100

// readDefuser returns the defender who disabled the defuser, for a round won
// that way. The scoreboard has no defuse counter, but the defuse is scored:
// at the end of the round every winner gains the same bonus and the defuser
// gains defuseBonus more. from is where the final timer tick starts; points
// gained from there to the end of the recording are compared.
func readDefuser(body []byte, players []playerObjects, defenders map[string]bool, from int) string {
	gains := map[string]int{}
	for _, p := range players {
		if p.score == "" || !defenders[p.name] {
			continue
		}
		before, last := -1, -1
		ref := append([]byte{tagObject}, p.score...)
		for pos := 0; ; {
			i := bytes.Index(body[pos:], ref)
			if i < 0 {
				break
			}
			off := pos + i
			pos = off + len(ref)
			readProps(body, pos, func(k uint32, val []byte) {
				if k != keyScorePoints || len(val) != 4 {
					return
				}
				v := int(binary.LittleEndian.Uint32(val))
				if off < from {
					before = v
				}
				last = v
			})
		}
		if before >= 0 {
			gains[p.name] = last - before
		}
	}
	// The defuser stands out by exactly the bonus; anything else is too
	// ambiguous to name anyone.
	best, bestGain, second := "", -1, -1
	for name, g := range gains {
		switch {
		case g > bestGain:
			second, best, bestGain = bestGain, name, g
		case g > second:
			second = g
		}
	}
	if best == "" || second < 0 || bestGain-second != defuseBonus {
		return ""
	}
	return best
}
