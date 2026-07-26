package project

import "regexp"

// prosePattern is one known credential shape scrubbed from free text before it
// leaves a read model.
type prosePattern struct {
	reason string
	regex  *regexp.Regexp
}

// proseRedactionPatterns mirrors the execution-ledger prose redaction rules in
// apps/runtime-agent/src/redaction.rs (the 离机前 redaction outlet from the
// evidence-grounding round). Keep the two sets in sync when adding shapes:
// runtime redacts ledger excerpts before they leave the host, while this Go
// mirror covers control-plane read projections of columns that were written
// without that pass (e.g. task_runs.error_message from terminal writeback).
var proseRedactionPatterns = []prosePattern{
	{reason: "jwt", regex: regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
	{reason: "bearer", regex: regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/-]{16,}=*`)},
	{reason: "anthropic_key", regex: regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`)},
	{reason: "github_token", regex: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{reason: "aws_access_key", regex: regexp.MustCompile(`(?:AKIA|ASIA)[A-Z0-9]{16}`)},
}

// redactProse replaces known credential shapes with `[REDACTED:{reason}]`,
// matching the runtime ledger redaction's placeholder format. Ordinary text is
// returned unchanged.
func redactProse(value string) string {
	if value == "" {
		return value
	}
	result := value
	for _, pattern := range proseRedactionPatterns {
		result = pattern.regex.ReplaceAllString(result, "[REDACTED:"+pattern.reason+"]")
	}
	return result
}
