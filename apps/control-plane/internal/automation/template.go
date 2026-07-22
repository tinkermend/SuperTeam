package automation

import (
	"fmt"
	"strings"
	"time"
)

// RenderTemplate replaces P1 template variables in Asia/Shanghai (or rule timezone).
func RenderTemplate(tmpl string, at time.Time, timezone, ruleName, projectName string) (string, error) {
	loc, err := loadTimezone(timezone)
	if err != nil {
		return "", err
	}
	local := at.In(loc)
	date := local.Format("2006-01-02")
	datetime := local.Format("2006-01-02 15:04:05")
	out := tmpl
	out = strings.ReplaceAll(out, "{{datetime}}", datetime)
	out = strings.ReplaceAll(out, "{{date}}", date)
	out = strings.ReplaceAll(out, "{{rule_name}}", ruleName)
	out = strings.ReplaceAll(out, "{{project_name}}", projectName)
	return out, nil
}

func loadTimezone(timezone string) (*time.Location, error) {
	tz := strings.TrimSpace(timezone)
	if tz == "" {
		tz = DefaultTimezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid timezone %q: %v", ErrInvalidInput, tz, err)
	}
	return loc, nil
}
