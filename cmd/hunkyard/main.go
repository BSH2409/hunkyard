package main

import (
	"fmt"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"os"

	integrationassets "github.com/BSH2409/hunkyard/integrations"
	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/session"
	webassets "github.com/BSH2409/hunkyard/web"
)

var frontendFS = webassets.FS
var integrationsFS = integrationassets.FS

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	localonly.Install()
	if err := localonly.ValidateArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(os.Args) < 2 {
		clicmd.Exit(session.RunReview(nil))
		return
	}
	if handled, err := dispatchCLI(os.Args[1:]); handled {
		clicmd.Exit(err)
		return
	}
	args := resolveAtPrefixedArgs(os.Args[1:])
	runPositionalCLI(args)
}
