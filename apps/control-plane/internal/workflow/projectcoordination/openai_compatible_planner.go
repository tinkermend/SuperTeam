package projectcoordination

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrPlannerUnavailable    = errors.New("route planner unavailable")
	ErrPlannerRequestTimeout = errors.New("route planner request timeout")
)

const maxChatCompletionResponseBytes = 1 << 20

// defaultPlannerRequestTimeout is generous because the planner targets a reasoning
// model, whose chain-of-thought on a full project planning prompt routinely takes far
// longer than a non-reasoning completion before it emits the final JSON content.
const defaultPlannerRequestTimeout = 120 * time.Second

type OpenAICompatiblePlannerConfig struct {
	APIKey         string
	BaseURL        string
	Model          string
	MaxTokens      int
	Temperature    float64
	MaxAttempts    int
	RequestTimeout time.Duration
}

type OpenAICompatibleChatRequest struct {
	Model       string
	System      string
	User        string
	MaxTokens   int
	Temperature float64
}

type chatCompletionClient interface {
	CreateChatCompletion(ctx context.Context, req OpenAICompatibleChatRequest) (string, error)
}

type OpenAICompatibleRoutePlanner struct {
	cfg    OpenAICompatiblePlannerConfig
	client chatCompletionClient
}

func NewOpenAICompatibleRoutePlanner(cfg OpenAICompatiblePlannerConfig, clients ...chatCompletionClient) *OpenAICompatibleRoutePlanner {
	var client chatCompletionClient
	if len(clients) > 0 {
		client = clients[0]
	}
	if client == nil {
		client = newOpenAICompatibleHTTPChatCompletionClient(cfg.BaseURL, cfg.APIKey, plannerRequestTimeout(cfg.RequestTimeout))
	}
	return &OpenAICompatibleRoutePlanner{cfg: cfg, client: client}
}

func (p *OpenAICompatibleRoutePlanner) Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	if p == nil || strings.TrimSpace(p.cfg.APIKey) == "" || strings.TrimSpace(p.cfg.BaseURL) == "" || strings.TrimSpace(p.cfg.Model) == "" || p.client == nil {
		return RouteDecisionPlan{}, ErrPlannerUnavailable
	}
	if err := ctx.Err(); err != nil {
		return RouteDecisionPlan{}, err
	}
	attempts := p.cfg.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return RouteDecisionPlan{}, err
		}
		requestCtx, cancel := p.requestContext(ctx)
		content, err := p.client.CreateChatCompletion(requestCtx, OpenAICompatibleChatRequest{
			Model:       p.cfg.Model,
			System:      buildPlannerSystemPrompt(),
			User:        buildPlannerUserPrompt(snapshot),
			MaxTokens:   p.cfg.MaxTokens,
			Temperature: p.cfg.Temperature,
		})
		cancel()
		if err != nil {
			contextErr, terminal := plannerContextError(ctx, requestCtx, err)
			if terminal {
				return RouteDecisionPlan{}, contextErr
			}
			if contextErr != nil {
				lastErr = contextErr
				break
			}
			lastErr = err
			continue
		}
		if err := ctx.Err(); err != nil {
			return RouteDecisionPlan{}, err
		}
		plan, err := decodePlannerJSON(content)
		if err != nil {
			if contextErr := terminalContextError(ctx); contextErr != nil {
				return RouteDecisionPlan{}, contextErr
			}
			lastErr = err
			continue
		}
		pool := activeExecutorIDs(snapshot.DigitalEmployeePool)
		applyRequiredHumanReviewPolicy(snapshot, &plan)
		if err := ValidateRouteDecisionGraph(plan, pool, GraphValidationPolicy{MaxTasks: 12}); err != nil {
			if contextErr := terminalContextError(ctx); contextErr != nil {
				return RouteDecisionPlan{}, contextErr
			}
			if requiredHumanReviewPolicyEnabled(snapshot.CoordinationPolicy) {
				repaired := synthesizeRequiredReviewPlan(snapshot, pool, plan)
				if repairErr := ValidateRouteDecisionGraph(repaired, pool, GraphValidationPolicy{MaxTasks: 12}); repairErr == nil {
					return repaired, nil
				}
			}
			lastErr = err
			continue
		}
		return plan, nil
	}
	if lastErr == nil {
		lastErr = ErrInvalidRouteDecision
	}
	return RouteDecisionPlan{}, lastErr
}

