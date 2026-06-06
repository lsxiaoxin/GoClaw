package memory

import (
	"regexp"
	"strings"
)

var sensitivePatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{name: "API key", re: regexp.MustCompile(`(?i)\b(api[_ -]?key|access[_ -]?key)\b`)},
	{name: "token", re: regexp.MustCompile(`(?i)\b(token|bearer)\b`)},
	{name: "password", re: regexp.MustCompile(`(?i)\b(pass(word)?|pwd)\b`)},
	{name: "secret", re: regexp.MustCompile(`(?i)\bsecret\b`)},
	{name: "OpenAI-style key", re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`)},
	{name: "Chinese ID number", re: regexp.MustCompile(`\b\d{17}[\dXx]\b`)},
	{name: "street address", re: regexp.MustCompile(`(?i)\b\d{1,6}\s+[A-Za-z0-9 .'-]+(?:street|st\.|road|rd\.|avenue|ave\.|lane|ln\.|drive|dr\.)\b`)},
}

// Sensitivity describes suspicious private or credential-like content.
type Sensitivity struct {
	Sensitive bool
	Reasons   []string
}

// DetectSensitive returns a conservative signal for content that should not be
// written to long-lived memory without explicit review.
func DetectSensitive(content string) Sensitivity {
	content = strings.TrimSpace(content)
	if content == "" {
		return Sensitivity{}
	}
	seen := make(map[string]struct{})
	var reasons []string
	for _, pattern := range sensitivePatterns {
		if !pattern.re.MatchString(content) {
			continue
		}
		if _, ok := seen[pattern.name]; ok {
			continue
		}
		seen[pattern.name] = struct{}{}
		reasons = append(reasons, pattern.name)
	}
	return Sensitivity{Sensitive: len(reasons) > 0, Reasons: reasons}
}
