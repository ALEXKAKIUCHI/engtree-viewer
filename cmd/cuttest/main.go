// Command cuttest is a small test screen for the linear and rectangular
// cut planners. It opens a page in the browser where parts and stock are
// typed in, the plan is calculated and the result is drawn.
//
// It is a tool for trying the planners with real cases, not the final
// screen: there is no BOM, no material list and nothing is saved on disk.
//
//	go run ./cmd/cuttest
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan"
)

func main() {
	port := flag.Int("port", 0, "fixed local port (default: a free one)")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser")
	flag.Parse()

	cutplan.Register()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + ln.Addr().String()
	fmt.Println("cuttest:", url, "(Ctrl+C to quit)")
	if !*noBrowser {
		openBrowser(url)
	}
	log.Fatal(http.Serve(ln, newServer().routes()))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
