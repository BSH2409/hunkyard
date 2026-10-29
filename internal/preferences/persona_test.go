package preferences

import (
	"fmt"
	"os"
	"testing"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

func TestPersonaReferenceParity(t *testing.T) {
	data, err := os.ReadFile("testdata/contracts/persona.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Reference string `json:"reference"`
		Events    []struct {
			Case, Kind string
			Rules      []rule
			Files      map[string]string
			Manifest   map[string]any
			Prompt     string
			RepoKey    string `json:"repo_key"`
			Result     Selection
		} `json:"events"`
	}
	if err := strictjson.Decode(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Events) == 0 {
		t.Fatal("no persona reference cases")
	}
	for i, event := range fixture.Events {
		t.Run(fmt.Sprintf("%s/%s/%d", event.Case, event.Kind, i), func(t *testing.T) {
			if event.Kind == "render" {
				files, manifest := renderPersona(event.Rules)
				if !sameValue(files, event.Files) || !sameValue(manifest, event.Manifest) {
					t.Fatal("rendered files or manifest differ from Ruby reference")
				}
			} else {
				actual := selectPersona(event.Rules, event.Prompt, event.RepoKey)
				if !sameValue(actual, event.Result) {
					t.Fatalf("selection differs\nactual %#v\nexpected %#v", actual, event.Result)
				}
			}
		})
	}
}
