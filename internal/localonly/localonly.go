// Package localonly enforces the local-only runtime boundary.
package localonly

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
)

var installed atomic.Bool

// Enabled reports whether the executable installed the local runtime boundary.
func Enabled() bool { return installed.Load() }

// ContentSecurityPolicy prevents reviewed content and embedded assets from loading remote resources.
const ContentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' blob:; worker-src 'self' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self' http://127.0.0.1:* http://localhost:* ws://127.0.0.1:* ws://localhost:*; frame-src 'self' http://127.0.0.1:* http://localhost:*; object-src 'none'; base-uri 'self'; form-action 'self'"

// PreviewContentSecurityPolicy allows the local preview to load the local agent bundle.
var PreviewContentSecurityPolicy = strings.Replace(ContentSecurityPolicy, "script-src 'self'", "script-src 'self' http://127.0.0.1:* http://localhost:*", 1)

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CheckURL rejects non-loopback destinations without performing DNS resolution.
func CheckURL(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ws" && u.Scheme != "wss") || !loopback(u.Hostname()) {
		return fmt.Errorf("local-only build: external destination is unavailable")
	}
	return nil
}

// NewTransport bypasses proxies and rejects non-loopback connections before DNS or dialing.
func NewTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || !loopback(host) {
			return nil, fmt.Errorf("local-only build: external connections are unavailable")
		}
		if host == "localhost" {
			host = "127.0.0.1"
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
	}
	return transport
}

// Install applies the process HTTP boundary and prevents Git from fetching missing objects.
func Install() {
	installed.Store(true)
	http.DefaultTransport = NewTransport()
	for key, value := range map[string]string{"GIT_NO_LAZY_FETCH": "1", "GIT_OPTIONAL_LOCKS": "0", "GIT_TERMINAL_PROMPT": "0"} {
		_ = os.Setenv(key, value)
	}
	appendGitConfig("core.fsmonitor", "false")
	appendGitConfig("diff.external", "")
	appendGitConfig("diff.submodule", "short")
	appendGitConfig("submodule.recurse", "false")
	// Git's clean/process filters can run commands even during a read-only diff.
	keys, _ := exec.Command("git", "config", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`).Output()
	for _, key := range strings.Split(string(keys), "\x00") {
		if key == "" {
			continue
		}
		value := ""
		if strings.HasSuffix(key, ".required") {
			value = "false"
		}
		appendGitConfig(key, value)
	}
}

func appendGitConfig(key, value string) {
	count, err := strconv.Atoi(os.Getenv("GIT_CONFIG_COUNT"))
	if err != nil || count < 0 {
		count = 0
	}
	_ = os.Setenv("GIT_CONFIG_KEY_"+strconv.Itoa(count), key)
	_ = os.Setenv("GIT_CONFIG_VALUE_"+strconv.Itoa(count), value)
	_ = os.Setenv("GIT_CONFIG_COUNT", strconv.Itoa(count+1))
}

// ValidateArgs rejects outbound operations before dispatch can spawn a forge CLI.
//
//nolint:gocyclo // The command allowlist and listener guards are explicit at the CLI boundary.
func ValidateArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if args[0] == "preferences" {
		return nil
	} // This namespace parses host names, not listener addresses.
	if args[0] == "story" {
		local := false
		for _, arg := range args[1:] {
			flag, _, _ := strings.Cut(arg, "=")
			if flag == "--prep" || flag == "--skip-llm" || flag == "--story-file" || flag == "--clear" || flag == "--no-spend" || flag == "--help" || flag == "-h" {
				local = true
			}
		}
		if !local {
			return fmt.Errorf("local-only build: story requires --prep, --story-file, --skip-llm, --clear, or --no-spend")
		}
	}
	if args[0] == "install" && len(args) > 1 {
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			switch arg {
			case "codex", "codex-plugin", "claude", "claude-code", "all", "preferences", "prompts", "story-prompts":
			default:
				return fmt.Errorf("local-only build: install supports Codex and Claude file-based integrations")
			}
		}
	}
	switch args[0] {
	case "live", "preview":
		return fmt.Errorf("local-only Hunkyard reviews source diffs; executable page previews are disabled")
	case "pr", "pull", "push", "update":
		return fmt.Errorf("local-only build: %s is unavailable; use a local --range comparison", args[0])
	}
	for i, arg := range args {
		flag, value, hasValue := strings.Cut(arg, "=")
		switch flag {
		case "--pr", "--remote", "--public-url", "--live-origin", "--preview-file":
			return fmt.Errorf("local-only build: %s is unavailable", flag)
		case "--vcs":
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
			}
			if value != "git" {
				return fmt.Errorf("local-only build supports the Git backend; use --vcs git in colocated repositories")
			}
		case "--host":
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
			}
			if !loopback(value) && value != "0.0.0.0" && value != "::" {
				return fmt.Errorf("local-only build: listener must use a loopback or wildcard address")
			}
		}
	}
	for i, arg := range args {
		if i == 0 || (i == 1 && args[0] == "live") {
			if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
				if err := CheckURL(arg); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