func (p *OpenAICompatibleRoutePlanner) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := plannerRequestTimeout(p.cfg.RequestTimeout)
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func plannerRequestTimeout(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return defaultPlannerRequestTimeout
	}
	if timeout < 0 {
		return 0
	}
	return timeout
}

type openAICompatibleHTTPChatCompletionClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func newOpenAICompatibleHTTPChatCompletionClient(baseURL, apiKey string, requestTimeout ...time.Duration) *openAICompatibleHTTPChatCompletionClient {
	timeout := defaultPlannerRequestTimeout
	if len(requestTimeout) > 0 {
		timeout = requestTimeout[0]
	}
	return &openAICompatibleHTTPChatCompletionClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *openAICompatibleHTTPChatCompletionClient) CreateChatCompletion(ctx context.Context, req OpenAICompatibleChatRequest) (string, error) {
	payload := openAICompatibleChatCompletionRequest{
		Model: req.Model,
		Messages: []openAICompatibleChatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
		Temperature: req.Temperature,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
	}
	if req.MaxTokens > 0 {
		payload.MaxTokens = &req.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("chat completion status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	responseBody, err := readLimitedSuccessBody(resp.Body)
	if err != nil {
		return "", err
	}
	var decoded openAICompatibleChatCompletionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", errors.New("chat completion response missing content")
	}
	return decoded.Choices[0].Message.Content, nil
}

type openAICompatibleChatCompletionRequest struct {
	Model          string                        `json:"model"`
	Messages       []openAICompatibleChatMessage `json:"messages"`
	MaxTokens      *int                          `json:"max_tokens,omitempty"`
	Temperature    float64                       `json:"temperature"`
	ResponseFormat map[string]string             `json:"response_format"`
}

type openAICompatibleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAICompatibleChatCompletionResponse struct {
	Choices []struct {
		Message openAICompatibleChatMessage `json:"message"`
	} `json:"choices"`
}

func buildPlannerSystemPrompt() string {
	return strings.Join([]string{
		"You are the SuperTeam project coordination route planner.",
		"Return a single JSON object only; do not wrap it in markdown.",
		"The JSON object must match this schema: reason string, requires_human_review bool, tasks array, budget_estimate object, template_key string, planner_metadata object.",
		"Each task JSON object must include key, title, summary, selected_employee_id as a UUID string, employee_selection_reason, required_capabilities, matched_capabilities, missing_capabilities, permission_requirements, tool_requirements, runtime_requirements, verification_requirements, selection_score, expected_outputs, input_requirements, handoff_contract, blocked_by_keys, risk_level, and task_kind.",
		"Use selected_employee_id only from active executor candidates provided by the user prompt.",
		"For every task, choose selected_employee_id by comparing planning_profile facts; explain the choice in employee_selection_reason and copy the required, matched, and missing capability arrays.",
		"A task with missing_capabilities must set requires_human_approval or make the whole route requires_human_review true.",
		"If coordination_policy.require_human_review_for_new_demands is true, still return at least one concrete task and set requires_human_review plus every task requires_human_approval to true.",
	}, "\n")
}

func buildPlannerUserPrompt(snapshot CoordinationSnapshot) string {
	payload := plannerPromptSnapshot{
		ProjectID:            snapshot.ProjectID.String(),
		Demand:               snapshot.Demand,
		DigitalEmployeePool:  snapshot.DigitalEmployeePool,
		CoordinationPolicy:   snapshot.CoordinationPolicy,
		PreviousRouteContext: snapshot.PreviousRouteContext,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte("{}")
	}
	return fmt.Sprintf("Plan the project demand route. Respond with JSON only.\nSnapshot JSON:\n%s", string(body))
}

type plannerPromptSnapshot struct {
	ProjectID            string                  `json:"project_id"`
	Demand               DemandSnapshot          `json:"demand"`
	DigitalEmployeePool  []ProjectMemberSnapshot `json:"digital_employee_pool"`
	CoordinationPolicy   map[string]any          `json:"coordination_policy,omitempty"`
	PreviousRouteContext map[string]any          `json:"previous_route_context,omitempty"`
}

func decodePlannerJSON(content string) (RouteDecisionPlan, error) {
	var decoded plannerJSON
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return RouteDecisionPlan{}, err
	}
	plan := RouteDecisionPlan{
		Reason:              decoded.Reason,
		RequiresHumanReview: decoded.RequiresHumanReview,
		BudgetEstimate:      nonNilMap(decoded.BudgetEstimate),
		TemplateKey:         decoded.TemplateKey,
		PlannerMetadata:     sanitizePlannerMetadata(decoded.PlannerMetadata),
		Tasks:               make([]PlannedTask, 0, len(decoded.Tasks)),
	}
	for _, task := range decoded.Tasks {
		plan.Tasks = append(plan.Tasks, PlannedTask{
			Key:                      task.Key,
			Title:                    task.Title,
			Summary:                  task.Summary,
			SelectedEmployeeID:       task.SelectedEmployeeID,
			EmployeeSelectionReason:  task.EmployeeSelectionReason,
			RequiredCapabilities:     nonNilStrings(task.RequiredCapabilities),
			MatchedCapabilities:      nonNilStrings(task.MatchedCapabilities),
			MissingCapabilities:      nonNilStrings(task.MissingCapabilities),
			PermissionRequirements:   nonNilStrings(task.PermissionRequirements),
			ToolRequirements:         nonNilStrings(task.ToolRequirements),
			RuntimeRequirements:      nonNilStrings(task.RuntimeRequirements),
			VerificationRequirements: nonNilStrings(task.VerificationRequirements),
			SelectionScore:           task.SelectionScore,
			TaskKind:                 task.TaskKind,
			StageIndex:               task.StageIndex,
			RiskLevel:                task.RiskLevel,
			RequiresHumanApproval:    task.RequiresHumanApproval,
			ExpectedOutputs:          nonNilStrings(task.ExpectedOutputs),
			InputRequirements:        nonNilMap(task.InputRequirements),
			HandoffContract:          nonNilMap(task.HandoffContract),
			BlockedByKeys:            nonNilStrings(task.BlockedByKeys),
		})
	}
	return plan, nil
}

