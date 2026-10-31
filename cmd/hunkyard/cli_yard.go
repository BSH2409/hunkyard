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

	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/server"
)

// runYard serves the Preference Yard on its own, without a review session.
// It listens on loopback by default because the Yard shows personal rules.
func runYard(args []string) {
	fs := flag.NewFlagSet("yard", flag.ExitOnError)
	port := fs.Int("port", 0, "Port to listen on (0 picks a free port)")
	host := fs.String("host", "", "Interface to listen on (default HUNKYARD_HOST, global config host, or 127.0.0.1)")
	noOpen := fs.Bool("no-open", false, "Do not open a browser")
	_ = fs.Parse(args)
	global, _, _ := config.LoadConfigFile(config.GlobalConfigPath())
	*host = yardHost(*host, global.Host)
	if ip := net.ParseIP(*host); *host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		fmt.Fprintf(os.Stderr, "Warning: the Preference Yard is listening on %s; anyone who can reach it can read and change your rules.\n", *host)
	}
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

// yardHost picks the listen host: flag, then HUNKYARD_HOST, then the global
// config's host. Unlike a review it falls back to loopback, because the Yard
// shows and changes personal rules. Project config cannot set it.
func yardHost(flagHost, globalHost string) string {
	if host := config.ResolveHost(flagHost, globalHost); host != "" {
		return host
	}
	return "127.0.0.1"
}

func isClosedListener(err error) bool {
	return errors.Is(err, net.ErrClosed)
}
