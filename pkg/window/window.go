// Package window opens a local web page as an app window and stops the
// program when that window is closed. EngTree and EngTree Corte use it.
//
// The window is an Edge or Chrome window in "app mode" (no address bar),
// a separate process: closing it does not stop the local server by
// itself. The page sends a request to the closing handler when it unloads;
// the program exits a few seconds later unless the page comes back (a
// reload). As a safety net it also exits after some minutes without any
// request, so the page must call the server now and then while it is open.
package window

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Launch opens url in Edge or Chrome app mode, or in the default browser
// when neither is found.
func Launch(url string) {
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		for _, exe := range []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			filepath.Join(local, `Google\Chrome\Application\chrome.exe`),
		} {
			if _, err := os.Stat(exe); err == nil {
				_ = exec.Command(exe, "--app="+url, "--window-size=1400,900").Start()
				return
			}
		}
	}
	_ = OpenWithSystem(url)
}

// OpenWithSystem opens a file or URL with the program the system uses for it.
func OpenWithSystem(target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

// Life tracks whether the window is still open.
type Life struct {
	ClosingPath string        // the request the page sends on unload, e.g. "/api/closing"
	Grace       time.Duration // how long after closing to wait for a reload
	Idle        time.Duration // exit after this long without any request

	mu        sync.Mutex
	lastSeen  time.Time
	closingAt time.Time
}

// NewLife uses EngTree's timings: 5 seconds to reload, 3 minutes idle.
func NewLife(closingPath string) *Life {
	return &Life{ClosingPath: closingPath, Grace: 5 * time.Second, Idle: 3 * time.Minute, lastSeen: time.Now()}
}

// Track wraps the server: every request but the closing one says the
// window is alive, and the closing one starts the countdown.
func (l *Life) Track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		if r.URL.Path == l.ClosingPath {
			l.closingAt = time.Now()
			l.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		l.lastSeen = time.Now()
		l.closingAt = time.Time{}
		l.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

// Done says whether the program should stop now.
func (l *Life) Done() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	closing := !l.closingAt.IsZero() && time.Since(l.closingAt) > l.Grace
	return closing || time.Since(l.lastSeen) > l.Idle
}

// Watch exits the program once Done. Run it in its own goroutine.
func (l *Life) Watch() {
	for range time.Tick(time.Second) {
		if l.Done() {
			os.Exit(0)
		}
	}
}
