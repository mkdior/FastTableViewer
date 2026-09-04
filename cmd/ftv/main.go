// Command ftv is a fast table viewer for delimited files in the terminal.
package main

import "github.com/mkdior/FastTableViewer/internal/app"

// version is the release version; overridden at build time with
// -ldflags "-X main.version=...".
var version = "0.10.0"

func main() {
	app.Execute(version)
}
