package clicmd

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestPlural(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "s"},
		{1, ""},
		{2, "s"},
		{100, "s"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d", tt.n), func(t *testing.T) {
			if got := Plural(tt.n); got != tt.want {
				t.Errorf("Plural(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestExitError_ErrorAndUnwrap(t *testing.T) {
	inner := errors.New("boom")
	err := ExitError{Code: 2, Err: inner}
	if err.Error() != "boom" {
		t.Errorf("Error() = %q, want boom", err.Error())
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap should expose inner error")
	}
	if (ExitError{Code: 1}).Error() != "exit" {
		t.Error("nil Err should return 'exit'")
	}
}

func TestUsage(t *testing.T) {
	err := Usage("bad flags")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("Usage should return ExitError code 1, got %T %v", err, err)
	}
	if exitErr.Error() != "bad flags" {
		t.Errorf("got %q", exitErr.Error())
	}
}

func TestRequireFlagValue(t *testing.T) {
	got, err := RequireFlagValue([]string{"--output", "/tmp"}, 0, "--output")
	if err != nil || got != "/tmp" {
		t.Fatalf("got %q, %v", got, err)
	}
	_, err = RequireFlagValue([]string{"--output"}, 0, "--output")
	if err == nil {
		t.Fatal("expected error when value missing")
	}
	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
}

func TestMustGetwd(t *testing.T) {
	wd, err := MustGetwd()
	if err != nil {
		t.Fatal(err)
	}
	if wd == "" {
		t.Fatal("expected non-empty cwd")
	}
	if wd != os.Getenv("PWD") && wd != mustGetwdFallback() {
		// PWD may be unset in some shells; resolved path should still exist.
		if _, statErr := os.Stat(wd); statErr != nil {
			t.Fatalf("cwd %q not stat-able: %v", wd, statErr)
		}
	}
}

func mustGetwdFallback() string {
	wd, _ := os.Getwd()
	return wd
}

func TestReorderFlagsFirst(t *testing.T) {
	boolFlags := map[string]bool{
		"no-open":                       true,
		"allow-unauthenticated-network": true,
		"quiet":                         true,
		"q":                             true,
	}
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "flag after file arg (hunkyard#787 repro)",
			args: []string{"design.md", "-p", "51573", "--public-url", "https://x.ts.net", "--no-open"},
			want: []string{"-p", "51573", "--public-url", "https://x.ts.net", "--no-open", "design.md"},
		},
		{
			name: "bool flag after file arg",
			args: []string{"design.md", "--no-open"},
			want: []string{"--no-open", "design.md"},
		},
		{
			name: "already flags-first is unchanged in effect",
			args: []string{"-p", "51573", "design.md"},
			want: []string{"-p", "51573", "design.md"},
		},
		{
			name: "multiple positional args stay in order",
			args: []string{"a.md", "--no-open", "b.md", "-p", "51573"},
			want: []string{"--no-open", "-p", "51573", "a.md", "b.md"},
		},
		{
			name: "equals-form flag needs no lookahead",
			args: []string{"design.md", "--public-url=https://x.ts.net"},
			want: []string{"--public-url=https://x.ts.net", "design.md"},
		},
		{
			name: "double-dash terminates reordering and is preserved",
			args: []string{"design.md", "--", "-not-a-flag"},
			want: []string{"--", "design.md", "-not-a-flag"},
		},
		{
			name: "flag before terminator keeps dash-looking positional",
			args: []string{"--no-open", "--", "-weird.md"},
			want: []string{"--no-open", "--", "-weird.md"},
		},
		{
			name: "leading terminator keeps following flags positional",
			args: []string{"--", "--no-open", "design.md"},
			want: []string{"--", "--no-open", "design.md"},
		},
		{
			name: "bare dash is positional, not a flag",
			args: []string{"-", "--no-open"},
			want: []string{"--no-open", "-"},
		},
		{
			name: "help flag treated as valueless even though unregistered",
			args: []string{"design.md", "-h"},
			want: []string{"-h", "design.md"},
		},
		{
			name: "empty args",
			args: []string{},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReorderFlagsFirst(tt.args, boolFlags)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReorderFlagsFirst(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestReorderFlagsFirst_RoundTripFlagParse(t *testing.T) {
	boolFlags := map[string]bool{"no-open": true}
	tests := []struct {
		name       string
		args       []string
		wantNoOpen bool
		wantArgs   []string
	}{
		{
			name:       "flag after file",
			args:       []string{"design.md", "--no-open"},
			wantNoOpen: true,
			wantArgs:   []string{"design.md"},
		},
		{
			name:       "dash-looking file after terminator",
			args:       []string{"--no-open", "--", "-weird.md"},
			wantNoOpen: true,
			wantArgs:   []string{"-weird.md"},
		},
		{
			name:       "leading terminator keeps flag literal",
			args:       []string{"--", "--no-open", "design.md"},
			wantNoOpen: false,
			wantArgs:   []string{"--no-open", "design.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet(tt.name, flag.ContinueOnError)
			noOpen := fs.Bool("no-open", false, "")
			if err := fs.Parse(ReorderFlagsFirst(tt.args, boolFlags)); err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if *noOpen != tt.wantNoOpen {
				t.Errorf("no-open = %v, want %v", *noOpen, tt.wantNoOpen)
			}
			if !reflect.DeepEqual(fs.Args(), tt.wantArgs) {
				t.Errorf("Args() = %v, want %v", fs.Args(), tt.wantArgs)
			}
		})
	}
}

// A command that already printed its error exits 1 without a second line.
func TestCLIErrorHasNoStrayExitLine(t *testing.T) {
	if os.Getenv("HUNKYARD_CLICMD_EXIT_TEST") == "1" {
		fmt.Fprintln(os.Stderr, "Error: invalid --days value")
		Exit(ExitError{Code: 1, Err: ErrReported})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLIErrorHasNoStrayExitLine$", "-test.count=1")
	cmd.Env = append(os.Environ(), "HUNKYARD_CLICMD_EXIT_TEST=1")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit = %v, want code 1", err)
	}
	if got := stderr.String(); got != "Error: invalid --days value\n" {
		t.Errorf("stderr = %q, want only the reported error", got)
	}
}
