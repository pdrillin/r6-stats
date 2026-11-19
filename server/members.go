package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// User is a person signed in with Discord.
type User struct {
	ID        string `json:"id"`
	DiscordID string `json:"discordId"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"` // Discord avatar hash, empty for the default one
}

// Role is what a member may do in a space.
type Role string

const (
	// Manager (captain, coach) invites and removes members, changes roles,
	// labels matches and chooses the space's own team.
	Manager Role = "manager"
	// Player sends replays and sees everything.
	Player Role = "player"
)

func (r Role) valid() bool { return r == Manager || r == Player }

// Errors of member management, shown to the user.
var (
	ErrForbidden   = errors.New("forbidden")
	ErrLastManager = errors.New("a space keeps at least one manager")
	ErrExpired     = errors.New("invitation expired or revoked")
)

// SessionLifetime is how long a sign-in lasts.
const SessionLifetime = 30 * 24 * time.Hour

// InvitationLifetime is how long an invitation link works.
const InvitationLifetime = 7 * 24 * time.Hour

// token returns a random secret for a cookie or a link, and its hash, the
// only form kept in the database.
func token() (secret, hash string) {
	b := make([]byte, 32)
	rand.Read(b)
	secret = base64.RawURLEncoding.EncodeToString(b)
	return secret, hashToken(secret)
}

func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func timeIn(d time.Duration) string {
	return time.Now().Add(d).UTC().Format(time.RFC3339)
}

// SignIn records a Discord account, creating the user on first sign-in and
// refreshing their name and avatar otherwise, and opens a session. It
// returns the session secret, for the cookie.
func (s *Server) SignIn(discordID, name, avatar string) (User, string, error) {
	u := User{DiscordID: discordID, Name: name, Avatar: avatar}
	err := s.db.QueryRow("SELECT id FROM users WHERE discord_id = ?", discordID).Scan(&u.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		u.ID = newID()
		_, err = s.db.Exec("INSERT INTO users (id, discord_id, name, avatar, created_at) VALUES (?, ?, ?, ?, ?)",
			u.ID, discordID, name, avatar, now())
	case err == nil:
		_, err = s.db.Exec("UPDATE users SET name = ?, avatar = ? WHERE id = ?", name, avatar, u.ID)
	}
	if err != nil {
		return User{}, "", err
	}
	secret, hash := token()
	if _, err := s.db.Exec("INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		hash, u.ID, now(), timeIn(SessionLifetime)); err != nil {
		return User{}, "", err
	}
	// Old sessions are swept on sign-in: there is no other housekeeping.
	s.db.Exec("DELETE FROM sessions WHERE expires_at < ?", now())
	return u, secret, nil
}

