package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const schemaVersion = 1

var surfaces = strings.Fields("api architecture communication database devops frontend jobs naming performance rails reviews ruby security services testing wordpress")

type NeighboringContext struct {
	Source              string `json:"source,omitempty"`
	PreviousUserRequest string `json:"previous_user_request,omitempty"`
	AgentResponse       string `json:"agent_response,omitempty"`
}

type observation struct {
	ID                string              `json:"id"`
	CapturedAt        string              `json:"captured_at"`
	Source            string              `json:"source"`
	RepoKey           *string             `json:"repo_key"`
	SessionKey        *string             `json:"session_key"`
	TurnKey           *string             `json:"turn_key"`
	PromptFingerprint string              `json:"prompt_fingerprint"`
	SafeExcerpt       *string             `json:"safe_excerpt"`
	Signals           observationSignals  `json:"signals"`
	SuggestedSurfaces []string            `json:"suggested_surfaces"`
	Context           *NeighboringContext `json:"context,omitempty"`
}

type observationSignals struct {
	Correction      bool `json:"correction"`
	DurableLanguage bool `json:"durable_language"`
	LikelyTaskLocal bool `json:"likely_task_local"`
}

// Rules remain plain schema records inside the package; only validated catalogs can publish them.
type rule map[string]any

func decodeYAML(data []byte, out any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("invalid UTF-8 in YAML")
	}
	var node yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&node); err != nil {
		return err
	}
	var extra yaml.Node
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("YAML must contain one document")
	}
	if err := checkYAMLNode(&node, 0); err != nil {
		return err
	}
	// KnownFields is enforced on the second decode, after inspecting all nodes.
	d = yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	return d.Decode(out)
}

func checkYAMLNode(node *yaml.Node, depth int) error {
	if depth > 128 {
		return fmt.Errorf("YAML nesting limit exceeded")
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("YAML aliases are unavailable")
	}
	switch node.Tag {
	case "", "!!str", "!!int", "!!bool", "!!null", "!!float", "!!map", "!!seq":
	default:
		return fmt.Errorf("unsupported YAML tag")
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return fmt.Errorf("duplicate or invalid YAML key")
			}
			seen[key.Value] = true
		}
	}
	for _, child := range node.Content {
		if err := checkYAMLNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	if err := validateUTF8(reflect.ValueOf(value), 0); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	d := json.NewDecoder(bytes.NewReader(encoded))
	d.UseNumber()
	if err := d.Decode(&normalized); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	writeCanonical(&out, normalized)
	return out.Bytes(), nil
}

//nolint:gocyclo // The type switch explicitly covers the canonical JSON and UTF-8 contract.
func validateUTF8(v reflect.Value, depth int) error {
	if depth > 128 {
		return fmt.Errorf("canonical data nesting limit exceeded")
	}
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return fmt.Errorf("invalid UTF-8 in canonical data")
		}
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			return validateUTF8(v.Elem(), depth+1)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := validateUTF8(v.Index(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			for _, item := range []reflect.Value{it.Key(), it.Value()} {
				if err := validateUTF8(item, depth+1); err != nil {
					return err
				}
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				if err := validateUTF8(v.Field(i), depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

//nolint:gocyclo // The type switch explicitly covers the canonical JSON and UTF-8 contract.
func writeCanonical(out *bytes.Buffer, value any) {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(v))
	case json.Number:
		out.WriteString(string(v))
	case string:
		out.WriteByte('"')
		for _, r := range v {
			switch r {
			case '"', '\\':
				out.WriteByte('\\')
				out.WriteRune(r)
			case '\b':
				out.WriteString(`\b`)
			case '\f':
				out.WriteString(`\f`)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			case '\t':
				out.WriteString(`\t`)
			default:
				if r < 32 {
					fmt.Fprintf(out, `\u%04x`, r)
				} else {
					out.WriteRune(r)
				}
			}
		}
		out.WriteByte('"')
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			writeCanonical(out, item)
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			writeCanonical(out, key)
			out.WriteByte(':')
			writeCanonical(out, v[key])
		}
		out.WriteByte('}')
	}
}

func exactFields(value map[string]any, required, optional []string) error {
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
		if _, ok := value[key]; !ok {
			return fmt.Errorf("missing required field %s", key)
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range value {
		if !allowed[key] {
			return fmt.Errorf("unknown record field %s", key)
		}
	}
	return nil
}
