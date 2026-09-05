// Package buildinfo expose les métadonnées de version de l'application,
// renseignées via -ldflags lors de la compilation (voir Makefile).
package buildinfo

import (
	"runtime"
)

// Valeurs injectées à la compilation via -ldflags "-X ...".
var (
	Version = "dev"
	Commit  = "n/a"
	Date    = ""
)

// Info décrit l'application pour l'endpoint /ops/info.
func Info() map[string]string {
	return map[string]string{
		"name":    "evt2sse",
		"version": Version,
		"commit":  Commit,
		"date":    Date,
		"go":      runtime.Version(),
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
	}
}
