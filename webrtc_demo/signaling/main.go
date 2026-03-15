package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"opus_lab/webrtc_demo/internal/signal"
)

type sessionState struct {
	mu        sync.RWMutex
	Offer     *signal.SDP
	Answer    *signal.SDP
	CreatedAt time.Time
}

type memoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionState
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		sessions: map[string]*sessionState{},
	}
}

func (s *memoryStore) createSession(id string) *sessionState {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state, ok := s.sessions[id]; ok {
		return state
	}
	state := &sessionState{
		CreatedAt: time.Now(),
	}
	s.sessions[id] = state
	return state
}

func (s *memoryStore) getSession(id string) (*sessionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.sessions[id]
	return state, ok
}

func main() {
	var addr string
	flag.StringVar(&addr, "addr", ":8090", "HTTP listen address")
	flag.Parse()

	store := newMemoryStore()
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req signal.SessionResponse
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.SessionID) == "" {
			http.Error(w, "session_id is required", http.StatusBadRequest)
			return
		}

		store.createSession(req.SessionID)
		writeJSON(w, signal.SessionResponse{SessionID: req.SessionID})
	})

	mux.HandleFunc("/api/session/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/session/")
		parts := strings.Split(path, "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		sessionID, target := parts[0], parts[1]
		if strings.TrimSpace(sessionID) == "" {
			http.Error(w, "empty session id", http.StatusBadRequest)
			return
		}

		state, ok := store.getSession(sessionID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		switch target {
		case "offer":
			handleSDP(w, r, state, true)
		case "answer":
			handleSDP(w, r, state, false)
		default:
			http.NotFound(w, r)
		}
	})

	log.Printf("signaling server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("signaling server failed: %v", err)
	}
}

func handleSDP(w http.ResponseWriter, r *http.Request, state *sessionState, isOffer bool) {
	switch r.Method {
	case http.MethodPost:
		var req signal.SDP
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid sdp payload: %v", err), http.StatusBadRequest)
			return
		}
		if req.SDP == "" || req.Type == "" {
			http.Error(w, "invalid sdp payload", http.StatusBadRequest)
			return
		}
		state.mu.Lock()
		if isOffer {
			state.Offer = &req
		} else {
			state.Answer = &req
		}
		state.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	case http.MethodGet:
		var payload *signal.SDP
		state.mu.RLock()
		if isOffer {
			payload = state.Offer
		} else {
			payload = state.Answer
		}
		state.mu.RUnlock()
		if payload == nil {
			http.Error(w, "not ready", http.StatusNotFound)
			return
		}
		writeJSON(w, payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json failed: %v", err)
	}
}
