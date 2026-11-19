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

// Handler serves the server's API:
//
//	GET  /api/spaces                  the spaces
//	POST /api/spaces                  create one: {"name": …, "kind": "team"|"league"}
//	POST /api/spaces/{id}/replays     send replays (multipart, field "files")
//	     /api/spaces/{id}/…           the statistics API of the space (package api)
//
// There is no authentication yet: the server must only listen on the local
// machine until accounts exist.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/spaces", func(w http.ResponseWriter, r *http.Request) {
		spaces, err := s.Spaces()
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, spaces)
	})
	mux.HandleFunc("POST /api/spaces", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string    `json:"name"`
			Kind SpaceKind `json:"kind"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		sp, err := s.CreateSpace(body.Name, body.Kind)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, sp)
	})
	mux.HandleFunc("POST /api/spaces/{id}/replays", s.upload)
	mux.HandleFunc("/api/spaces/{id}/", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		v, err := s.view(id)
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "space not found", http.StatusNotFound)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		// The space's API answers under /api/, as when it runs alone.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/api/" + strings.TrimPrefix(r.URL.Path, "/api/spaces/"+id+"/")
		r2.URL.RawPath = ""
		v.ServeHTTP(w, r2)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return mux
}

// upload receives the replays of a match. Files are read one after the other
// and decoded a few at a time, so memory stays bounded whatever the number
// of files.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if ok, err := s.spaceExists(id); err != nil {
		serverError(w, err)
		return
	} else if !ok {
		http.Error(w, "space not found", http.StatusNotFound)
		return
	}
	// A multipart form is not a "simple" cross-origin request only when
	// sent with fetch and a custom header; plain HTML forms can post it.
	// Require the header the interface sends, so another site cannot.
	if r.Header.Get("X-R6-Upload") != "1" {
		http.Error(w, "missing X-R6-Upload header", http.StatusForbidden)
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
			res, err := s.Add(id, Upload{Name: name, Data: data})
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