type plannerJSON struct {
	Reason              string         `json:"reason"`
	RequiresHumanReview bool           `json:"requires_human_review"`
	BudgetEstimate      map[string]any `json:"budget_estimate"`
	TemplateKey         string         `json:"template_key"`
	PlannerMetadata     map[string]any `json:"planner_metadata"`
	Tasks               []plannerTask  `json:"tasks"`
}

type plannerTask struct {
	Key                      string         `json:"key"`
	Title                    string         `json:"title"`
	Summary                  string         `json:"summary"`
	SelectedEmployeeID       uuid.UUID      `json:"selected_employee_id"`
	EmployeeSelectionReason  string         `json:"employee_selection_reason"`
	RequiredCapabilities     []string       `json:"required_capabilities"`
	MatchedCapabilities      []string       `json:"matched_capabilities"`
	MissingCapabilities      []string       `json:"missing_capabilities"`
	PermissionRequirements   []string       `json:"permission_requirements"`
	ToolRequirements         []string       `json:"tool_requirements"`
	RuntimeRequirements      []string       `json:"runtime_requirements"`
	VerificationRequirements []string       `json:"verification_requirements"`
	SelectionScore           int            `json:"selection_score"`
	TaskKind                 string         `json:"task_kind"`
	StageIndex               *int32         `json:"stage_index"`
	RiskLevel                string         `json:"risk_level"`
	RequiresHumanApproval    bool           `json:"requires_human_approval"`
	ExpectedOutputs          []string       `json:"expected_outputs"`
	InputRequirements        map[string]any `json:"input_requirements"`
	HandoffContract          map[string]any `json:"handoff_contract"`
	BlockedByKeys            []string       `json:"blocked_by_keys"`
}

