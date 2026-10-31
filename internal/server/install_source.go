package server

import (
	"os"
	"path/filepath"
	"strings"
)

// installationSource describes how the running binary was installed, so
// Updates can show the right update step.
const (
	installationSourceUnknown = "unknown"
	installationSourceGo      = "go"
	installationSourceNix     = "nix"
	installationSourceRelease = "release" // an official release binary
	installationSourceSource  = "source"  // built from a checkout
)

// buildDistribution is "release" in official release binaries, stamped by
// the release workflow through `make build-all DISTRIBUTION=release`
// (-X .../internal/server.buildDistribution=release). Other builds leave it
// empty.
var buildDistribution string

var executablePath = os.Executable

func detectInstallationSource() string {
	path, err := executablePath()
	if err != nil {
		return installationSourceUnknown
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	home, _ := os.UserHomeDir()
	return installationSourceForPath(path, os.Getenv("GOBIN"), os.Getenv("GOPATH"), home, buildDistribution)
}

// installationSourceForPath prefers the Nix store, then the release stamp,
// then a Go bin directory (`go install` builds carry no stamp); any other
// unstamped binary was built from a checkout.
func installationSourceForPath(path, goBin, goPath, home, distribution string) string {
	cleanPath := filepath.Clean(path)
	portablePath := filepath.ToSlash(cleanPath)

	if strings.HasPrefix(portablePath, "/nix/store/") {
		return installationSourceNix
	}
	if distribution == "release" {
		return installationSourceRelease
	}

	goBins := []string{goBin}
	if goPath != "" {
		for _, root := range filepath.SplitList(goPath) {
			goBins = append(goBins, filepath.Join(root, "bin"))
		}
	}
	if home != "" {
		goBins = append(goBins, filepath.Join(home, "go", "bin"))
	}
	for _, bin := range goBins {
		if bin != "" && filepath.Dir(cleanPath) == filepath.Clean(bin) {
			return installationSourceGo
		}
	}
	return installationSourceSource
}
