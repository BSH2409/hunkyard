package preferences

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type contractCase struct {
	SourceFile        string `json:"source_file"`
	Test              string `json:"test"`
	Owner             string `json:"owner"`
	GoTest            string `json:"go_test"`
	Provenance        string `json:"provenance"`
	Status            string `json:"status"`
	ReplacementReason string `json:"replacement_reason,omitempty"`
}

func readContract(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile("testdata/contracts/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func TestContractInventory(t *testing.T) {
	var inventory struct {
		Tests []contractCase `json:"tests"`
	}
	readContract(t, "index.json", &inventory)
	if len(inventory.Tests) != 284 {
		t.Fatalf("source cases = %d, want 284", len(inventory.Tests))
	}
	seen := map[string]bool{}
	for _, c := range inventory.Tests {
		key := c.SourceFile + "/" + c.Test
		if seen[key] {
			t.Fatalf("duplicate case %s", key)
		}
		seen[key] = true
		if c.Owner == "" || !strings.HasPrefix(c.GoTest, "Test") || c.Provenance == "" || (c.Status != "replaced" && c.Status != "covered") || c.Status == "replaced" && c.ReplacementReason == "" {
			t.Fatalf("incomplete contract mapping: %s", key)
		}
	}
}

func TestCLIContractFixtures(t *testing.T) {
	var fixtures struct {
		Cases []struct {
			Args   []string `json:"args"`
			Stdout string   `json:"stdout"`
			Exit   int      `json:"exit"`
		} `json:"cases"`
	}
	readContract(t, "cli.json", &fixtures)
	want := map[string]bool{}
	for _, command := range strings.Fields("capture search build audit retire maintain migrate install-hook install-cron status") {
		want[command] = false
	}
	for _, c := range fixtures.Cases {
		if len(c.Args) == 0 {
			t.Fatal("missing argv")
		}
		if _, ok := want[c.Args[0]]; ok && c.Exit == 2 && json.Valid([]byte(c.Stdout)) {
			want[c.Args[0]] = true
		}
	}
	for command, found := range want {
		if !found {
			t.Errorf("missing observed usage fixture: %s", command)
		}
	}
}
