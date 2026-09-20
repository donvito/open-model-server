// Command modelserver is the self-hosted small model server.
package main

import (
	"os"

	"github.com/donvito/modelserver/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:]))
}