// SessionUser returns the user of a session secret, if the session is valid.
func (s *Server) SessionUser(secret string) (User, bool) {
	if secret == "" {
		return User{}, false
	}
	var u User
	err := s.db.QueryRow(`SELECT u.id, u.discord_id, u.name, u.avatar FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, hashToken(secret), now()).
		Scan(&u.ID, &u.DiscordID, &u.Name, &u.Avatar)
	return u, err == nil
}

// SignOut ends a session.
func (s *Server) SignOut(secret string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", hashToken(secret))
	return err
}

// role is the role of a user in a space, empty when they are not a member.
func (s *Server) role(spaceID, userID string) (Role, error) {
	var r Role
	err := s.db.QueryRow("SELECT role FROM memberships WHERE space_id = ? AND user_id = ?", spaceID, userID).Scan(&r)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return r, err
}

// addMember makes a user member of a space. An existing member keeps the
// higher of their role and the new one.
func (s *Server) addMember(spaceID, userID string, role Role) error {
	_, err := s.db.Exec(`INSERT INTO memberships (space_id, user_id, role, joined_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (space_id, user_id) DO UPDATE SET role = CASE
			WHEN memberships.role = 'manager' OR excluded.role = 'manager' THEN 'manager' ELSE 'player' END`,
		spaceID, userID, role, now())
	return err
}

// Member is a user's place in a space.
type Member struct {
	User
	Role     Role   `json:"role"`
	JoinedAt string `json:"joinedAt"`
}

// Members lists the members of a space, managers first.
func (s *Server) Members(spaceID string) ([]Member, error) {
	rows, err := s.db.Query(`SELECT u.id, u.discord_id, u.name, u.avatar, m.role, m.joined_at
		FROM memberships m JOIN users u ON u.id = m.user_id WHERE m.space_id = ?
		ORDER BY m.role = 'player', m.joined_at, u.name`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.DiscordID, &m.Name, &m.Avatar, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetRole changes the role of a member. A space keeps at least one manager.
func (s *Server) SetRole(spaceID, userID string, role Role) error {
	if !role.valid() {
		return fmt.Errorf("unknown role %q", role)
	}
	return s.changeMember(spaceID, userID, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE memberships SET role = ? WHERE space_id = ? AND user_id = ?", role, spaceID, userID)
		return err
	})
}

// RemoveMember takes a user out of a space. A space keeps at least one
// manager.
func (s *Server) RemoveMember(spaceID, userID string) error {
	return s.changeMember(spaceID, userID, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM memberships WHERE space_id = ? AND user_id = ?", spaceID, userID)
		return err
	})
}

// changeMember applies a change to a membership, refusing it when it would
// leave the space without a manager.
func (s *Server) changeMember(spaceID, userID string, change func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM memberships WHERE space_id = ? AND user_id = ?", spaceID, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := change(tx); err != nil {
		return err
	}
	if err := tx.QueryRow("SELECT COUNT(*) FROM memberships WHERE space_id = ? AND role = 'manager'", spaceID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrLastManager
	}
	return tx.Commit()
}

// Invitation is a link to join a space. Its secret is only known when it is
// created: the database keeps its hash.
type Invitation struct {
	ID        string `json:"id"`
	Role      Role   `json:"role"`
	CreatedBy string `json:"createdBy"` // name of the user who created it
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
	// Token is the secret of the link, only set when the invitation is
	// created.
	Token string `json:"token,omitempty"`
}

// Invite creates an invitation to a space.
func (s *Server) Invite(spaceID, byUserID string, role Role) (Invitation, error) {
	if !role.valid() {
		return Invitation{}, fmt.Errorf("unknown role %q", role)
	}
	secret, hash := token()
	inv := Invitation{ID: newID(), Role: role, CreatedAt: now(), ExpiresAt: timeIn(InvitationLifetime), Token: secret}
	var by any
	if byUserID != "" {
		by = byUserID
	}
	_, err := s.db.Exec(`INSERT INTO invitations (id, token_hash, space_id, role, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, inv.ID, hash, spaceID, role, by, inv.CreatedAt, inv.ExpiresAt)
	return inv, err
}

// Invitations lists the invitations of a space that still work.
func (s *Server) Invitations(spaceID string) ([]Invitation, error) {
	rows, err := s.db.Query(`SELECT i.id, i.role, COALESCE(u.name, ''), i.created_at, i.expires_at
		FROM invitations i LEFT JOIN users u ON u.id = i.created_by
		WHERE i.space_id = ? AND i.expires_at > ? ORDER BY i.created_at`, spaceID, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.Role, &inv.CreatedBy, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Revoke deletes an invitation of a space.
func (s *Server) Revoke(spaceID, invitationID string) error {
	r, err := s.db.Exec("DELETE FROM invitations WHERE id = ? AND space_id = ?", invitationID, spaceID)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// InvitationInfo is what an invitation link shows before it is accepted.
type InvitationInfo struct {
	SpaceID   string    `json:"spaceId"`
	SpaceName string    `json:"spaceName"`
	SpaceKind SpaceKind `json:"spaceKind"`
	Role      Role      `json:"role"`
	ExpiresAt string    `json:"expiresAt"`
}

// Invitation returns what a link invites to, if it still works.
func (s *Server) Invitation(secret string) (InvitationInfo, error) {
	var info InvitationInfo
	err := s.db.QueryRow(`SELECT s.id, s.name, s.kind, i.role, i.expires_at FROM invitations i JOIN spaces s ON s.id = i.space_id
		WHERE i.token_hash = ? AND i.expires_at > ?`, hashToken(secret), now()).
		Scan(&info.SpaceID, &info.SpaceName, &info.SpaceKind, &info.Role, &info.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return info, ErrExpired
	}
	return info, err
}

// Accept makes a user member of the space of an invitation.
func (s *Server) Accept(secret, userID string) (InvitationInfo, error) {
	info, err := s.Invitation(secret)
	if err != nil {
		return info, err
	}
	return info, s.addMember(info.SpaceID, userID, info.Role)
}
