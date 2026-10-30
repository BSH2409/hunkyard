package server_test

import (
	"testing"

	"github.com/BSH2409/hunkyard/internal/server"
)

func TestResolveDaemonCLIConfig_RemoteFlagIgnoredWithoutFocus(t *testing.T) {
	sc, err := server.ResolveDaemonCLIConfig([]string{"--remote", "file.md"})
	if err != nil {
		t.Fatal(err)
	}
	if sc.RemoteFiles {
		t.Errorf("expected RemoteFiles=false without --pr/--range, got %+v", sc)
	}
}

func TestResolveDaemonCLIConfig_RemoteDefaultsFalse(t *testing.T) {
	sc, err := server.ResolveDaemonCLIConfig([]string{"file.md"})
	if err != nil {
		t.Fatal(err)
	}
	if sc.RemoteFiles {
		t.Errorf("expected RemoteFiles=false by default, got %+v", sc)
	}
}
