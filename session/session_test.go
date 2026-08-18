package session

import (
	"testing"
	"time"
)

func TestSessionLockSerializesAccess(t *testing.T) {
	session := &Session{}
	session.Lock()
	acquired := make(chan struct{})
	go func() {
		session.Lock()
		close(acquired)
		session.Unlock()
	}()

	select {
	case <-acquired:
		t.Fatal("second lock acquired before first lock released")
	case <-time.After(20 * time.Millisecond):
	}

	session.Unlock()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second lock did not acquire after release")
	}
}
