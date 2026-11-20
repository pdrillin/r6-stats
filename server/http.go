package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

// MaxFilesPerUpload bounds one upload request: a match is one file per round,
// at most a few dozen with overtime and rehosts.
const MaxFilesPerUpload = 64

// Handler serves the server: sign-in, and the API.
//
//	GET  /auth/login?next=/…            sign in with Discord
//	GET  /auth/callback                 Discord's answer
//	POST /auth/logout                   sign out
//	GET  /auth/dev?name=…               sign in without Discord (Config.DevLogin)
//
//	GET  /api/info                      who runs the site (public, for the privacy page)
//	GET  /api/me                        the signed-in user
//	GET  /api/me/export                 everything kept about them (JSON download)
//	DELETE /api/me                      delete their account
//	GET  /api/spaces                    their spaces, with their role
//	POST /api/spaces                    create one: {"name", "kind": "team"|"league"}
//	GET  /api/invitations/{token}       what an invitation link leads to
//	POST /api/invitations/{token}/accept
//
// In a space (members only; anyone else gets 404, as if it did not exist):
//
//	POST   /api/spaces/{id}/replays               send replays (multipart, field "files")
//	GET    /api/spaces/{id}/members
//	PUT    /api/spaces/{id}/members/{user}        {"role"}           manager
//	DELETE /api/spaces/{id}/members/{user}        manager, or oneself to leave
//	GET    /api/spaces/{id}/invitations                              manager
//	POST   /api/spaces/{id}/invitations           {"role"}           manager
//	DELETE /api/spaces/{id}/invitations/{inv}                        manager
//	DELETE /api/spaces/{id}/matches/{match}       manager
//	DELETE /api/spaces/{id}/matches               manager: every match, the space stays
//	DELETE /api/spaces/{id}                       manager
//	       /api/spaces/{id}/…                     statistics (package api); writes: manager
func (s *Server) Handler(cfg Config) http.Handler {
	s.cfg = cfg
	mux := http.NewServeMux()

	mux.HandleFunc("GET /auth/login", s.login)
	mux.HandleFunc("GET /auth/callback", s.callback)
	mux.HandleFunc("POST /auth/logout", s.logout)
	if cfg.DevLogin {
		mux.HandleFunc("GET /auth/dev", s.devLogin)
	}

	mux.HandleFunc("GET /api/me", s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		writeJSON(w, http.StatusOK, struct {
			User
			CanCreate bool `json:"canCreate"`
		}{u, s.canCreate(u)})
	}))
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Operator string `json:"operator"`
			Contact  string `json:"contact"`
		}{s.cfg.Operator, s.cfg.Contact})
	})
	mux.HandleFunc("GET /api/me/export", s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		e, err := s.ExportUser(u.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="stratdata-mes-donnees.json"`)
		writeJSON(w, http.StatusOK, e)
	}))
	mux.HandleFunc("DELETE /api/me", s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		err := s.DeleteAccount(u.ID)
		var sole ErrSoleManager
		if errors.As(err, &sole) {
			writeJSON(w, http.StatusConflict, struct {
				Error  string   `json:"error"`
				Spaces []string `json:"spaces"`
			}{"sole manager", sole.Spaces})
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1})
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/spaces", s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		spaces, err := s.SpacesOf(u.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, spaces)
	}))
	mux.HandleFunc("POST /api/spaces", s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		if !s.canCreate(u) {
			http.Error(w, "only invited people can create spaces during the beta", http.StatusForbidden)
			return
		}
		var body struct {
			Name string    `json:"name"`
			Kind SpaceKind `json:"kind"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		sp, err := s.CreateSpace(body.Name, body.Kind, u.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, sp)
	}))
	mux.HandleFunc("GET /api/invitations/{token}", s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		info, err := s.Invitation(r.PathValue("token"))
		if errors.Is(err, ErrExpired) {
			http.Error(w, err.Error(), http.StatusGone)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}))
	mux.HandleFunc("POST /api/invitations/{token}/accept", s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		info, err := s.Accept(r.PathValue("token"), u.ID)
		if errors.Is(err, ErrExpired) {
			http.Error(w, err.Error(), http.StatusGone)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		s.forget(info.SpaceID)
		writeJSON(w, http.StatusOK, info)
	}))

	mux.HandleFunc("POST /api/spaces/{id}/replays", s.inSpace(Player, s.upload))
	mux.HandleFunc("GET /api/spaces/{id}/members", s.inSpace(Player, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		members, err := s.Members(r.PathValue("id"))
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, members)
	}))
	mux.HandleFunc("PUT /api/spaces/{id}/members/{user}", s.inSpace(Manager, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		var body struct {
			Role Role `json:"role"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		memberError(w, s.SetRole(r.PathValue("id"), r.PathValue("user"), body.Role))
	}))
	mux.HandleFunc("DELETE /api/spaces/{id}/members/{user}", s.inSpace(Player, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		target := r.PathValue("user")
		if target != u.ID && role != Manager {
			http.Error(w, "only managers remove other members", http.StatusForbidden)
			return
		}
		memberError(w, s.RemoveMember(r.PathValue("id"), target))
	}))
	mux.HandleFunc("GET /api/spaces/{id}/invitations", s.inSpace(Manager, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		list, err := s.Invitations(r.PathValue("id"))
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}))
	mux.HandleFunc("POST /api/spaces/{id}/invitations", s.inSpace(Manager, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		var body struct {
			Role Role `json:"role"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		inv, err := s.Invite(r.PathValue("id"), u.ID, body.Role)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, struct {
			Invitation
			URL string `json:"url"`
		}{inv, s.cfg.BaseURL + "/invite/" + inv.Token})
	}))
	mux.HandleFunc("DELETE /api/spaces/{id}/invitations/{inv}", s.inSpace(Manager, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		memberError(w, s.Revoke(r.PathValue("id"), r.PathValue("inv")))
	}))
	mux.HandleFunc("DELETE /api/spaces/{id}/matches/{match}", s.inSpace(Manager, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		memberError(w, s.DeleteMatch(r.PathValue("id"), r.PathValue("match")))
	}))
	mux.HandleFunc("DELETE /api/spaces/{id}/matches", s.inSpace(Manager, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		n, err := s.ClearSpace(r.PathValue("id"))
		if err != nil {
			memberError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Replays int `json:"replays"`
		}{n})
	}))
	mux.HandleFunc("DELETE /api/spaces/{id}", s.inSpace(Manager, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		memberError(w, s.DeleteSpace(r.PathValue("id")))
	}))
	mux.HandleFunc("/api/spaces/{id}/", s.inSpace(Player, func(w http.ResponseWriter, r *http.Request, u User, role Role) {
		// Reading is for members, changing labels and settings for
		// managers.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && role != Manager {
			http.Error(w, "only managers change the space", http.StatusForbidden)
			return
		}
		id := r.PathValue("id")
		v, err := s.view(id)
		if err != nil {
			serverError(w, err)
			return
		}
		// The space's API answers under /api/, as when it runs alone.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/api/" + strings.TrimPrefix(r.URL.Path, "/api/spaces/"+id+"/")
		r2.URL.RawPath = ""
		v.ServeHTTP(w, r2)
	}))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return s.withUser(s.sameSite(mux))
}

