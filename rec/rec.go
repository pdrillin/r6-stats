// Package rec reads Rainbow Six Siege match replay files (.rec, "dissect" format).
//
// A match is a folder holding one .rec file per round. Each file is laid out as:
//
//	magic "dissect\0"
//	preamble   u32, string, u32, u32
//	properties u64 count, then count × (string key, string value)
//	(an index region that is not decoded yet)
//	chunks     each: tag "200VRPMC", u32 size, u32 compressed size, zstd frame
//
// Strings are a u64 length followed by UTF-8 bytes. All integers are little-endian.
package rec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/klauspost/compress/zstd"
)

var (
	magic     = []byte("dissect\x00")
	chunkTag  = []byte("200VRPMC")
	zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}
)

// ErrNotReplay is returned when the data does not start with the replay magic.
var ErrNotReplay = errors.New("rec: not a dissect replay file")

const (
	maxKeyLen   = 256
	maxValueLen = 1 << 16
	maxProps    = 1 << 16
	// maxChunkSize bounds a single decompressed chunk; observed chunks are ~512 KiB.
	maxChunkSize = 64 << 20
)

// Property is one raw key/value pair of the file header, in file order.
// Keys repeat (one block per player, many "gmsetting"), so order matters.
type Property struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Player is one player block of the header.
type Player struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileId"` // Ubisoft profile UUID; zeroed in tournament replays
	Name      string `json:"name"`
	Team      int    `json:"team"` // index into Header.Teams
	Side      Side   `json:"side"`
	Operator  string `json:"operator"` // display name, e.g. "FROST"
	HeroID    string `json:"heroId"`
}

// Side is the role a player holds during one round.
type Side string

const (
	Attack  Side = "attack"
	Defense Side = "defense"
)

// sideOf decodes the header "alliance" value. Checked against operator picks
// on both halves of a map: 3 is the attacking side, 4 the defending one.
func sideOf(alliance string) Side {
	switch alliance {
	case "3":
		return Attack
	case "4":
		return Defense
	}
	return ""
}

// Team is one of the two rosters of the match.
type Team struct {
	Name          string `json:"name"`
	StartingScore int    `json:"startingScore"` // score before this round is played
}

// Header is the decoded property block of one round file.
type Header struct {
	GameVersion         string `json:"gameVersion"` // e.g. "Y11S2_Alpha04"
	CodeVersion         int    `json:"codeVersion"`
	Timestamp           string `json:"timestamp"` // as written by the game: 2006-01-02-15-04-05
	MatchID             string `json:"matchId"`
	MatchType           string `json:"matchType"`
	MapID               string `json:"mapId"`
	Map                 string `json:"map,omitempty"` // empty when MapID is unknown
	GameModeID          string `json:"gameModeId"`
	RoundNumber         int    `json:"roundNumber"` // zero-based
	OvertimeRoundNumber int    `json:"overtimeRoundNumber"`
	RoundsPerMatch      int    `json:"roundsPerMatch"`
	RoundsPerOvertime   int    `json:"roundsPerOvertime"`
	MaxPlayersPerTeam   int    `json:"maxPlayersPerTeam"`
	RecordingPlayerID   string `json:"recordingPlayerId"`
	RecordingProfileID  string `json:"recordingProfileId"`
	IsSpectator         bool   `json:"isSpectator"`

	Teams   [2]Team  `json:"teams"`
	Players []Player `json:"players"`

	// Properties is every key/value pair exactly as stored, including the ones
	// not mapped to a field above.
	Properties []Property `json:"-"`
}

// Chunk describes one compressed block of the replay body.
type Chunk struct {
	Offset         int // offset of the zstd frame in the file
	CompressedSize int
	Size           int // decompressed size
}

// File is a parsed round file. The body stays compressed until read.
type File struct {
	Header Header
	Chunks []Chunk

	data []byte
}

