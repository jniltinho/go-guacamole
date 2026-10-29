// Command goguacamole is a web gateway for Apache guacd (RDP, SSH, VNC) with
// an embedded Vue 3 frontend. Go + Echo v5 + Cobra; SQLite, MariaDB/MySQL or
// PostgreSQL storage.
package main

import (
	"embed"
	"os"

	// Embed the IANA time zone database: access windows are evaluated in each
	// user's time zone and minimal images have no /usr/share/zoneinfo.
	_ "time/tzdata"

	"github.com/jniltinho/go-guacamole/cmd"
)

// web/dist is produced by `make frontend` (vp build); web/dist/.gitkeep keeps
// the directory present so the embed directive always compiles.
//
//go:embed all:web/dist
var embeddedFiles embed.FS

func main() {
	os.Exit(cmd.Execute(embeddedFiles))
}
