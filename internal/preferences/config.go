package preferences

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Root  string
	Clock func() time.Time

	explicitRoot bool
}

// resolveConfig defaults the root to $LEARN_PREFERENCES_HOME, then ~/.agents,
// the same canonical store the learn-preferences skill has always used.
func resolveConfig(config Config) (Config, error) {
	config.explicitRoot = config.Root != ""
	if config.Root == "" {
		config.Root = os.Getenv("LEARN_PREFERENCES_HOME")
		if strings.TrimSpace(config.Root) == "" {
			home, err := os.UserHomeDir()
			if err != nil || strings.TrimSpace(home) == "" {
				current, lookupErr := user.Current()
				if lookupErr != nil {
					return Config{}, lookupErr
				}
				home = current.HomeDir
				if home == "" {
					return Config{}, fmt.Errorf("home directory is unavailable")
				}
			}
			config.Root = filepath.Join(home, ".agents")
		}
	}
	if strings.TrimSpace(config.Root) == "" {
		return Config{}, fmt.Errorf("preference root cannot be empty")
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return Config{}, err
	}
	if filepath.Dir(root) == root {
		return Config{}, fmt.Errorf("preference root cannot be the filesystem root")
	}
	config.Root = root
	if config.Clock == nil {
		config.Clock = func() time.Time { return time.Now().UTC() }
	}
	return config, nil
}