// sameSite refuses changes requested by another site: a browser tells the
// origin of the page that sends a request. GET requests change nothing.
func (s *Server) sameSite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
				http.Error(w, "cross-site request refused", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" && s.cfg.BaseURL != "" && o != s.cfg.BaseURL {
				http.Error(w, "cross-site request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// signedIn lets through signed-in users only.
func (s *Server) signedIn(h func(http.ResponseWriter, *http.Request, User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := currentUser(r)
		if !ok {
			http.Error(w, "sign in first", http.StatusUnauthorized)
			return
		}
		h(w, r, u)
	}
}

// inSpace lets through members of the space {id} with at least the role
// need. Non-members get 404: a space's existence is private too.
func (s *Server) inSpace(need Role, h func(http.ResponseWriter, *http.Request, User, Role)) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request, u User) {
		role, err := s.role(r.PathValue("id"), u.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		if role == "" {
			http.Error(w, "space not found", http.StatusNotFound)
			return
		}
		if need == Manager && role != Manager {
			http.Error(w, "managers only", http.StatusForbidden)
			return
		}
		h(w, r, u, role)
	})
}

// memberError answers a member management call.
func memberError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrLastManager):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		if strings.HasPrefix(err.Error(), "unknown role") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		serverError(w, err)
	}
}

// upload receives the replays of a match. Files are read one after the other
// and decoded a few at a time, so memory stays bounded whatever the number
// of files.
func (s *Server) upload(w http.ResponseWriter, r *http.Request, u User, _ Role) {
	id := r.PathValue("id")
	// Plain HTML forms can post multipart from any site: require the
	// header the interface sends, which a form cannot add.
	if r.Header.Get("X-Stratdata-Upload") != "1" {
		http.Error(w, "missing X-Stratdata-Upload header", http.StatusForbidden)
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "multipart/form-data" {
		http.Error(w, "send multipart/form-data", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxFilesPerUpload*(MaxReplaySize+1<<10))
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var (
		results []Result
		mu      sync.Mutex
		wg      sync.WaitGroup
		failed  error
		sem     = make(chan struct{}, 4) // decodes at a time
	)
	for n := 0; ; n++ {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			wg.Wait()
			return
		}
		if part.FormName() != "files" {
			part.Close()
			continue
		}
		if n >= MaxFilesPerUpload {
			http.Error(w, "too many files in one upload", http.StatusRequestEntityTooLarge)
			wg.Wait()
			return
		}
		name := filepath.Base(part.FileName())
		data, err := io.ReadAll(io.LimitReader(part, MaxReplaySize+1))
		part.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			wg.Wait()
			return
		}
		// Workers write into results while it grows: both under mu.
		mu.Lock()
		i := len(results)
		results = append(results, Result{Name: name})
		mu.Unlock()
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := s.Add(id, Upload{Name: name, Data: data, By: u.ID})
			mu.Lock()
			defer mu.Unlock()
			if err != nil && failed == nil {
				failed = err
			}
			results[i] = res
		}()
	}
	wg.Wait()
	if failed != nil {
		serverError(w, failed)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("server error: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// readJSON decodes a JSON request body; see api for why only JSON is taken.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, "send application/json", http.StatusUnsupportedMediaType)
		return false
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}
