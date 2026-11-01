package main

import (
	"net"
	"testing"
)

func TestBindReviewListenerFallsBackOnlyFromTheDefaultPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port

	ln, err := bindReviewListener("127.0.0.1", port, true)
	if err != nil {
		t.Fatalf("defaulted port in use: %v, want a free port instead", err)
	}
	if got := ln.Addr().(*net.TCPAddr).Port; got == port {
		t.Fatalf("listener took the busy port %d", got)
	}
	ln.Close()

	if ln, err := bindReviewListener("127.0.0.1", port, false); err == nil {
		ln.Close()
		t.Fatal("an explicitly chosen busy port silently moved")
	}
}
