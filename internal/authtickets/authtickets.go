// Package authtickets implements short-lived, single-use WebSocket
// authentication tickets. It replaces the previous band-aid of
// putting the JWT in the WebSocket query string (which then ended
// up in access logs, proxy logs, browser history and Referer
// headers) with a proper ticket exchange:
//
//  1. The dashboard calls POST /api/ws-ticket (auth via Authorization
//     Bearer JWT) and receives a one-time 32-char ticket valid for
//     30 seconds.
//  2. The dashboard opens the WebSocket:
//     wss://host/api/devices/{id}/ssh?ticket=<ticket>
//  3. The server redeems the ticket: validates it, marks it used,
//     and lets the upgrade proceed. The ticket is gone from memory
//     after the single use.
//  4. Any further authentication (e.g. resolving the device's
//     tenant schema) is done from the redeemed ticket's stored
//     username + role rather than from a fresh JWT parse.
//
// Tickets never reach the log file: the URL the upgrader sees is
//
//	/api/devices/{id}/ssh?ticket=<redacted-by-us>
package authtickets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// DefaultTicketTTL is the lifetime of a freshly-issued ticket. Set
// low so a leaked ticket has a short blast radius; the dashboard
// should fetch a new ticket immediately before opening the WS.
const DefaultTicketTTL = 30 * time.Second

// Errors returned by Consume / Validate. Exported so the HTTP
// handler can map them to specific status codes.
var (
	ErrTicketNotFound = errors.New("auth ticket: not found or already consumed")
	ErrTicketExpired  = errors.New("auth ticket: expired")
	ErrTicketReused   = errors.New("auth ticket: already used")
	ErrTicketScope    = errors.New("auth ticket: wrong device")
)

// Ticket is the in-memory representation of an issued ticket. It keeps the
// issuing identity and resource scope so the WebSocket middleware can
// authenticate without re-parsing the JWT.
type Ticket struct {
	Username     string
	Role         string
	DeviceID     string
	TenantSchema string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	Consumed     bool
	ConsumedAt   time.Time
}

// Store is the process-wide in-memory ticket registry. A single
// instance is created by LoadStore and accessed via GetStore().
type Store struct {
	mu      sync.Mutex
	tickets map[string]*Ticket
	ttl     time.Duration
}

// globalStore is the process-wide Store, initialised by LoadStore()
// at startup. nil until then.
var globalStore *Store

// LoadStore initialises the process-wide ticket store with the
// supplied TTL. If ttl is zero, DefaultTicketTTL is used.
func LoadStore(ttl time.Duration) *Store {
	if ttl == 0 {
		ttl = DefaultTicketTTL
	}
	s := &Store{
		tickets: make(map[string]*Ticket),
		ttl:     ttl,
	}
	globalStore = s
	return s
}

// GetStore returns the process-wide ticket store, or nil if
// LoadStore has not been called. Handlers should fail closed
// (return 503) if this returns nil.
func GetStore() *Store { return globalStore }

// Issue creates a new single-use ticket for the given user, role, tenant, and
// device, then returns the ticket ID. The ticket has the configured TTL.
func (s *Store) Issue(username, role, deviceID, tenantSchema string) (string, *Ticket, error) {
	if s == nil {
		return "", nil, errors.New("auth ticket store not initialised")
	}
	id, err := newTicketID()
	if err != nil {
		return "", nil, err
	}
	now := time.Now()
	t := &Ticket{
		Username:     username,
		Role:         role,
		DeviceID:     deviceID,
		TenantSchema: tenantSchema,
		IssuedAt:     now,
		ExpiresAt:    now.Add(s.ttl),
	}
	s.mu.Lock()
	s.tickets[id] = t
	s.gcLocked(now)
	s.mu.Unlock()
	return id, t, nil
}

// Validate checks that the ticket exists, has not been consumed,
// and has not expired. Returns the live *Ticket on success.
func (s *Store) Validate(id string) (*Ticket, error) {
	if s == nil {
		return nil, errors.New("auth ticket store not initialised")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	if !ok {
		return nil, ErrTicketNotFound
	}
	if t.Consumed {
		return nil, ErrTicketReused
	}
	if time.Now().After(t.ExpiresAt) {
		return nil, ErrTicketExpired
	}
	return t, nil
}

// Consume atomically validates and marks a ticket as used without checking a
// device scope. Device-bound WebSocket callers should use ConsumeForDevice.
func (s *Store) Consume(id string) (*Ticket, error) {
	return s.consume(id, "", false)
}

// ConsumeForDevice atomically validates, scopes, and marks a ticket as used.
// A scope mismatch does not consume the ticket so the intended WebSocket can
// still redeem it after an unrelated request presents the ticket elsewhere.
func (s *Store) ConsumeForDevice(id, deviceID string) (*Ticket, error) {
	return s.consume(id, deviceID, true)
}

func (s *Store) consume(id, deviceID string, enforceDevice bool) (*Ticket, error) {
	if s == nil {
		return nil, errors.New("auth ticket store not initialised")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	if !ok {
		return nil, ErrTicketNotFound
	}
	if t.Consumed {
		return nil, ErrTicketReused
	}
	if time.Now().After(t.ExpiresAt) {
		return nil, ErrTicketExpired
	}
	if enforceDevice && (t.DeviceID == "" || t.TenantSchema == "" || t.DeviceID != deviceID) {
		return nil, ErrTicketScope
	}
	t.Consumed = true
	t.ConsumedAt = time.Now()
	return t, nil
}

// gcLocked removes expired tickets from the map, including tickets that were
// never consumed. Caller must hold s.mu.
func (s *Store) gcLocked(now time.Time) {
	for id, t := range s.tickets {
		if now.After(t.ExpiresAt) {
			delete(s.tickets, id)
		}
	}
}

// StartGC runs a background goroutine that periodically removes
// expired tickets. Returns immediately if the store is
// nil. The goroutine exits when the provided channel is closed.
func (s *Store) StartGC(interval time.Duration, stop <-chan struct{}) {
	if s == nil {
		return
	}
	if interval == 0 {
		interval = time.Minute
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				s.mu.Lock()
				s.gcLocked(now)
				s.mu.Unlock()
			}
		}
	}()
}

// newTicketID returns a 32-character hex string (128 bits of
// entropy). crypto/rand failures here are exceptional (kernel
// CSPRNG exhausted) and the caller is expected to surface the
// error as 500.
func newTicketID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
