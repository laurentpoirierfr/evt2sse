// Package relay maintient des connexions PostgreSQL en LISTEN sur un ou
// plusieurs canaux et relaie chaque notification reçue vers ses abonnés.
// Il fournit aussi Publish pour émettre un pg_notify via un pool séparé
// (jamais en concurrence avec les connexions de LISTEN).
package relay

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notify est une notification PostgreSQL NOTIFY reçue.
type Notify struct {
	Channel string
	Payload string
	Time    time.Time
}

// Relay relaie les notifications des canaux écoutés vers ses watchers.
type Relay struct {
	connStr        string
	defaultChannel string
	mu             sync.Mutex
	watchers       map[chan Notify]struct{}
	subs           map[string]*sub
	pool           *pgxpool.Pool
	closed         bool
}

type sub struct {
	conn   *pgx.Conn
	cancel context.CancelFunc
}

func New(connStr, defaultChannel string) *Relay {
	return &Relay{
		connStr:        connStr,
		defaultChannel: defaultChannel,
		watchers:       make(map[chan Notify]struct{}),
		subs:           make(map[string]*sub),
	}
}

// Start ouvre le pool d'écriture et s'abonne au canal par défaut.
func (r *Relay) Start() error {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, r.connStr)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.pool = pool
	r.closed = false
	r.mu.Unlock()

	if err := r.SubscribeChannel(r.defaultChannel); err != nil {
		pool.Close()
		return err
	}
	return nil
}

func (r *Relay) DefaultChannel() string { return r.defaultChannel }

// SubscribeChannel ouvre un LISTEN PostgreSQL sur name (idempotent).
func (r *Relay) SubscribeChannel(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.pool == nil {
		return nil
	}
	if _, ok := r.subs[name]; ok {
		return nil
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, r.connStr)
	if err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{name}.Sanitize()); err != nil {
		conn.Close(ctx)
		return err
	}

	subCtx, cancel := context.WithCancel(ctx)
	r.subs[name] = &sub{conn: conn, cancel: cancel}
	go r.listenLoop(subCtx, name)
	return nil
}

// UnsubscribeChannel ferme l'écoute de name (idempotent).
func (r *Relay) UnsubscribeChannel(name string) error {
	r.mu.Lock()
	s, ok := r.subs[name]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.subs, name)
	r.mu.Unlock()

	s.cancel()
	s.conn.Close(context.Background())
	return nil
}

// Channels renvoie les canaux actuellement écoutés, triés.
func (r *Relay) Channels() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.subs))
	for name := range r.subs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Connected indique que le relais est démarré et non fermé.
func (r *Relay) Connected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && r.pool != nil
}

func (r *Relay) Close() error {
	r.mu.Lock()
	r.closed = true
	subs := r.subs
	r.subs = make(map[string]*sub)
	pool := r.pool
	r.mu.Unlock()

	for _, s := range subs {
		s.cancel()
		s.conn.Close(context.Background())
	}
	if pool != nil {
		pool.Close()
	}
	return nil
}

// listenLoop relaie les notifications du canal name vers les watchers.
func (r *Relay) listenLoop(ctx context.Context, name string) {
	for {
		s := r.sub(name)
		if s == nil {
			return
		}
		n, err := s.conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // désabonné ou arrêt
			}
			log.Printf("relay: canal %q: %v; reconnexion dans 3s", name, err)
			time.Sleep(3 * time.Second)
			if err := r.reconnectChannel(name); err != nil {
				log.Printf("relay: reconnexion canal %q échouée: %v", name, err)
				continue
			}
			return
		}
		r.dispatch(Notify{
			Channel: name,
			Payload: n.Payload,
			Time:    time.Now().UTC(),
		})
	}
}

func (r *Relay) reconnectChannel(name string) error {
	r.mu.Lock()
	s, ok := r.subs[name]
	if !ok {
		r.mu.Unlock()
		return nil
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, r.connStr)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{name}.Sanitize()); err != nil {
		conn.Close(ctx)
		r.mu.Unlock()
		return err
	}

	s.cancel()
	old := s.conn
	subCtx, cancel := context.WithCancel(ctx)
	s.conn = conn
	s.cancel = cancel
	r.mu.Unlock()

	old.Close(ctx)
	go r.listenLoop(subCtx, name)
	return nil
}

func (r *Relay) sub(name string) *sub {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.subs[name]
}

// dispatch envoie n aux watchers (non bloquant).
func (r *Relay) dispatch(n Notify) {
	r.mu.Lock()
	for ch := range r.watchers {
		select {
		case ch <- n:
		default:
		}
	}
	r.mu.Unlock()
}

// Notifications enregistre un watcher et renvoie la fonction de retrait.
func (r *Relay) Notifications() (<-chan Notify, func()) {
	ch := make(chan Notify, 64)
	r.mu.Lock()
	r.watchers[ch] = struct{}{}
	r.mu.Unlock()

	return ch, func() {
		r.mu.Lock()
		delete(r.watchers, ch)
		close(ch)
		r.mu.Unlock()
	}
}

// Publish émet un NOTIFY PostgreSQL sur le canal donné.
func (r *Relay) Publish(ctx context.Context, channel, payload string) error {
	if channel == "" {
		channel = r.defaultChannel
	}
	_, err := r.pool.Exec(ctx,
		"SELECT pg_notify("+quoteLiteral(channel)+", "+quoteLiteral(payload)+")")
	return err
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
