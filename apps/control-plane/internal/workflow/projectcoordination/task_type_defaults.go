package projectcoordination

// taskTypeCapabilityDefaults maps a canonical task kind to the capabilities the
// platform requires by default for that kind. The planner may add more; these are
// the floor that guarantees capability matching even when the model forgets to
// declare required capabilities for a well-known task type (spec §9).
var taskTypeCapabilityDefaults = map[string][]string{
	"database_analysis": {
		"database.read",
		"sql.analysis",
		"data.quality.check",
		"business.metric.interpretation",
	},
	"incident_triage": {
		"incident.triage",
		"log.analysis",
		"metrics.analysis",
		"code.path.tracing",
		"runtime.diagnostics",
	},
	"feature_development": {
		"codebase.analysis",
		"implementation",
		"testing.verification",
		"artifact.reporting",
	},
}

// taskKindAliases maps common task-kind variants emitted by the planner to their
// canonical key in taskTypeCapabilityDefaults. Keeping this separate from the
// defaults map lets us evolve aliases without duplicating capability lists.
var taskKindAliases = map[string]string{
	"db_analysis":            "database_analysis",
	"database_analytics":     "database_analysis",
	"data_analysis":          "database_analysis",
	"database_query":         "database_analysis",
	"incident_investigation": "incident_triage",
	"incident_diagnosis":     "incident_triage",
	"system_diagnostics":     "incident_triage",
	"system_incident":        "incident_triage",
	"incident_response":      "incident_triage",
	"feature_implementation": "feature_development",
	"feature_dev":            "feature_development",
	"software_development":   "feature_development",
	"code_implementation":    "feature_development",
}

const (
	WorkspaceModeNone        = "none"
	WorkspaceModeReadonly    = "readonly"
	WorkspaceModeDiff        = "diff"
	WorkspaceModeDetachedRun = "detached_run"
	WorkspaceModeBranch      = "branch"
)

var taskKindWorkspaceModes = map[string]string{
	"feature_development":    WorkspaceModeBranch,
	"feature_implementation": WorkspaceModeBranch,
	"feature_dev":            WorkspaceModeBranch,
	"software_development":   WorkspaceModeBranch,
	"code_implementation":    WorkspaceModeBranch,
	"implementation":         WorkspaceModeBranch,
	"bug_fix":                WorkspaceModeBranch,
	"code_change":            WorkspaceModeBranch,
	"code_review":            WorkspaceModeDiff,
	"review":                 WorkspaceModeDiff,
	"pull_request_review":    WorkspaceModeDiff,
	"diff_review":            WorkspaceModeDiff,
	"test_verification":      WorkspaceModeDetachedRun,
	"testing_verification":   WorkspaceModeDetachedRun,
	"build_verification":     WorkspaceModeDetachedRun,
	"verification":           WorkspaceModeDetachedRun,
	"qa":                     WorkspaceModeDetachedRun,
	"database_analysis":      WorkspaceModeReadonly,
	"db_analysis":            WorkspaceModeReadonly,
	"database_analytics":     WorkspaceModeReadonly,
	"data_analysis":          WorkspaceModeReadonly,
	"database_query":         WorkspaceModeReadonly,
	"incident_triage":        WorkspaceModeReadonly,
	"incident_investigation": WorkspaceModeReadonly,
	"incident_diagnosis":     WorkspaceModeReadonly,
	"system_diagnostics":     WorkspaceModeReadonly,
	"system_incident":        WorkspaceModeReadonly,
	"incident_response":      WorkspaceModeReadonly,
	"analysis":               WorkspaceModeReadonly,
	"status_report":          WorkspaceModeNone,
	"human_approval":         WorkspaceModeNone,
	"acceptance_summary":     WorkspaceModeNone,
}

// canonicalTaskKind normalizes a planner-emitted task kind into the canonical key
// used by taskTypeCapabilityDefaults. Returns "" when the kind is unrecognized.
func canonicalTaskKind(kind string) string {
	normalized := normalizePlanningString(kind)
	if normalized == "" {
		return ""
	}
	if canonical, ok := taskKindAliases[normalized]; ok {
		return canonical
	}
	if _, ok := taskTypeCapabilityDefaults[normalized]; ok {
		return normalized
	}
	return ""
}

func WorkspaceModeForTaskKind(kind string) string {
	normalized := normalizePlanningString(kind)
	if normalized == "" {
		return WorkspaceModeNone
	}
	if canonical := canonicalTaskKind(normalized); canonical != "" {
		normalized = canonical
	}
	if mode, ok := taskKindWorkspaceModes[normalized]; ok {
		return mode
	}
	return WorkspaceModeNone
}

// DefaultRequiredCapabilities returns the platform-default required capabilities
// for the given task kind. The result is a copy; callers may mutate it freely.
// Returns nil for unknown kinds so that arbitrary/custom task kinds are not
// forced through capability matching with stale defaults.
func DefaultRequiredCapabilities(taskKind string) []string {
	canonical := canonicalTaskKind(taskKind)
	if canonical == "" {
		return nil
	}
	defaults := taskTypeCapabilityDefaults[canonical]
	out := make([]string, 0, len(defaults))
	for _, value := range defaults {
		if normalized := normalizePlanningString(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

// ApplyTaskTypeDefaults merges platform-default required capabilities into each
// planner task based on its task kind. The merge is a union: capabilities the
// model already declared are preserved, and only missing defaults are appended.
// This runs after the planner decodes its output and before scoring/validation
// so that defaults flow through capability matching, hard-failure recording, and
// the selection evidence persisted to route-decision/task metadata.
//
// Tasks whose task kind is empty or unknown are left untouched, preserving the
// planner's authority over custom task types. When defaults are applied, the
// affected task keys are recorded under planner_metadata.task_type_defaults_applied
// for auditability.
func ApplyTaskTypeDefaults(plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	applied := make([]string, 0)
	for index := range plan.Tasks {
		task := &plan.Tasks[index]
		defaults := DefaultRequiredCapabilities(task.TaskKind)
		if len(defaults) == 0 {
			continue
		}
		before := len(task.RequiredCapabilities)
		for _, value := range defaults {
			task.RequiredCapabilities = appendUniqueString(task.RequiredCapabilities, value)
		}
		if len(task.RequiredCapabilities) > before {
			applied = append(applied, task.Key)
		}
	}
	if len(applied) == 0 {
		return
	}
	if plan.PlannerMetadata == nil {
		plan.PlannerMetadata = map[string]any{}
	}
	plan.PlannerMetadata["task_type_defaults_applied"] = applied
}
