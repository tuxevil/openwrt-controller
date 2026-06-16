package authtickets

import (
	"sync"
	"testing"
	"time"
)

func TestIssueAndConsume(t *testing.T) {
	s := LoadStore(time.Minute)
	id, t0, err := s.Issue("alice", "ADMIN")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("ticket id length = %d, want 32", len(id))
	}
	if t0.Username != "alice" || t0.Role != "ADMIN" {
		t.Errorf("ticket = %+v, want user=alice role=ADMIN", t0)
	}
	if t0.Consumed {
		t.Error("freshly issued ticket should not be consumed")
	}

	// First Consume succeeds.
	got, err := s.Consume(id)
	if err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if got.Username != "alice" {
		t.Errorf("consumed ticket username = %q, want alice", got.Username)
	}

	// Second Consume fails.
	if _, err := s.Consume(id); err != ErrTicketReused {
		t.Errorf("second Consume err = %v, want ErrTicketReused", err)
	}
}

func TestValidateDoesNotConsume(t *testing.T) {
	s := LoadStore(time.Minute)
	id, _, _ := s.Issue("bob", "OPERATOR")
	// Validate is read-only.
	if _, err := s.Validate(id); err != nil {
		t.Errorf("Validate #1: %v", err)
	}
	if _, err := s.Validate(id); err != nil {
		t.Errorf("Validate #2 (idempotent): %v", err)
	}
	// Consume still works.
	if _, err := s.Consume(id); err != nil {
		t.Errorf("Consume after Validate: %v", err)
	}
}

func TestExpiredTicket(t *testing.T) {
	s := LoadStore(10 * time.Millisecond)
	id, _, _ := s.Issue("carol", "VIEWER")
	time.Sleep(20 * time.Millisecond)
	if _, err := s.Validate(id); err != ErrTicketExpired {
		t.Errorf("Validate after expiry err = %v, want ErrTicketExpired", err)
	}
	if _, err := s.Consume(id); err != ErrTicketExpired {
		t.Errorf("Consume after expiry err = %v, want ErrTicketExpired", err)
	}
}

func TestUnknownTicket(t *testing.T) {
	s := LoadStore(time.Minute)
	if _, err := s.Validate("never-issued"); err != ErrTicketNotFound {
		t.Errorf("Validate unknown err = %v, want ErrTicketNotFound", err)
	}
	if _, err := s.Consume("never-issued"); err != ErrTicketNotFound {
		t.Errorf("Consume unknown err = %v, want ErrTicketNotFound", err)
	}
}

func TestConcurrentConsume(t *testing.T) {
	// Two goroutines racing to consume the same ticket: exactly one
	// must win, the other must see ErrTicketReused.
	s := LoadStore(time.Minute)
	id, _, _ := s.Issue("dave", "ADMIN")
	var (
		wg          sync.WaitGroup
		successes   int
		varMu       sync.Mutex
	)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Consume(id); err == nil {
				varMu.Lock()
				successes++
				varMu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Errorf("consume races: %d successes, want exactly 1", successes)
	}
}
