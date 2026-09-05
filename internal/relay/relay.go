// Package relay maintient des connexions PostgreSQL en LISTEN sur un ou
// plusieurs canaux et relaie chaque notification reçue vers ses abonnés.
// Il fournit aussi Publish pour émettre un pg_notify via un pool séparé
// (jamais en concurrence avec les connexions de LISTEN).
package relay

import (
	"context"
	"log"
	"math/rand/v2"
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

const (
	// idemMax est le nombre maximal d'ids récents conservés pour la dédup.
	idemMax = 4096
	// idemTTL est la durée de mémorisation d'un id d'envoi.
	idemTTL = 5 * time.Minute
)

// Relay relaie les notifications des canaux écoutés vers ses watchers.
type Relay struct {
	connStr        string
	defaultChannel string
	mu             sync.Mutex
	watchers       map[chan Notify]struct{}
	subs           map[string]*sub
	dedupe         *idemStore
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
		dedupe:         newIdemStore(idemMax, idemTTL),
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

// listenLoop relaie les notifications du canal name vers les watchers,
// avec reconnexion en backoff exponentiel + jitter.
func (r *Relay) listenLoop(ctx context.Context, name string) {
	attempt := 0
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
			attempt++
			delay := backoffDelay(attempt)
			log.Printf("relay: canal %q: %v; reconnexion dans %s", name, err, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			if err := r.reconnectChannel(name); err != nil {
				log.Printf("relay: reconnexion canal %q échouée: %v", name, err)
				continue
			}
			return
		}
		attempt = 0
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
	return r.PublishWithID(ctx, channel, payload, "")
}

// PublishWithID émet un NOTIFY idempotent : si id est fourni et a déjà été
// notifié avec succès récemment, l'envoi est acquitté sans réémission (dédup).
func (r *Relay) PublishWithID(ctx context.Context, channel, payload, id string) error {
	if channel == "" {
		channel = r.defaultChannel
	}
	if id != "" && r.dedupe.seen(id) {
		return nil // doublon : acquitté silencieusement
	}
	_, err := r.pool.Exec(ctx,
		"SELECT pg_notify("+quoteLiteral(channel)+", "+quoteLiteral(payload)+")")
	if err != nil {
		return err
	}
	if id != "" {
		// Enregistrement après succès : un échec de publication ne doit pas
		// « brûler » l'id, sinon un retry serait acquitté à tort.
		r.dedupe.add(id)
	}
	return nil
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// idemStore mémorise les ids d'envois récents, borné et expirant.
type idemStore struct {
	mu  sync.Mutex
	ids map[string]time.Time
	max int
	ttl time.Duration
}

func newIdemStore(max int, ttl time.Duration) *idemStore {
	return &idemStore{ids: make(map[string]time.Time), max: max, ttl: ttl}
}

// seen renvoie true si id a déjà été enregistré (doublon possible).
func (d *idemStore) seen(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.ids[id]
	return ok
}

// add enregistre id comme notifié avec succès (avec borne de taille).
func (d *idemStore) add(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ids[id] = time.Now()
	if len(d.ids) > d.max {
		d.purgeLocked()
	}
}

func (d *idemStore) purgeLocked() {
	cut := time.Now().Add(-d.ttl)
	for k, v := range d.ids {
		if v.Before(cut) {
			delete(d.ids, k)
		}
	}
	// Borne en taille : évince les plus anciens si la capacité est dépassée
	// (le TTL seul ne suffirait pas en cas de débit soutenu).
	for len(d.ids) > d.max {
		var oldest string
		oldestT := time.Now()
		for k, v := range d.ids {
			if v.Before(oldestT) {
				oldest, oldestT = k, v
			}
		}
		delete(d.ids, oldest)
	}
}

// backoffDelay calcule un délai de reconnexion avec backoff exponentiel et
// jitter (+/-20%), borné à 30 s (plafond appliqué après le jitter).
func backoffDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	base := time.Second
	for i := 0; i < attempt && base < 30*time.Second; i++ {
		base *= 2
	}
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	jitter := base / 5
	d := base - jitter + time.Duration(rand.Int64N(int64(2*jitter)+1))
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
