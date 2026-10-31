package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/BSH2409/hunkyard/internal/server"
)

// runYard serves the Preference Yard on its own, without a review session.
// It listens on loopback by default because the Yard shows personal rules.
func runYard(args []string) {
	fs := flag.NewFlagSet("yard", flag.ExitOnError)
	port := fs.Int("port", 0, "Port to listen on (0 picks a free port)")
	host := fs.String("host", "127.0.0.1", "Interface to listen on")
	noOpen := fs.Bool("no-open", false, "Do not open a browser")
	_ = fs.Parse(args)
	listener, err := bindListener(*host, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting the Preference Yard: %v\n", err)
		os.Exit(1)
	}
	actual := listener.Addr().(*net.TCPAddr).Port
	srv, err := server.NewServer(nil, frontendFS, "", version, actual, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating the Preference Yard: %v\n", err)
		os.Exit(1)
	}
	srv.SetListenHost(*host)
	url := "http://localhost:" + strconv.Itoa(actual) + "/yard"
	fmt.Fprintf(os.Stderr, "Preference Yard at %s (Ctrl+C to stop)\n", url)
	if !*noOpen {
		openBrowser(url, "")
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		_ = listener.Close()
	}()
	if err := http.Serve(listener, srv); err != nil && !isClosedListener(err) {
		fmt.Fprintf(os.Stderr, "Preference Yard stopped: %v\n", err)
		os.Exit(1)
	}
}

func isClosedListener(err error) bool {
	return errors.Is(err, net.ErrClosed)
}
