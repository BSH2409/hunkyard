package server

import "testing"

func TestHunkyardDefaultListener(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HUNKYARD_HOST", "")
	t.Setenv("HUNKYARD_ALLOW_UNAUTHENTICATED_NETWORK", "")
	t.Setenv("HUNKYARD_PORT", "")
	sc, err := ResolveDaemonCLIConfig([]string{"--no-open"})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Port != 5000 || sc.Host != "0.0.0.0" || !sc.AllowUnauthenticatedNetwork {
		t.Fatalf("listener = %s:%d", sc.Host, sc.Port)
	}
	explicit, err := ResolveDaemonCLIConfig([]string{"--port", "0", "--no-open"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Port != 0 {
		t.Fatalf("explicit random port became %d", explicit.Port)
	}
	loopback, err := ResolveDaemonCLIConfig([]string{"--host", "127.0.0.1", "--no-open"})
	if err != nil {
		t.Fatal(err)
	}
	if loopback.Host != "127.0.0.1" || loopback.AllowUnauthenticatedNetwork {
		t.Fatalf("loopback override = %+v", loopback)
	}
}
