package daemon

import "testing"

func TestAdvertisedURLBracketsIPv6Hosts(t *testing.T) {
	if got := AdvertisedURL("", "::1", 5000, "/inbox"); got != "http://[::1]:5000/inbox" {
		t.Fatalf("AdvertisedURL = %q, want http://[::1]:5000/inbox", got)
	}
	if got := AdvertisedURL("", "127.0.0.1", 5000, ""); got != "http://localhost:5000" {
		t.Fatalf("AdvertisedURL = %q", got)
	}
}
