package project

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	projectAcceptanceTitleMaxRunes  = 80
	projectAcceptanceSummaryMaxRunes = 400
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

// BuildProjectAcceptancePresentation builds inbox/approval title, summary and
// structured context from the project and its terminal demands. Title identity
// is always the project name (结项确认 · {项目}); never the opaque legacy
// "验收项目交付" and never a demand title borrowed as the card identity (F3/F4).
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
			"id":     d.ID.String(),
			"title":  strings.TrimSpace(d.Title),
			"status": d.Status,
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
		out.Summary = truncateRunes(fmt.Sprintf("项目「%s」全部需求已完成，请确认结项并归档", projectName), projectAcceptanceSummaryMaxRunes)
		return out
	}

	primary := sorted[0]
	out.PrimaryDemandID = primary.ID
	out.Context["primary_demand_id"] = primary.ID.String()

	var b strings.Builder
	b.WriteString("项目「")
	b.WriteString(projectName)
	b.WriteString("」· 需求")
	for i, d := range sorted {
		title := strings.TrimSpace(d.Title)
		if title == "" {
			title = d.ID.String()
		}
		if i == 0 {
			b.WriteString("「")
			b.WriteString(title)
			b.WriteString("」")
		} else {
			b.WriteString("；「")
			b.WriteString(title)
			b.WriteString("」")
		}
		if len(d.TaskTitles) > 0 {
			taskBits := make([]string, 0, len(d.TaskTitles))
			for _, t := range d.TaskTitles {
				t = strings.TrimSpace(t)
				if t != "" {
					taskBits = append(taskBits, t)
				}
			}
			if len(taskBits) > 0 {
				b.WriteString("（含任务：")
				b.WriteString(strings.Join(taskBits, "、"))
				b.WriteString("）")
			}
		}
	}
	b.WriteString("已完成，请确认结项并归档")
	out.Summary = truncateRunes(b.String(), projectAcceptanceSummaryMaxRunes)
	return out
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