func (t *plannerTask) UnmarshalJSON(data []byte) error {
	type plannerTaskJSON struct {
		Key                      string          `json:"key"`
		Title                    string          `json:"title"`
		Summary                  string          `json:"summary"`
		SelectedEmployeeID       uuid.UUID       `json:"selected_employee_id"`
		EmployeeSelectionReason  string          `json:"employee_selection_reason"`
		RequiredCapabilities     json.RawMessage `json:"required_capabilities"`
		MatchedCapabilities      json.RawMessage `json:"matched_capabilities"`
		MissingCapabilities      json.RawMessage `json:"missing_capabilities"`
		PermissionRequirements   json.RawMessage `json:"permission_requirements"`
		ToolRequirements         json.RawMessage `json:"tool_requirements"`
		RuntimeRequirements      json.RawMessage `json:"runtime_requirements"`
		VerificationRequirements json.RawMessage `json:"verification_requirements"`
		SelectionScore           int             `json:"selection_score"`
		TaskKind                 string          `json:"task_kind"`
		StageIndex               *int32          `json:"stage_index"`
		RiskLevel                string          `json:"risk_level"`
		RequiresHumanApproval    bool            `json:"requires_human_approval"`
		ExpectedOutputs          json.RawMessage `json:"expected_outputs"`
		InputRequirements        json.RawMessage `json:"input_requirements"`
		HandoffContract          json.RawMessage `json:"handoff_contract"`
		BlockedByKeys            []string        `json:"blocked_by_keys"`
	}
	var raw plannerTaskJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	inputRequirements, err := decodeRequiredPlannerObject(raw.InputRequirements, "input_requirements")
	if err != nil {
		return err
	}
	handoffContract, err := decodeRequiredPlannerObject(raw.HandoffContract, "handoff_contract")
	if err != nil {
		return err
	}
	*t = plannerTask{
		Key:                      raw.Key,
		Title:                    raw.Title,
		Summary:                  raw.Summary,
		SelectedEmployeeID:       raw.SelectedEmployeeID,
		EmployeeSelectionReason:  raw.EmployeeSelectionReason,
		RequiredCapabilities:     decodePlannerStringArray(raw.RequiredCapabilities),
		MatchedCapabilities:      decodePlannerStringArray(raw.MatchedCapabilities),
		MissingCapabilities:      decodePlannerStringArray(raw.MissingCapabilities),
		PermissionRequirements:   decodePlannerStringArray(raw.PermissionRequirements),
		ToolRequirements:         decodePlannerStringArray(raw.ToolRequirements),
		RuntimeRequirements:      decodePlannerStringArray(raw.RuntimeRequirements),
		VerificationRequirements: decodePlannerStringArray(raw.VerificationRequirements),
		SelectionScore:           raw.SelectionScore,
		TaskKind:                 raw.TaskKind,
		StageIndex:               raw.StageIndex,
		RiskLevel:                raw.RiskLevel,
		RequiresHumanApproval:    raw.RequiresHumanApproval,
		ExpectedOutputs:          decodePlannerStringArray(raw.ExpectedOutputs),
		InputRequirements:        inputRequirements,
		HandoffContract:          handoffContract,
		BlockedByKeys:            raw.BlockedByKeys,
	}
	return nil
}

// decodePlannerStringArray coerces a planner string-array field (expected_outputs,
// blocked_by_keys) into []string. Reasoning models sometimes emit a single string or a
// non-string scalar instead of an array; a scalar/string is wrapped as a one-element
// slice so a valid plan is not rejected on shape alone.
func decodePlannerStringArray(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if trimmed := strings.TrimSpace(single); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	}
	return nil
}