// Open reads and parses the replay at path.
func Open(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Parse decodes the header and indexes the chunks of a replay held in memory.
// It keeps a reference to data.
func Parse(data []byte) (*File, error) {
	r := &reader{b: data}
	if !bytes.HasPrefix(data, magic) {
		return nil, ErrNotReplay
	}
	r.pos = len(magic)

	// Preamble: values observed constant across files, meaning unknown.
	r.u32()
	r.str(maxKeyLen)
	r.u32()
	r.u32()

	count := r.u64()
	if r.err == nil && count > maxProps {
		return nil, fmt.Errorf("rec: implausible property count %d", count)
	}
	props := make([]Property, 0, count)
	for i := uint64(0); i < count && r.err == nil; i++ {
		k := r.str(maxKeyLen)
		v := r.str(maxValueLen)
		props = append(props, Property{k, v})
	}
	if r.err != nil {
		return nil, fmt.Errorf("rec: header: %w", r.err)
	}

	chunks, err := indexChunks(data, r.pos)
	if err != nil {
		return nil, err
	}
	return &File{Header: buildHeader(props), Chunks: chunks, data: data}, nil
}

// indexChunks walks the chunk list starting at from. Bytes between two chunks
// (section descriptors, not decoded yet) are skipped.
func indexChunks(data []byte, from int) ([]Chunk, error) {
	var chunks []Chunk
	pos := from
	for {
		i := bytes.Index(data[pos:], chunkTag)
		if i < 0 {
			break
		}
		hdr := pos + i
		body := hdr + len(chunkTag) + 8
		if body+len(zstdMagic) > len(data) || !bytes.Equal(data[body:body+len(zstdMagic)], zstdMagic) {
			// The tag bytes happened to occur inside undecoded data.
			pos = hdr + 1
			continue
		}
		size := int(binary.LittleEndian.Uint32(data[hdr+8:]))
		csize := int(binary.LittleEndian.Uint32(data[hdr+12:]))
		if size > maxChunkSize {
			return nil, fmt.Errorf("rec: chunk at %d: implausible size %d", hdr, size)
		}
		if body+csize > len(data) {
			return nil, fmt.Errorf("rec: chunk at %d: truncated (%d bytes missing)", hdr, body+csize-len(data))
		}
		chunks = append(chunks, Chunk{Offset: body, CompressedSize: csize, Size: size})
		pos = body + csize
	}
	if len(chunks) == 0 {
		return nil, errors.New("rec: no data chunk found")
	}
	return chunks, nil
}

// Size returns the total decompressed size of the body.
func (f *File) Size() int {
	n := 0
	for _, c := range f.Chunks {
		n += c.Size
	}
	return n
}

// WriteBody decompresses every chunk, in file order, into w.
func (f *File) WriteBody(w io.Writer) error {
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return err
	}
	defer dec.Close()

	var buf []byte
	for i, c := range f.Chunks {
		buf, err = dec.DecodeAll(f.data[c.Offset:c.Offset+c.CompressedSize], buf[:0])
		if err != nil {
			return fmt.Errorf("rec: chunk %d: %w", i, err)
		}
		if len(buf) != c.Size {
			return fmt.Errorf("rec: chunk %d: got %d bytes, header says %d", i, len(buf), c.Size)
		}
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func buildHeader(props []Property) Header {
	h := Header{Properties: props}
	var cur *Player // player block being filled, nil outside of one
	for _, p := range props {
		if p.Key == "playerid" {
			h.Players = append(h.Players, Player{ID: p.Value})
			cur = &h.Players[len(h.Players)-1]
			continue
		}
		if cur != nil {
			switch p.Key {
			case "profileid":
				cur.ProfileID = p.Value
			case "playername":
				cur.Name = p.Value
			case "team":
				cur.Team = atoi(p.Value)
			case "alliance":
				cur.Side = sideOf(p.Value)
			case "rolename":
				cur.Operator = p.Value
			case "heroname":
				cur.HeroID = p.Value
			case "roleimage", "roleportrait":
			default:
				cur = nil
			}
			if cur != nil {
				continue
			}
		}
		switch p.Key {
		case "version":
			h.GameVersion = p.Value
		case "code":
			h.CodeVersion = atoi(p.Value)
		case "datetime":
			h.Timestamp = p.Value
		case "id":
			h.MatchID = p.Value
		case "matchtype":
			h.MatchType = p.Value
		case "worldid":
			h.MapID = p.Value
			h.Map = mapNames[p.Value]
		case "gamemodeid":
			h.GameModeID = p.Value
		case "roundnumber":
			h.RoundNumber = atoi(p.Value)
		case "overtimeroundnumber":
			h.OvertimeRoundNumber = atoi(p.Value)
		case "roundspermatch":
			h.RoundsPerMatch = atoi(p.Value)
		case "roundspermatchovertime":
			h.RoundsPerOvertime = atoi(p.Value)
		case "maxnbplayersperteam":
			h.MaxPlayersPerTeam = atoi(p.Value)
		case "recordingplayerid":
			h.RecordingPlayerID = p.Value
		case "recordingprofileid":
			h.RecordingProfileID = p.Value
		case "isspectator":
			h.IsSpectator = p.Value == "1"
		case "teamname0":
			h.Teams[0].Name = p.Value
		case "teamname1":
			h.Teams[1].Name = p.Value
		case "startingteamscore0":
			h.Teams[0].StartingScore = atoi(p.Value)
		case "startingteamscore1":
			h.Teams[1].StartingScore = atoi(p.Value)
		}
	}
	return h
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// reader is a bounds-checked little-endian cursor. After the first failure
// every read returns a zero value and err stays set.
type reader struct {
	b   []byte
	pos int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.b) {
		r.err = fmt.Errorf("unexpected end of data at offset %d", r.pos)
		return nil
	}
	s := r.b[r.pos : r.pos+n]
	r.pos += n
	return s
}

func (r *reader) u32() uint32 {
	if s := r.take(4); s != nil {
		return binary.LittleEndian.Uint32(s)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if s := r.take(8); s != nil {
		return binary.LittleEndian.Uint64(s)
	}
	return 0
}

func (r *reader) str(max int) string {
	n := r.u64()
	if r.err != nil {
		return ""
	}
	if n > uint64(max) {
		r.err = fmt.Errorf("string of %d bytes at offset %d exceeds limit %d", n, r.pos-8, max)
		return ""
	}
	return string(r.take(int(n)))
}
