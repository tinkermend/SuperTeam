package project

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	projectAcceptanceTitleMaxRunes   = 80
	projectAcceptanceSummaryMaxRunes = 400
	// minCompletedClauseRunes is the smallest completed-demand clause worth
	// keeping; below it the summary degrades to plain truncation.
	minCompletedClauseRunes = 12
)

// ProjectAcceptanceDemandInput is one terminal demand used to build the
// human-facing project_acceptance presentation. TaskTitles are optional
// best-effort labels for tasks under that demand.
type ProjectAcceptanceDemandInput struct {
	ID         uuid.UUID
	Title      string
	Status     string
	UpdatedAt  time.Time
	TaskTitles []string
}

// ProjectAcceptancePresentation is the project-first copy and context for a
// project_acceptance / closure_confirm decision. Technical decision_type stays
// project_acceptance; human identity must lead with the project name (§4.3 /
// §5.3), with demand/task details relegated to summary / why / evidence.
type ProjectAcceptancePresentation struct {
	Title           string
	Summary         string
	Context         map[string]any
	PrimaryDemandID uuid.UUID
}

// demandTerminalStatusLabel maps a terminal demand status to its Chinese label
// (see apps/web/src/lib/status-labels.ts). Unknown statuses fall back to the raw
// value so a new server-side status never renders as an empty string.
func demandTerminalStatusLabel(status string) string {
	switch strings.TrimSpace(status) {
	case string(ProjectDemandStatusCompleted):
		return "已完成"
	case string(ProjectDemandStatusFailed):
		return "失败"
	case string(ProjectDemandStatusCancelled):
		return "已取消"
	default:
		return strings.TrimSpace(status)
	}
}

func demandCompleted(d ProjectAcceptanceDemandInput) bool {
	return strings.TrimSpace(d.Status) == string(ProjectDemandStatusCompleted)
}

// BuildProjectAcceptancePresentation builds inbox/approval title, summary and
// structured context from the project and its terminal demands. Title identity
// is always the project name (结项确认 · {项目}); never the opaque legacy
// "验收项目交付" and never a demand title borrowed as the card identity (F3/F4).
//
// Demands carries every terminal demand (completed / failed / cancelled) so the
// closure card shows the full picture, but the summary sentence only asserts
// 已完成 over the completed ones — a cancelled or failed demand must never be
// narrated as finished. Non-completed demands are stated separately.
func BuildProjectAcceptancePresentation(projectName string, projectID uuid.UUID, demands []ProjectAcceptanceDemandInput) ProjectAcceptancePresentation {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		projectName = "未命名项目"
	}

	sorted := append([]ProjectAcceptanceDemandInput(nil), demands...)
	// Primary = most recently updated terminal demand (usually the one that
	// just closed the project gate). Stable tie-break by title then id.
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if demandNewer(sorted[j], sorted[i]) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	out := ProjectAcceptancePresentation{
		Title: truncateRunes("结项确认 · "+projectName, projectAcceptanceTitleMaxRunes),
		Context: map[string]any{
			"decision_type": "project_acceptance",
			"project_id":    projectID.String(),
			"project_name":  projectName,
		},
	}

	demandEntries := make([]map[string]any, 0, len(sorted))
	for _, d := range sorted {
		entry := map[string]any{
			"id":           d.ID.String(),
			"title":        strings.TrimSpace(d.Title),
			"status":       d.Status,
			"status_label": demandTerminalStatusLabel(d.Status),
		}
		if len(d.TaskTitles) > 0 {
			titles := make([]string, 0, len(d.TaskTitles))
			for _, t := range d.TaskTitles {
				t = strings.TrimSpace(t)
				if t != "" {
					titles = append(titles, t)
				}
			}
			if len(titles) > 0 {
				entry["task_titles"] = titles
			}
		}
		demandEntries = append(demandEntries, entry)
	}
	out.Context["demands"] = demandEntries

	if len(sorted) == 0 {
		// No terminal demand to cite: state the gate, not a completion claim.
		out.Summary = truncateRunes(fmt.Sprintf("项目「%s」已达结项条件，请确认结项并归档", projectName), projectAcceptanceSummaryMaxRunes)
		return out
	}

	completed := make([]ProjectAcceptanceDemandInput, 0, len(sorted))
	unfinished := make([]ProjectAcceptanceDemandInput, 0, len(sorted))
	for _, d := range sorted {
		if demandCompleted(d) {
			completed = append(completed, d)
			continue
		}
		unfinished = append(unfinished, d)
	}

	// Primary identifies the demand that closed the project gate, so it must be
	// a completed one; only fall back to the newest terminal demand when the
	// project reaches closure without any completed demand.
	primary := sorted[0]
	if len(completed) > 0 {
		primary = completed[0]
	}
	out.PrimaryDemandID = primary.ID
	out.Context["primary_demand_id"] = primary.ID.String()

	prefix := "项目「" + projectName + "」"
	const tail = "，请确认结项并归档"

	var completedPart strings.Builder
	if len(completed) > 0 {
		completedPart.WriteString("· 需求")
		for i, d := range completed {
			if i == 0 {
				completedPart.WriteString("「")
			} else {
				completedPart.WriteString("；「")
			}
			completedPart.WriteString(demandLabel(d))
			completedPart.WriteString("」")
			if bits := taskTitleBits(d); len(bits) > 0 {
				completedPart.WriteString("（含任务：")
				completedPart.WriteString(strings.Join(bits, "、"))
				completedPart.WriteString("）")
			}
		}
		completedPart.WriteString("已完成")
	} else {
		completedPart.WriteString("无已完成需求")
	}

	var unfinishedPart strings.Builder
	if len(unfinished) > 0 {
		unfinishedPart.WriteString("；需求")
		for i, d := range unfinished {
			if i > 0 {
				unfinishedPart.WriteString("、")
			}
			unfinishedPart.WriteString("「")
			unfinishedPart.WriteString(demandLabel(d))
			unfinishedPart.WriteString("」")
			unfinishedPart.WriteString(demandTerminalStatusLabel(d.Status))
		}
		unfinishedPart.WriteString("，未计入本次结项")
	}

	out.Summary = composeAcceptanceSummary(prefix, completedPart.String(), unfinishedPart.String(), tail)
	return out
}

