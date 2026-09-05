package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ListenOption configure une écoute.
type ListenOption func(*listenConfig)

type listenConfig struct {
	reconnect bool
}

// WithAutoReconnect active (défaut) ou désactive la reconnexion automatique
// au flux SSE en cas de coupure.
func WithAutoReconnect(enable bool) ListenOption {
	return func(cfg *listenConfig) { cfg.reconnect = enable }
}

// Stream reçoit les événements d'un flux SSE.
// Copie la résilience d'un EventSource : reconnexion automatique (backoff
// exponentiel + jitter) et reprise via Last-Event-ID après coupure réseau.
type Stream struct {
	events chan Event
	errs   chan error
	done   chan struct{}
	once   sync.Once
	cancel context.CancelFunc

	client *Client
	ctx    context.Context
	lastID int64 // dernier id SSE reçu (utilisé pour la reprise)
}

// Listen ouvre un flux SSE vers /api/listen. La réception se fait sur
// Events(); les erreurs transitoires sont publiées sur Errs() (non bloquant).
func (c *Client) Listen(ctx context.Context, opts ...ListenOption) *Stream {
	cfg := listenConfig{reconnect: true}
	for _, o := range opts {
		o(&cfg)
	}

	ictx, cancel := context.WithCancel(ctx)
	s := &Stream{
		events: make(chan Event, 64),
		errs:   make(chan error, 4),
		done:   make(chan struct{}),
		cancel: cancel,
		client: c,
		ctx:    ictx,
	}
	go s.loop(cfg)
	return s
}

// Events renvoie le canal des notifications reçues. Il est fermé à l'arrêt
// du flux (contexte annulé ou Close appelée).
func (s *Stream) Events() <-chan Event { return s.events }

// Errs renvoie un canal d'erreurs non bloquant (reconnexion, HTTP, parse).
func (s *Stream) Errs() <-chan error { return s.errs }

// Close arrête la lecture et libère la connexion HTTP.
func (s *Stream) Close() {
	s.once.Do(func() {
		s.cancel()
		close(s.done)
	})
}

func (s *Stream) loop(cfg listenConfig) {
	defer close(s.events)

	attempt := 0
	for {
		delivered, err := s.runOnce()
		if s.ctx.Err() != nil || s.stopped() {
			return
		}
		if delivered {
			attempt = 0
		} else {
			attempt++
		}
		if err != nil && err != io.EOF {
			select {
			case s.errs <- err:
			default:
			}
		}
		if !cfg.reconnect {
			return
		}

		delay := backoff(attempt-1, s.client.minRecon, s.client.maxRecon)
		select {
		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (s *Stream) stopped() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// runOnce lit le flux jusqu'à sa fermeture ; io.EOF correspond à une
// fermeture propre (reconnexion silencieuse). Elle renvoie true si au moins
// un événement a été reçu.
func (s *Stream) runOnce() (bool, error) {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.client.baseURL+"/api/listen", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if s.lastID > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(s.lastID, 10))
	}

	resp, err := s.client.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return false, fmt.Errorf("listen: statut HTTP %d (%s)", resp.StatusCode, resp.Status)
	}

	br := bufio.NewReader(resp.Body)
	delivered := false
	for {
		ev, err := readSSEEvent(br)
		if err != nil {
			return delivered, err
		}
		if id, perr := strconv.ParseInt(ev.ID, 10, 64); perr == nil && id > s.lastID {
			s.lastID = id
		}
		if ev.Type != "" && ev.Type != "message" {
			continue
		}

		var d sseData
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, d.Time)
		if err != nil {
			t = time.Time{}
		}

		select {
		case s.events <- Event{ID: d.ID, Channel: d.Channel, Payload: d.Payload, Time: t}:
			delivered = true
		case <-s.ctx.Done():
			return delivered, s.ctx.Err()
		case <-s.done:
			return delivered, io.EOF
		}
	}
}

type sseData struct {
	ID      int64  `json:"id"`
	Channel string `json:"channel"`
	Payload string `json:"payload"`
	Time    string `json:"time"`
}

type sseEvent struct {
	Type string
	ID   string
	Data []byte
}

// readSSEEvent lit un événement SSE (blocs séparés par une ligne vide).
func readSSEEvent(br *bufio.Reader) (*sseEvent, error) {
	ev := &sseEvent{}
	gotData := false

	for {
		line, err := br.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(line)

		if trimmed != "" {
			if strings.HasPrefix(trimmed, ":") {
				// commentaire SSE
			} else {
				field, value, _ := strings.Cut(trimmed, ":")
				value = strings.TrimPrefix(value, " ")
				switch field {
				case "event":
					ev.Type = value
				case "id":
					ev.ID = value
				case "data":
					if len(ev.Data) > 0 {
						ev.Data = append(ev.Data, '\n')
					}
					ev.Data = append(ev.Data, value...)
					gotData = true
				}
			}
		} else if gotData {
			return ev, nil
		}

		if err != nil {
			if gotData && err == io.EOF {
				return ev, nil
			}
			return nil, err
		}
	}
}
