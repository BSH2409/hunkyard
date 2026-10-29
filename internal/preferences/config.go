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
}

func resolveConfig(config Config) (Config, error) {
	if config.Root == "" {
		config.Root = os.Getenv("CODEX_HOME")
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
			config.Root = filepath.Join(home, ".codex")
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
