package preferences

import (
	"regexp"
	"strings"
	"unicode"
)

type privacyResult struct {
	Accepted                     bool
	Excerpt, Fingerprint, Reason string
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\bpassword\s*=`),
	regexp.MustCompile(`(?i)\bauthorization\s*:`),
	regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s/@:]+:[^\s@/]+@`),
	regexp.MustCompile(`(?i)\b(?:sk|pk|rk|ghp|github_pat|xox[abprs])-?[a-z0-9_-]{12,}\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|auth[_-]?token|secret|token|credential)\s*[:=]\s*["']?[A-Za-z0-9+/_=-]{20,}`),
}
var environmentAssignment = regexp.MustCompile(`(?:^|\s)(?:export\s+)?[A-Z][A-Z0-9_]{2,}\s*=`)
var codeLine = regexp.MustCompile(`^\s*(?:def |class |module |function |const |let |var |if |else |return\b|puts\b|print\b|require\b|include\b|describe\b|it\b|[A-Za-z_][A-Za-z0-9_]*\s*[=({])`)
var repositoryURL = regexp.MustCompile(`(?i)\b[a-z0-9._-]+@[a-z0-9.-]+:[a-z0-9._~/-]+\b`)
var ordinaryURL = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://\S+`)
var emailAddress = regexp.MustCompile("(?i)\\b[a-z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+\\b")

func validUTF8(text string) string {
	// Range replaces each invalid byte, matching the reference's UTF-8 conversion.
	var out strings.Builder
	for _, r := range text {
		out.WriteRune(r)
	}
	return out.String()
}

func normalize(text string) string { return strings.Join(strings.Fields(validUTF8(text)), " ") }

func sanitize(text string) privacyResult {
	text = validUTF8(text)
	normalized := normalize(text)
	result := privacyResult{Fingerprint: digest(normalized)}
	sensitive := len(environmentAssignment.FindAllStringIndex(text, -1)) >= 3
	for _, pattern := range sensitivePatterns {
		sensitive = sensitive || pattern.MatchString(text)
	}
	codeLines := 0
	for _, line := range strings.Split(text, "\n") {
		if codeLine.MatchString(line) {
			codeLines++
		}
	}
	if sensitive || codeLines >= 20 {
		result.Reason = "sensitive_content"
		return result
	}
	excerpt := repositoryURL.ReplaceAllString(normalized, "[REPOSITORY_URL]")
	excerpt = ordinaryURL.ReplaceAllString(excerpt, "[URL]")
	excerpt = emailAddress.ReplaceAllString(excerpt, "[EMAIL]")
	result.Accepted = true
	result.Excerpt = strings.TrimRightFunc(truncate(excerpt, 1000), unicode.IsSpace)
	return result
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return text
}
