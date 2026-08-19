package runner

import (
	"fmt"
	"regexp"
	"strings"
)

// Redact secrets that may appear in Docker logs, inspect env, or error strings.
var secretRedact = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ghp_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)gho_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)ghs_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)github_pat_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)Bearer\s+\S+`),
	regexp.MustCompile(`(?i)RUNNER_TOKEN=[^\s]+`),
	regexp.MustCompile(`(?i)ACCESS_TOKEN=[^\s]+`),
}

// RedactSecrets replaces GitHub tokens and runner registration secrets in s.
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	out := s
	for _, re := range secretRedact {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			lower := strings.ToLower(m)
			switch {
			case strings.HasPrefix(lower, "bearer"):
				return "Bearer ***"
			case strings.HasPrefix(lower, "runner_token="):
				return "RUNNER_TOKEN=***"
			case strings.HasPrefix(lower, "access_token="):
				return "ACCESS_TOKEN=***"
			case strings.HasPrefix(lower, "github_pat_"):
				return "github_pat_***"
			case strings.HasPrefix(lower, "ghp_"):
				return "ghp_***"
			case strings.HasPrefix(lower, "gho_"):
				return "gho_***"
			case strings.HasPrefix(lower, "ghs_"):
				return "ghs_***"
			default:
				return "***"
			}
		})
	}
	return out
}

func sanitizeErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", RedactSecrets(err.Error()))
}
