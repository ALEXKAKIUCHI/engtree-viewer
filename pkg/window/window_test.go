package window

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLife(t *testing.T) {
	l := NewLife("/api/closing")
	l.Grace, l.Idle = 50*time.Millisecond, time.Hour
	var served int
	h := l.Track(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { served++ }))
	get := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code
	}
	get("/api/status")
	if l.Done() || served != 1 {
		t.Fatal("a fresh window is not done")
	}
	if get("/api/closing") != http.StatusNoContent || served != 1 {
		t.Fatal("closing is answered by Track, not passed on")
	}
	get("/") // a reload comes back in time
	time.Sleep(80 * time.Millisecond)
	if l.Done() {
		t.Fatal("a reload cancels closing")
	}
	get("/api/closing")
	time.Sleep(80 * time.Millisecond)
	if !l.Done() {
		t.Fatal("closed and not back: done")
	}
	l2 := NewLife("/x")
	l2.Idle = 30 * time.Millisecond
	time.Sleep(50 * time.Millisecond)
	if !l2.Done() {
		t.Fatal("idle too long: done")
	}
}
