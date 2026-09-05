// Package server expose les routes HTTP et diffuse les notifications Postgres
// vers les clients Server-Sent Events (SSE).
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/laurentpoirierfr/evt2sse/internal/relay"
	"github.com/laurentpoirierfr/evt2sse/internal/web"
)

type sendRequest struct {
	Channel string `json:"channel,omitempty"`
	Payload string `json:"payload"`
}

// Server diffuse les notifications reçues du relay vers ses clients SSE.
type Server struct {
	relay   *relay.Relay
	channel string

	mu      sync.Mutex
	clients map[string]chan []byte
	lastID  int64
}

func New(r *relay.Relay, defaultChannel string) *Server {
	return &Server{
		relay:   r,
		channel: defaultChannel,
		clients: make(map[string]chan []byte),
	}
}

// Start abonne le serveur aux notifications du relay et les diffuse.
func (s *Server) Start(ctx context.Context) {
	ch, stop := s.relay.Notifications()
	go func() {
		defer stop()
		for {
			select {
			case <-ctx.Done():
				return
			case n, ok := <-ch:
				if !ok {
					return
				}
				s.broadcast(n)
			}
		}
	}()
}

// Handler construit le routeur HTTP.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/send", s.handleSend)
	mux.HandleFunc("GET /api/listen", s.handleListen)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/channels", s.handleChannels)
	mux.HandleFunc("POST /api/channels", s.handleSubscribe)
	mux.HandleFunc("DELETE /api/channels/{name}", s.handleUnsubscribe)
	mux.HandleFunc("GET /", s.handleIndex)
	return mux
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"body JSON invalide"}`, http.StatusBadRequest)
		return
	}
	channel := req.Channel
	if channel == "" {
		channel = s.channel
	}

	if err := s.relay.Publish(r.Context(), channel, req.Payload); err != nil {
		log.Printf("server: notification échouée: %v", err)
		http.Error(w, `{"error":"échec de notification"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"channel": channel,
		"payload": req.Payload,
	})
}

func (s *Server) handleListen(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE non supporté", http.StatusInternalServerError)
		return
	}

	clientID := newClientID(r)
	ch := make(chan []byte, 64)

	s.mu.Lock()
	s.clients[clientID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, clientID)
		s.mu.Unlock()
		close(ch)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ready, _ := json.Marshal(map[string]any{
		"connected": true,
		"channel":   s.channel,
	})
	w.Write([]byte("retry: 3000\n"))
	w.Write([]byte("event: ready\ndata: " + string(ready) + "\n\n"))
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(evt); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	clients := len(s.clients)
	lastID := s.lastID
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"channel":   s.channel,
		"channels":  s.relay.Channels(),
		"clients":   clients,
		"last_id":   lastID,
		"connected": s.relay.Connected(),
	})
}

type channelRequest struct {
	Channel string `json:"channel"`
}

// handleChannels liste les canaux actuellement écoutés.
func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"default":  s.channel,
		"channels": s.relay.Channels(),
	})
}

// handleSubscribe active l'écoute d'un canal côté serveur.
func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"body JSON invalide"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Channel)
	if name == "" {
		http.Error(w, `{"error":"channel manquant"}`, http.StatusBadRequest)
		return
	}
	if err := s.relay.SubscribeChannel(name); err != nil {
		log.Printf("server: abonnement %q échoué: %v", name, err)
		http.Error(w, `{"error":"abonnement impossible"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": name})
}

// handleUnsubscribe ferme l'écoute d'un canal.
func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.relay.UnsubscribeChannel(name); err != nil {
		log.Printf("server: désabonnement %q échoué: %v", name, err)
		http.Error(w, `{"error":"désabonnement impossible"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": name})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(web.IndexHTML)
}

// broadcast construit le message SSE puis l'envoie non-bloquant aux clients.
func (s *Server) broadcast(n relay.Notify) {
	s.mu.Lock()
	s.lastID++
	id := s.lastID
	channel := n.Channel
	if channel == "" {
		channel = s.channel
	}
	clients := make([]chan []byte, 0, len(s.clients))
	for _, ch := range s.clients {
		clients = append(clients, ch)
	}
	s.mu.Unlock()

	msg := map[string]any{
		"id":      id,
		"channel": channel,
		"payload": n.Payload,
		"time":    n.Time.Format(time.RFC3339Nano),
	}
	data, _ := json.Marshal(msg)

	evt := []byte("id: " + strconv.FormatInt(id, 10) + "\ndata: " + string(data) + "\n\n")

	for _, ch := range clients {
		select {
		case ch <- evt:
		default:
		}
	}
}

func newClientID(r *http.Request) string {
	return r.RemoteAddr + ":" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