// decodeRequiredPlannerObject coerces a planner task field into map[string]any.
// Reasoning models sometimes emit input_requirements / handoff_contract as a JSON
// array or scalar rather than the object the schema asks for; normalizing here keeps
// a valid plan from being rejected on shape alone. An object is kept as-is; an array
// is wrapped as {"items": [...]}; any other scalar becomes {"value": ...}.
func decodeRequiredPlannerObject(raw json.RawMessage, field string) (map[string]any, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, fmt.Errorf("planner task %s must be a JSON object", field)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err == nil && object != nil {
		return object, nil
	}
	var array []any
	if err := json.Unmarshal(raw, &array); err == nil {
		return map[string]any{"items": array}, nil
	}
	var scalar any
	if err := json.Unmarshal(raw, &scalar); err == nil && scalar != nil {
		return map[string]any{"value": scalar}, nil
	}
	return nil, fmt.Errorf("planner task %s must be a JSON object", field)
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func sanitizePlannerMetadata(metadata map[string]any) map[string]any {
	sanitized := map[string]any{}
	for key, value := range metadata {
		if plannerMetadataKeyDenied(key) {
			continue
		}
		sanitized[key] = value
	}
	return sanitized
}

func plannerMetadataKeyDenied(key string) bool {
	canonical := canonicalPlannerMetadataKey(key)
	deniedFragments := []string{
		"prompt",
		"rawmodel",
		"rawresponse",
		"rawcompletion",
		"rawmessage",
		"rawcontent",
		"rawoutput",
		"rawtext",
	}
	for _, fragment := range deniedFragments {
		if strings.Contains(canonical, fragment) {
			return true
		}
	}
	return false
}

func canonicalPlannerMetadataKey(key string) string {
	key = strings.ToLower(key)
	var builder strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func applyRequiredHumanReviewPolicy(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) {
	if plan == nil || !requiredHumanReviewPolicyEnabled(snapshot.CoordinationPolicy) {
		return
	}
	plan.RequiresHumanReview = true
	for i := range plan.Tasks {
		plan.Tasks[i].RequiresHumanApproval = true
	}
}

func synthesizeRequiredReviewPlan(snapshot CoordinationSnapshot, pool []uuid.UUID, source RouteDecisionPlan) RouteDecisionPlan {
	if len(pool) == 0 {
		return source
	}
	title := strings.TrimSpace(snapshot.Demand.Title)
	if title == "" {
		title = "处理项目需求"
	}
	summary := strings.TrimSpace(snapshot.Demand.Content)
	if summary == "" {
		summary = title
	}
	reason := strings.TrimSpace(source.Reason)
	if reason == "" {
		reason = "协调策略要求先进行人类审核，因此生成一个待审核的执行任务图"
	}
	expectedOutputs := []string{"execution_summary", "evidence_refs", "recommended_next_action"}
	stageIndex := int32(0)
	metadata := clonePlannerMap(source.PlannerMetadata)
	metadata["planner_repair"] = "policy_required_human_review"
	metadata["repair_reason"] = "model_output_invalid_for_required_review"
	templateKey := strings.TrimSpace(source.TemplateKey)
	if templateKey == "" {
		templateKey = "policy.required_human_review.single_task"
	}
	budgetEstimate := clonePlannerMap(source.BudgetEstimate)
	if len(budgetEstimate) == 0 {
		budgetEstimate["mode"] = "policy_default"
	}
	return RouteDecisionPlan{
		Reason:              reason,
		RequiresHumanReview: true,
		BudgetEstimate:      budgetEstimate,
		TemplateKey:         templateKey,
		PlannerMetadata:     metadata,
		Tasks: []PlannedTask{{
			Key:                   "required_review_execute_demand",
			Title:                 title,
			Summary:               summary,
			SelectedEmployeeID:    pool[0],
			TaskKind:              "execution",
			StageIndex:            &stageIndex,
			RiskLevel:             "normal",
			RequiresHumanApproval: true,
			ExpectedOutputs:       expectedOutputs,
			InputRequirements: map[string]any{
				"demand_id":             snapshot.Demand.ID.String(),
				"title":                 title,
				"content":               snapshot.Demand.Content,
				"requires_route_review": true,
			},
			HandoffContract: map[string]any{
				"expected_outputs": stringsToAny(expectedOutputs),
				"completion_path":  "project_task_attempt_writeback",
			},
		}},
	}
}

func requiredHumanReviewPolicyEnabled(policy map[string]any) bool {
	value, ok := policy["require_human_review_for_new_demands"].(bool)
	return ok && value
}

func clonePlannerMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func readLimitedSuccessBody(body io.Reader) ([]byte, error) {
	responseBody, err := io.ReadAll(io.LimitReader(body, maxChatCompletionResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(responseBody) > maxChatCompletionResponseBytes {
		return nil, fmt.Errorf("chat completion response too large")
	}
	return responseBody, nil
}

func terminalContextError(ctx context.Context) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return nil
}

func plannerContextError(parentCtx, requestCtx context.Context, err error) (error, bool) {
	if parentErr := terminalContextError(parentCtx); parentErr != nil {
		return parentErr, true
	}
	if errors.Is(err, context.DeadlineExceeded) && errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrPlannerRequestTimeout, err), false
	}
	var timeoutErr interface{ Timeout() bool }
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return fmt.Errorf("%w: %w", ErrPlannerRequestTimeout, err), false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err, false
	}
	return nil, false
}
