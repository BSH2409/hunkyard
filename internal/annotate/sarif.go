package annotate

import (
	"encoding/json"
	"fmt"
	"io"
)

type sarifLog struct {
	Runs []struct {
		Tool struct {
			Driver struct {
				Name string `json:"name"`
			} `json:"driver"`
		} `json:"tool"`
		Results []struct {
			RuleID  string `json:"ruleId"`
			Level   string `json:"level"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region *struct {
						StartLine int `json:"startLine"`
						EndLine   int `json:"endLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

// parseSarif reads a SARIF 2.1 log. A result without a region is a
// file-level finding; a result without a location is ignored.
func parseSarif(r io.Reader, opts Options) (Annotations, error) {
	var log sarifLog
	if err := json.NewDecoder(io.LimitReader(r, 64<<20)).Decode(&log); err != nil {
		return Annotations{}, fmt.Errorf("invalid SARIF: %w", err)
	}
	out := Annotations{Tool: "sarif", Findings: []Finding{}}
	for i, run := range log.Runs {
		if i == 0 && run.Tool.Driver.Name != "" {
			out.Tool = run.Tool.Driver.Name
		}
		for _, result := range run.Results {
			if len(result.Locations) == 0 || result.Locations[0].PhysicalLocation.ArtifactLocation.URI == "" {
				out.IgnoredFiles++
				continue
			}
			loc := result.Locations[0].PhysicalLocation
			f := Finding{Path: relativePath(loc.ArtifactLocation.URI, opts), Message: result.Message.Text, Rule: result.RuleID, Level: result.Level}
			if loc.Region != nil && loc.Region.StartLine > 0 {
				f.Line = loc.Region.StartLine
				f.EndLine = loc.Region.EndLine
				if f.EndLine < f.Line {
					f.EndLine = f.Line
				}
			}
			out.Findings = append(out.Findings, f)
		}
	}
	return out, nil
}
