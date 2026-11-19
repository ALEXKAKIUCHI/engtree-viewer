// Command engtreecorte is EngTree Corte: the cut planner on its own, to
// take to the shop floor before it is part of EngTree. Parts and stock are
// typed or pasted, the plan is calculated and drawn, and it is exported as
// the PDF report and, for sheets, the files of the Giben saw.
//
// It opens in its own window, like EngTree, and stops when the window is
// closed. Plans are kept in the window (and can be saved to a file); the
// program writes nothing on disk by itself.
//
//	go run ./cmd/engtreecorte              (opens the window)
//	go run ./cmd/engtreecorte -no-window   (prints the address; for development)
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/window"
)

// version is set when releasing: go build -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	port := flag.Int("port", 0, "fixed local port (default: a free one)")
	noWindow := flag.Bool("no-window", false, "do not open a window and do not stop when idle (for development)")
	flag.Parse()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + ln.Addr().String()
	handler := newServer().routes()
	if *noWindow {
		fmt.Println("EngTree Corte:", url, "(Ctrl+C to quit)")
	} else {
		life := window.NewLife("/api/closing")
		handler = life.Track(handler)
		go window.Launch(url)
		go life.Watch()
	}
	log.Fatal(http.Serve(ln, localOnly(handler)))
}

// localOnly refuses requests that do not come from the page itself, as
// EngTree does: another site open in the browser cannot call this server.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		origin := r.Header.Get("Origin")
		if (host != "127.0.0.1" && host != "localhost") ||
			r.Header.Get("Sec-Fetch-Site") == "cross-site" ||
			(origin != "" && origin != "http://"+r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
