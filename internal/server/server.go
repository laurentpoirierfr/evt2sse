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
	ID      string `json:"id,omitempty"`
}

// Server diffuse les notifications reçues du relay vers ses clients SSE.
type Server struct {
	relay   *relay.Relay
	channel string

	mu      sync.Mutex
	clients map[string]chan []byte
	lastID  int64

	// history : tampon borné des derniers événements, permet la reprise
	// (Last-Event-ID) après une coupure réseau.
	history []historyEntry
}

type historyEntry struct {
	id    int64
	frame []byte
}

const (
	// historyRetention est le nombre d'événements conservés pour la reprise.
	historyRetention = 256
	// maxBodyBytes borne les corps de requêtes POST.
	maxBodyBytes = 64 << 10 // 64 KiB
	// heartbeatInterval évite les timeouts de proxy et détecte les coupures.
	heartbeatInterval = 15 * time.Second
)

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

// Handler construit le routeur HTTP (avec lecture de panique).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/send", s.handleSend)
	mux.HandleFunc("GET /api/listen", s.handleListen)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/channels", s.handleChannels)
	mux.HandleFunc("POST /api/channels", s.handleSubscribe)
	mux.HandleFunc("DELETE /api/channels/{name}", s.handleUnsubscribe)
	mux.HandleFunc("GET /", s.handleIndex)
	return s.recoverPanic(mux)
}

// recoverPanic protège le serveur contre une panique dans un handler.
func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("server: panique récupérée sur %s %s: %v", r.Method, r.URL.Path, rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"body JSON invalide"}`, http.StatusBadRequest)
		return
	}
	channel := req.Channel
	if channel == "" {
		channel = s.channel
	}

	if err := s.relay.PublishWithID(r.Context(), channel, req.Payload, req.ID); err != nil {
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

	// Reprise après coupure : le client annonce le dernier événement reçu.
	var resumeResult replayResult
	if lastID := parseLastEventID(r.Header.Get("Last-Event-ID")); lastID > 0 {
		s.mu.Lock()
		resumeResult = s.replayAfter(lastID)
		s.mu.Unlock()
	}

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
	w.Write([]byte("retry: 1000\n"))
	w.Write([]byte("event: ready\ndata: " + string(ready) + "\n\n"))

	// Rejeu des événements manqués pendant la coupure.
	if resumeResult.frames != nil {
		meta, _ := json.Marshal(map[string]any{
			"from":  resumeResult.from,
			"count": len(resumeResult.frames),
			"gap":   !resumeResult.contiguous,
		})
		w.Write([]byte("event: resume\ndata: " + string(meta) + "\n\n"))
		for _, frame := range resumeResult.frames {
			if _, err := w.Write(frame); err != nil {
				return
			}
		}
	}
	flusher.Flush()

	hb := time.NewTicker(heartbeatInterval)
	defer hb.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-hb.C:
			// Commentaire SSE : garde la connexion vivante, donne un signal
			// détectable d'écriture échouée (coupure silencieuse du client).
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
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
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
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

	msg := map[string]any{
		"id":      id,
		"channel": channel,
		"payload": n.Payload,
		"time":    n.Time.Format(time.RFC3339Nano),
	}
	data, _ := json.Marshal(msg)

	evt := []byte("id: " + strconv.FormatInt(id, 10) + "\ndata: " + string(data) + "\n\n")
	s.pushHistory(id, evt)
	s.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- evt:
		default:
		}
	}
}

const maxLastEventIDLen = 24

func parseLastEventID(h string) int64 {
	if len(h) == 0 || len(h) > maxLastEventIDLen {
		return 0
	}
	id, err := strconv.ParseInt(h, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

type replayResult struct {
	frames     [][]byte
	from       int64
	contiguous bool
}

// pushHistory ajoute un événement au tampon de reprise (borné).
// À appeler en tenant s.mu.
func (s *Server) pushHistory(id int64, frame []byte) {
	s.history = append(s.history, historyEntry{id: id, frame: frame})
	if len(s.history) > historyRetention {
		s.history = s.history[len(s.history)-historyRetention:]
	}
}

// replayAfter renvoie les trames postérieures à lastID (incluses) pour la
// reprise. contiguous indique si lastID est encore dans le tampon.
// À appeler en tenant s.mu.
func (s *Server) replayAfter(lastID int64) replayResult {
	if lastID <= 0 || len(s.history) == 0 {
		return replayResult{contiguous: true}
	}
	idx := -1
	for i := range s.history {
		if s.history[i].id == lastID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return replayResult{contiguous: false} // trou dans l'historique
	}
	out := make([][]byte, 0, len(s.history)-idx-1)
	for i := idx + 1; i < len(s.history); i++ {
		out = append(out, s.history[i].frame)
	}
	return replayResult{
		frames:     out,
		from:       lastID,
		contiguous: true,
	}
}

func newClientID(r *http.Request) string {
	return r.RemoteAddr + ":" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