// composeAcceptanceSummary joins the summary parts under the rune cap, spending
// the budget on the completed enumeration only. The 未完成 clause is the one
// statement that must never be lost to truncation — dropping it silently turns
// the summary back into "everything completed", the exact defect this copy
// exists to prevent. It is kept whole and the completed list is clipped instead.
func composeAcceptanceSummary(prefix, completedPart, unfinishedPart, tail string) string {
	full := prefix + completedPart + unfinishedPart + tail
	if utf8.RuneCountInString(full) <= projectAcceptanceSummaryMaxRunes {
		return full
	}
	fixed := utf8.RuneCountInString(prefix) + utf8.RuneCountInString(unfinishedPart) + utf8.RuneCountInString(tail)
	budget := projectAcceptanceSummaryMaxRunes - fixed
	// Degenerate case (pathological project name / very many cancelled demands):
	// no room left to say anything about completions — clip the whole string.
	if budget < minCompletedClauseRunes {
		return truncateRunes(full, projectAcceptanceSummaryMaxRunes)
	}
	return prefix + truncateRunes(completedPart, budget) + unfinishedPart + tail
}

func demandLabel(d ProjectAcceptanceDemandInput) string {
	title := strings.TrimSpace(d.Title)
	if title == "" {
		return d.ID.String()
	}
	return title
}

func taskTitleBits(d ProjectAcceptanceDemandInput) []string {
	bits := make([]string, 0, len(d.TaskTitles))
	for _, t := range d.TaskTitles {
		t = strings.TrimSpace(t)
		if t != "" {
			bits = append(bits, t)
		}
	}
	return bits
}

func demandNewer(a, b ProjectAcceptanceDemandInput) bool {
	if !a.UpdatedAt.Equal(b.UpdatedAt) {
		return a.UpdatedAt.After(b.UpdatedAt)
	}
	at := strings.TrimSpace(a.Title)
	bt := strings.TrimSpace(b.Title)
	if at != bt {
		return at < bt
	}
	return a.ID.String() < b.ID.String()
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
