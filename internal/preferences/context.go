package preferences

import (
	"github.com/BSH2409/hunkyard/internal/strictjson"
	"regexp"
	"strings"
)

var fencedCode = regexp.MustCompile("(?s)```.*?```")
var contextPath = regexp.MustCompile(`(^|[^A-Za-z0-9_])/(?:[^\s/]+/)+[^\s,;]+`)
var contextIdentifier = regexp.MustCompile(`(?i)\b((?:site|account|customer|client|tenant|user)[_ -]?id)\s*[:=]?\s*\d+\b`)

func sanitizeContext(input NeighboringContext) *NeighboringContext {
	prose := func(text string, limit int) string {
		if len(text) > 256*1024 || !sanitize(text).Accepted {
			return ""
		}
		text = fencedCode.ReplaceAllString(text, " ")
		lines := []string{}
		for _, line := range strings.Split(text, "\n") {
			if !codeLine.MatchString(line) {
				lines = append(lines, line)
			}
		}
		text = normalize(strings.Join(lines, " "))
		text = repositoryURL.ReplaceAllString(text, "[REPOSITORY_URL]")
		text = ordinaryURL.ReplaceAllString(text, "[URL]")
		text = emailAddress.ReplaceAllString(text, "[EMAIL]")
		text = contextPath.ReplaceAllString(text, "${1}[PATH]")
		text = contextIdentifier.ReplaceAllString(text, "${1} [ID]")
		return truncate(text, limit)
	}
	out := &NeighboringContext{Source: input.Source, PreviousUserRequest: prose(input.PreviousUserRequest, 400), AgentResponse: prose(input.AgentResponse, 800)}
	if out.PreviousUserRequest == "" && out.AgentResponse == "" {
		return nil
	}
	if out.Source != "provided" && out.Source != "transcript" {
		out.Source = "provided"
	}
	return out
}

func validateContext(value any) error {
	data, err := canonicalJSON(value)
	if err != nil {
		return err
	}
	var context NeighboringContext
	if err := strictjson.Decode(data, &context); err != nil {
		return err
	}
	normalized := sanitizeContext(context)
	if normalized == nil || !canonicalEqual(value, normalized) {
		return invalidRecord("neighboring context must be bounded sanitized prose")
	}
	return nil
}
