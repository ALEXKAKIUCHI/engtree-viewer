// Command engtree starts the EngTree Viewer, the free version.
//
// The program itself lives in the app package, so that another build can
// register extra features (see the extension package) and start the same
// program. This main only exists to be the Windows executable: it is also
// where rsrc_windows_amd64.syso sits, the file that carries the icon.
package main

import "github.com/ALEXKAKIUCHI/engtree-viewer/app"

func main() {
	app.Run()
}
