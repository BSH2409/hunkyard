package preferences

import (
	"encoding/json"
	"github.com/BSH2409/hunkyard/internal/strictjson"
	"strings"
	"testing"
)

func TestCanonicalJSONReferenceParity(t *testing.T) {
	var fixtures []struct {
		Input json.RawMessage
		Bytes string
	}
	readContract(t, "canonical.json", &fixtures)
	for _, c := range fixtures {
		var input any
		d := json.NewDecoder(strings.NewReader(string(c.Input)))
		d.UseNumber()
		if err := d.Decode(&input); err != nil {
			t.Fatal(err)
		}
		got, err := canonicalJSON(input)
		if err != nil || string(got) != c.Bytes {
			t.Fatalf("canonical bytes differ: %q, %v", got, err)
		}
	}
	if _, err := canonicalJSON(map[string]any{"invalid": string([]byte{0xff})}); err == nil {
		t.Fatal("invalid UTF-8 accepted in canonical data")
	}
}

func TestStrictRecordsJSON(t *testing.T) {
	for _, input := range []string{`{"status":true,"status":false}`, `{"nested":{"x":1,"x":2}}`, `{} {}`, `{"values":[{"x":1,"x":2}]}`} {
		var out any
		if err := strictjson.Decode([]byte(input), &out); err == nil {
			t.Errorf("accepted ambiguous JSON %s", input)
		}
	}
	var out struct {
		Flag  bool `json:"flag"`
		Count int  `json:"count"`
	}
	if err := strictjson.Decode([]byte(`{"flag":false,"count":0}`), &out); err != nil {
		t.Fatal(err)
	}
	if err := strictjson.Decode([]byte(`{"extra":0}`), &out); err == nil {
		t.Fatal("unknown record fields accepted")
	}
}

func TestStrictRecordsYAML(t *testing.T) {
	for _, input := range []string{"x: 1\nx: 2\n", "x: &a [1]\ny: *a\n", "x: !!ruby/object:Example {}\n", "x: 1\n---\nx: 2\n", "? [a, b]\n: invalid\n"} {
		var out any
		if err := decodeYAML([]byte(input), &out); err == nil {
			t.Fatal("ambiguous or unsafe YAML accepted")
		}
	}
	var out map[string]any
	if err := decodeYAML([]byte("flag: false\ncount: 0\nnullable: null\n"), &out); err != nil {
		t.Fatal(err)
	}
	if out["flag"] != false || out["count"] != 0 || out["nullable"] != nil {
		t.Fatal("required false/zero/null values changed")
	}
}
