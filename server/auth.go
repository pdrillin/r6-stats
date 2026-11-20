package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config is what the server needs to know about where it runs and who may
// use it.
type Config struct {
	// BaseURL is the address users reach the server at, without trailing
	// slash, e.g. https://stats.example.org or http://localhost:8642. The
	// Discord redirect URI is BaseURL + "/auth/callback".
	BaseURL string
	// Discord is the OAuth2 application used to sign in. Without a client
	// id, Discord sign-in is off.
	Discord Discord
	// Creators are the Discord ids allowed to create spaces, everyone else
	// joins through invitations (closed beta). Empty lets every signed-in
	// user create spaces.
	Creators []string
	// DevLogin enables /auth/dev?name=…, which signs in without Discord.
	// Only for development on the local machine.
	DevLogin bool
	// Operator and Contact name who runs the site and how to reach them,
	// shown on the privacy page: the data controller under the GDPR.
	Operator string
	Contact  string
}

// Discord is an OAuth2 application of the Discord developer portal.
type Discord struct {
	ClientID     string
	ClientSecret string
	// Endpoints, Discord's own when empty (tests replace them).
	AuthorizeURL string
	TokenURL     string
	UserURL      string
}

func (d Discord) withDefaults() Discord {
	if d.AuthorizeURL == "" {
		d.AuthorizeURL = "https://discord.com/oauth2/authorize"
	}
	if d.TokenURL == "" {
		d.TokenURL = "https://discord.com/api/oauth2/token"
	}
	if d.UserURL == "" {
		d.UserURL = "https://discord.com/api/users/@me"
	}
	return d
}

const (
	sessionCookie = "stratdata_session"
	stateCookie   = "stratdata_oauth_state"
)

type userKey struct{}

// currentUser is the signed-in user of a request, if any.
func currentUser(r *http.Request) (User, bool) {
	u, ok := r.Context().Value(userKey{}).(User)
	return u, ok
}

// withUser reads the session cookie and puts its user in the request
// context.
func (s *Server) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			if u, ok := s.SessionUser(c.Value); ok {
				r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// secure tells whether cookies must be HTTPS-only.
func (c Config) secure() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func (s *Server) setSession(w http.ResponseWriter, secret string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    secret,
		Path:     "/",
		MaxAge:   int(SessionLifetime / time.Second),
		HttpOnly: true,
		Secure:   s.cfg.secure(),
		// Lax: the cookie is not sent with requests that other sites
		// make in the background, so they cannot act as the user.
		SameSite: http.SameSiteLaxMode,
	})
}

// localNext keeps a redirect target on this site: a path, never a full URL
// that could send the user elsewhere after signing in.
func localNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// login sends the browser to Discord. The state, a random secret also kept
// in a short-lived cookie, ties the answer to this browser.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	d := s.cfg.Discord.withDefaults()
	if d.ClientID == "" {
		http.Error(w, "Discord sign-in is not configured", http.StatusServiceUnavailable)
		return
	}
	state, _ := token()
	next := localNext(r.URL.Query().Get("next"))
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    state + "|" + url.QueryEscape(next),
		Path:     "/auth/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   s.cfg.secure(),
		SameSite: http.SameSiteLaxMode,
	})
	q := url.Values{
		"client_id":     {d.ClientID},
		"redirect_uri":  {s.cfg.BaseURL + "/auth/callback"},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
		"prompt":        {"none"},
	}
	http.Redirect(w, r, d.AuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

// callback receives Discord's answer, reads who the user is, and signs them
// in. The Discord token is used once and not kept.
func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(stateCookie)
	if err != nil {
		http.Error(w, "sign-in expired, try again", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Path: "/auth/", MaxAge: -1})
	state, nextEsc, _ := strings.Cut(c.Value, "|")
	q := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 || state == "" {
		http.Error(w, "sign-in state mismatch, try again", http.StatusBadRequest)
		return
	}
	if q.Get("error") != "" {
		// The user declined on Discord.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	code := q.Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	du, err := s.discordUser(r.Context(), code)
	if err != nil {
		log.Printf("discord sign-in: %v", err)
		http.Error(w, "Discord sign-in failed", http.StatusBadGateway)
		return
	}
	_, secret, err := s.SignIn(du.ID, du.displayName(), du.Avatar)
	if err != nil {
		serverError(w, err)
		return
	}
	s.setSession(w, secret)
	next, _ := url.QueryUnescape(nextEsc)
	http.Redirect(w, r, localNext(next), http.StatusFound)
}

type discordUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Avatar     string `json:"avatar"`
}

func (u discordUser) displayName() string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// discordUser trades the authorization code for a token and reads the
// user's identity.
func (s *Server) discordUser(ctx context.Context, code string) (discordUser, error) {
	d := s.cfg.Discord.withDefaults()
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {s.cfg.BaseURL + "/auth/callback"},
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", d.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(d.ClientID, d.ClientSecret)
	var tok struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := doJSON(req, &tok); err != nil {
		return discordUser{}, fmt.Errorf("token: %w", err)
	}
	if tok.AccessToken == "" {
		return discordUser{}, fmt.Errorf("token: empty access token")
	}
	req, _ = http.NewRequestWithContext(ctx, "GET", d.UserURL, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	var u discordUser
	if err := doJSON(req, &u); err != nil {
		return discordUser{}, fmt.Errorf("user: %w", err)
	}
	if u.ID == "" {
		return discordUser{}, fmt.Errorf("user: no id")
	}
	return u, nil
}

func doJSON(req *http.Request, v any) error {
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, v)
}

// devLogin signs in as a made-up user, without Discord. The handler is only
// mounted when Config.DevLogin is set.
func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" || len(name) > 32 {
		http.Error(w, "name: 1 to 32 characters", http.StatusBadRequest)
		return
	}
	_, secret, err := s.SignIn("dev:"+name, name, "")
	if err != nil {
		serverError(w, err)
		return
	}
	s.setSession(w, secret)
	http.Redirect(w, r, localNext(r.URL.Query().Get("next")), http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.SignOut(c.Value); err != nil {
			serverError(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// canCreate tells whether a user may create spaces.
func (s *Server) canCreate(u User) bool {
	if len(s.cfg.Creators) == 0 {
		return true
	}
	for _, id := range s.cfg.Creators {
		if id == u.DiscordID {
			return true
		}
	}
	return false
}
