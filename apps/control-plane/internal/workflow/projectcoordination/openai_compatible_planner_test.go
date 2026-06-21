package projectcoordination

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompatibleRoutePlannerParsesJSONGraph(t *testing.T) {
	employeeID := uuid.New()
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxTokens:   1024,
		MaxAttempts: 1,
	}, fakeChatCompletionClient{content: fmt.Sprintf(`{
		"reason":"split demand",
		"requires_human_review":false,
		"tasks":[
			{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"employee_selection_reason":"具备 execution 能力","required_capabilities":["execution"],"matched_capabilities":["execution"],"missing_capabilities":[],"permission_requirements":[],"tool_requirements":[],"runtime_requirements":[],"verification_requirements":["写回 project task attempt 结果"],"selection_score":0,"stage_index":0,"expected_outputs":["execution_summary"],"input_requirements":{},"handoff_contract":{},"blocked_by_keys":[],"risk_level":"medium","task_kind":"analysis"}
		],
		"budget_estimate":{"mode":"planner"},
		"template_key":"default",
		"planner_metadata":{"provider":"openai-compatible"}
	}`, employeeID.String())})

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestExecutorMember(employeeID),
		},
	})

	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, employeeID, plan.Tasks[0].SelectedEmployeeID)
	require.Equal(t, int32(0), *plan.Tasks[0].StageIndex)
}

func TestOpenAICompatibleRoutePlannerUnavailableConfigDoesNotCallClient(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  OpenAICompatiblePlannerConfig
	}{
		{
			name: "missing api key",
			cfg:  OpenAICompatiblePlannerConfig{BaseURL: "https://planner.example", Model: "planner-model"},
		},
		{
			name: "missing base url",
			cfg:  OpenAICompatiblePlannerConfig{APIKey: "test-key", Model: "planner-model"},
		},
		{
			name: "missing model",
			cfg:  OpenAICompatiblePlannerConfig{APIKey: "test-key", BaseURL: "https://planner.example"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &countingChatCompletionClient{content: `{}`}
			planner := NewOpenAICompatibleRoutePlanner(tc.cfg, client)

			_, err := planner.Plan(context.Background(), CoordinationSnapshot{})

			require.ErrorIs(t, err, ErrPlannerUnavailable)
			require.Equal(t, int32(0), client.calls.Load())
		})
	}
}

func TestOpenAICompatibleRoutePlannerRetriesInvalidOutput(t *testing.T) {
	employeeID := uuid.New()
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "invalid json", content: `not-json`},
		{name: "invalid graph", content: fmt.Sprintf(`{
			"reason":"split demand",
			"requires_human_review":false,
			"tasks":[
				{"key":" bad ","title":"分析","summary":"分析需求","selected_employee_id":%q,"expected_outputs":["execution_summary"],"input_requirements":{},"handoff_contract":{}}
			]
		}`, employeeID.String())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &countingChatCompletionClient{content: tc.content}
			planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
				APIKey:      "test-key",
				BaseURL:     "https://planner.example",
				Model:       "planner-model",
				MaxAttempts: 3,
			}, client)

			_, err := planner.Plan(context.Background(), CoordinationSnapshot{
				Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
				DigitalEmployeePool: []ProjectMemberSnapshot{
					{PrincipalID: employeeID, ProjectRole: "executor", Status: "active"},
				},
			})

			require.Error(t, err)
			require.Equal(t, int32(3), client.calls.Load())
		})
	}
}

func TestOpenAICompatibleRoutePlannerSynthesizesReviewPlanWhenPolicyRequiresHumanReview(t *testing.T) {
	employeeID := uuid.New()
	client := &countingChatCompletionClient{content: `{
		"reason":"pause for owner review before dispatch",
		"requires_human_review":true,
		"tasks":[],
		"budget_estimate":{"mode":"planner"},
		"template_key":"route_review",
		"planner_metadata":{"provider":"openai-compatible"}
	}`}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "删除生产数据", Content: "需要先确认风险"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestExecutorMember(employeeID),
		},
		CoordinationPolicy: map[string]any{"require_human_review_for_new_demands": true},
	})

	require.NoError(t, err)
	require.True(t, plan.RequiresHumanReview)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, employeeID, plan.Tasks[0].SelectedEmployeeID)
	require.True(t, plan.Tasks[0].RequiresHumanApproval)
	require.NotEmpty(t, plan.Tasks[0].EmployeeSelectionReason)
	require.Equal(t, []string{"execution"}, plan.Tasks[0].RequiredCapabilities)
	require.Equal(t, []string{"execution"}, plan.Tasks[0].MatchedCapabilities)
	require.NotZero(t, plan.Tasks[0].SelectionScore)
	require.NotEmpty(t, plan.Tasks[0].VerificationRequirements)
	require.NotEmpty(t, plan.Tasks[0].PlanningProfileSnapshotHash)
	require.NotEmpty(t, plan.Tasks[0].ExpectedOutputs)
	require.NotEmpty(t, plan.Tasks[0].InputRequirements)
	require.NotEmpty(t, plan.Tasks[0].HandoffContract)
	require.Equal(t, int32(1), client.calls.Load())
}

func TestOpenAICompatibleRoutePlannerRejectsRequiredReviewRepairWithoutValidPlanningProfile(t *testing.T) {
	employeeID := uuid.New()

	for _, tc := range []struct {
		name   string
		member ProjectMemberSnapshot
	}{
		{
			name: "missing profile",
			member: ProjectMemberSnapshot{
				PrincipalID: employeeID,
				ProjectRole: "executor",
				Status:      "active",
			},
		},
		{
			name: "mismatched profile identity",
			member: func() ProjectMemberSnapshot {
				member := openAITestExecutorMember(employeeID)
				member.PlanningProfile.DigitalEmployeeID = uuid.New()
				return member
			}(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &countingChatCompletionClient{content: `{
				"reason":"pause for owner review before dispatch",
				"requires_human_review":true,
				"tasks":[],
				"budget_estimate":{"mode":"planner"},
				"template_key":"route_review",
				"planner_metadata":{"provider":"openai-compatible"}
			}`}
			planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
				APIKey:      "test-key",
				BaseURL:     "https://planner.example",
				Model:       "planner-model",
				MaxAttempts: 1,
			}, client)

			_, err := planner.Plan(context.Background(), CoordinationSnapshot{
				Demand: DemandSnapshot{ID: uuid.New(), Title: "删除生产数据", Content: "需要先确认风险"},
				DigitalEmployeePool: []ProjectMemberSnapshot{
					tc.member,
				},
				CoordinationPolicy: map[string]any{"require_human_review_for_new_demands": true},
			})

			require.ErrorIs(t, err, ErrInvalidRouteDecision)
			require.Equal(t, int32(1), client.calls.Load())
		})
	}
}

func TestOpenAICompatibleRoutePlannerRejectsMissingRequiredTaskMaps(t *testing.T) {
	employeeID := uuid.New()
	for _, tc := range []struct {
		name string
		task string
	}{
		{
			name: "missing input requirements",
			task: fmt.Sprintf(`{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"expected_outputs":["execution_summary"],"handoff_contract":{}}`, employeeID.String()),
		},
		{
			name: "null input requirements",
			task: fmt.Sprintf(`{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"expected_outputs":["execution_summary"],"input_requirements":null,"handoff_contract":{}}`, employeeID.String()),
		},
		{
			name: "missing handoff contract",
			task: fmt.Sprintf(`{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"expected_outputs":["execution_summary"],"input_requirements":{}}`, employeeID.String()),
		},
		{
			name: "null handoff contract",
			task: fmt.Sprintf(`{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"expected_outputs":["execution_summary"],"input_requirements":{},"handoff_contract":null}`, employeeID.String()),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &countingChatCompletionClient{content: fmt.Sprintf(`{
				"reason":"split demand",
				"requires_human_review":false,
				"tasks":[%s]
			}`, tc.task)}
			planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
				APIKey:      "test-key",
				BaseURL:     "https://planner.example",
				Model:       "planner-model",
				MaxAttempts: 2,
			}, client)

			_, err := planner.Plan(context.Background(), CoordinationSnapshot{
				Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
				DigitalEmployeePool: []ProjectMemberSnapshot{
					{PrincipalID: employeeID, ProjectRole: "executor", Status: "active"},
				},
			})

			require.Error(t, err)
			require.Equal(t, int32(2), client.calls.Load())
		})
	}
}

func TestOpenAICompatibleRoutePlannerNormalizesNonObjectRequirementMaps(t *testing.T) {
	// Reasoning models sometimes emit input_requirements/handoff_contract as an array
	// or scalar; the planner normalizes these into objects instead of rejecting the plan.
	employeeID := uuid.New()
	client := &countingChatCompletionClient{content: fmt.Sprintf(`{
		"reason":"split demand",
		"requires_human_review":false,
		"tasks":[{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"employee_selection_reason":"具备 execution 能力","required_capabilities":["execution"],"matched_capabilities":["execution"],"missing_capabilities":[],"permission_requirements":[],"tool_requirements":[],"runtime_requirements":[],"verification_requirements":["写回 project task attempt 结果"],"selection_score":0,"expected_outputs":["execution_summary"],"input_requirements":["a","b"],"handoff_contract":"none"}]
	}`, employeeID.String())}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 2,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestExecutorMember(employeeID),
		},
	})

	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, []any{"a", "b"}, plan.Tasks[0].InputRequirements["items"])
	require.Equal(t, "none", plan.Tasks[0].HandoffContract["value"])
}

func TestOpenAICompatibleRoutePlannerDoesNotRetryContextDone(t *testing.T) {
	employeeID := uuid.New()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &countingChatCompletionClient{err: tc.err}
			planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
				APIKey:      "test-key",
				BaseURL:     "https://planner.example",
				Model:       "planner-model",
				MaxAttempts: 3,
			}, client)

			_, err := planner.Plan(context.Background(), CoordinationSnapshot{
				Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
				DigitalEmployeePool: []ProjectMemberSnapshot{
					{PrincipalID: employeeID, ProjectRole: "executor", Status: "active"},
				},
			})

			require.ErrorIs(t, err, tc.err)
			require.Equal(t, int32(1), client.calls.Load())
		})
	}
}

func TestOpenAICompatibleRoutePlannerDoesNotCallClientWhenContextAlreadyDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &countingChatCompletionClient{content: `{}`}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 3,
	}, client)

	_, err := planner.Plan(ctx, CoordinationSnapshot{})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int32(0), client.calls.Load())
}

func TestOpenAICompatibleHTTPChatCompletionClientBuildsRequest(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var gotContentType string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"reason\":\"ok\",\"tasks\":[]}"}}]}`))
	}))
	defer server.Close()

	client := newOpenAICompatibleHTTPChatCompletionClient(server.URL+"/", "test-key")

	content, err := client.CreateChatCompletion(context.Background(), OpenAICompatibleChatRequest{
		Model:       "planner-model",
		System:      "system json",
		User:        "user json",
		MaxTokens:   1024,
		Temperature: 0.2,
	})

	require.NoError(t, err)
	require.JSONEq(t, `{"reason":"ok","tasks":[]}`, content)
	require.Equal(t, "/chat/completions", gotPath)
	require.Equal(t, "Bearer test-key", gotAuthorization)
	require.Equal(t, "application/json", gotContentType)
	require.Equal(t, "planner-model", gotBody["model"])
	require.Equal(t, float64(1024), gotBody["max_tokens"])
	require.Equal(t, 0.2, gotBody["temperature"])
	require.Len(t, gotBody["messages"], 2)
	responseFormat, ok := gotBody["response_format"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "json_object", responseFormat["type"])
}

func TestOpenAICompatibleHTTPChatCompletionClientBuildsQwenRequest(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"reason\":\"ok\",\"tasks\":[]}"}}]}`))
	}))
	defer server.Close()

	client := newOpenAICompatibleHTTPChatCompletionClient(server.URL+"/", "test-key")

	content, err := client.CreateChatCompletion(context.Background(), OpenAICompatibleChatRequest{
		Model:       "qwen-plus",
		System:      "system json",
		User:        "user json",
		MaxTokens:   2048,
		Temperature: 0.1,
	})

	require.NoError(t, err)
	require.JSONEq(t, `{"reason":"ok","tasks":[]}`, content)
	require.Equal(t, "qwen-plus", gotBody["model"])
	require.Equal(t, float64(2048), gotBody["max_tokens"])
}

func TestOpenAICompatibleHTTPChatCompletionClientErrorsAreProviderNeutral(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newOpenAICompatibleHTTPChatCompletionClient(server.URL, "test-key")

	_, err := client.CreateChatCompletion(context.Background(), OpenAICompatibleChatRequest{
		Model:  "qwen-plus",
		System: "system json",
		User:   "user json",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "chat completion status 429")
	require.NotContains(t, strings.ToLower(err.Error()), "deepseek")
	require.NotContains(t, strings.ToLower(err.Error()), "qwen")
	require.NotContains(t, strings.ToLower(err.Error()), "openai-compatible")
}

func TestOpenAICompatibleHTTPChatCompletionClientOmitsNonPositiveMaxTokens(t *testing.T) {
	for _, maxTokens := range []int{0, -1} {
		t.Run(fmt.Sprintf("max_tokens_%d", maxTokens), func(t *testing.T) {
			var gotBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
			}))
			defer server.Close()

			client := newOpenAICompatibleHTTPChatCompletionClient(server.URL, "test-key")

			_, err := client.CreateChatCompletion(context.Background(), OpenAICompatibleChatRequest{
				Model:       "planner-model",
				System:      "system json",
				User:        "user json",
				MaxTokens:   maxTokens,
				Temperature: 0,
			})

			require.NoError(t, err)
			require.NotContains(t, gotBody, "max_tokens")
		})
	}
}

func TestOpenAICompatibleHTTPChatCompletionClientRejectsOversizedSuccessBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat(" ", 2*1024*1024)))
	}))
	defer server.Close()
	client := newOpenAICompatibleHTTPChatCompletionClient(server.URL, "test-key")

	_, err := client.CreateChatCompletion(context.Background(), OpenAICompatibleChatRequest{
		Model:  "planner-model",
		System: "system json",
		User:   "user json",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "too large")
}

func TestOpenAICompatibleRoutePlannerPromptsIncludeJSONWord(t *testing.T) {
	employeeID := uuid.New()
	client := &capturingChatCompletionClient{content: fmt.Sprintf(`{
		"reason":"split demand",
		"requires_human_review":false,
		"tasks":[
			{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"employee_selection_reason":"具备 execution 能力","required_capabilities":["execution"],"matched_capabilities":["execution"],"missing_capabilities":[],"permission_requirements":[],"tool_requirements":[],"runtime_requirements":[],"verification_requirements":["写回 project task attempt 结果"],"selection_score":0,"expected_outputs":["execution_summary"],"input_requirements":{},"handoff_contract":{}}
		]
	}`, employeeID.String())}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	_, err := planner.Plan(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestExecutorMember(employeeID),
		},
	})

	require.NoError(t, err)
	require.Contains(t, strings.ToLower(client.req.System), "json")
	require.Contains(t, strings.ToLower(client.req.User), "json")
}

func TestOpenAICompatiblePlannerPromptIncludesPlanningProfiles(t *testing.T) {
	employeeID := uuid.New()
	client := &capturingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"按能力选择数据库分析员工",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库",
				"summary":"检查慢查询",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备 database.read 和 sql.analysis",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":["database.read","sql.analysis"],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功"],
				"selection_score":100,
				"expected_outputs":["execution_summary"],
				"input_requirements":{},
				"handoff_contract":{},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, employeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	_, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{
			ID:      uuid.New(),
			Title:   "分析数据库",
			Content: "检查慢查询",
		},
		DigitalEmployeePool: []ProjectMemberSnapshot{{
			PrincipalID: employeeID,
			ProjectRole: "executor",
			Status:      "active",
			DisplayName: "数据库员工",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: employeeID,
				RoleProfile: PlanningRoleProfile{
					PrimaryRole: "data_analyst",
				},
				Capabilities: []PlanningCapability{
					{
						Key:        "database.read",
						Level:      "strong",
						Source:     "test",
						Confidence: 1,
					},
					{
						Key:        "sql.analysis",
						Level:      "strong",
						Source:     "test",
						Confidence: 1,
					},
				},
				Skills: []PlanningSkill{{
					Key:    "sql.analysis",
					Source: "test",
				}},
				ToolBindings: []PlanningToolBinding{{
					Type:   "mcp",
					Key:    "postgres.readonly",
					Status: "available",
				}},
				RuntimeRequirements: PlanningRuntimeRequirements{
					ProviderTypes:  []string{"codex"},
					ProviderStatus: "ready",
				},
				Permissions: []PlanningPermission{{
					Scope:    "database.read",
					Resource: "dev_database",
					Status:   "granted",
				}},
				LoadState: PlanningLoadState{
					AvailableSlots: 1,
					Lendable:       true,
				},
				ProfileFreshness: PlanningProfileFreshness{
					SourceState: "ready",
				},
			},
		}},
	})

	require.NoError(t, err)
	require.Contains(t, client.req.User, `"planning_profile"`)
	require.Contains(t, client.req.User, `"database.read"`)
	require.Contains(t, client.req.System, "employee_selection_reason")
	require.Contains(t, client.req.System, "required_capabilities")
}

func TestOpenAICompatiblePlannerAppliesProfileScoresToAcceptedPlan(t *testing.T) {
	employeeID := uuid.New()
	client := &countingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"按能力选择数据库分析员工",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库",
				"summary":"检查慢查询",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备数据库分析经验",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":[],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功"],
				"selection_score":0,
				"expected_outputs":["execution_summary"],
				"input_requirements":{},
				"handoff_contract":{},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, employeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)
	snapshot := CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{
			ID:      uuid.New(),
			Title:   "分析数据库",
			Content: "检查慢查询",
		},
		DigitalEmployeePool: []ProjectMemberSnapshot{openAITestDatabaseMember(employeeID)},
	}

	plan, err := planner.Plan(context.Background(), snapshot)

	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	task := plan.Tasks[0]
	require.Equal(t, 100, task.SelectionScore)
	require.Equal(t, []string{"database.read", "sql.analysis", "data.quality.check", "business.metric.interpretation"}, task.MatchedCapabilities)
	require.Empty(t, task.MissingCapabilities)
	require.Equal(t, PlanningProfileSnapshotHash(*snapshot.DigitalEmployeePool[0].PlanningProfile), task.PlanningProfileSnapshotHash)
}

func TestOpenAICompatiblePlannerDatabaseAnalysisRequiresDatabaseProfile(t *testing.T) {
	dbEmployeeID := uuid.New()
	genericEmployeeID := uuid.New()
	client := &countingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"数据库分析需要具备 database.read 的员工",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库异常",
				"summary":"检查慢查询和异常状态",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备 database.read、sql.analysis 和 postgres.readonly",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":["database.read","sql.analysis"],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功","结果包含证据引用"],
				"selection_score":100,
				"expected_outputs":["execution_summary","evidence_refs"],
				"input_requirements":{"scope":"database_analysis"},
				"handoff_contract":{"completion_path":"project_task_attempt_writeback"},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{"mode":"planner"},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, dbEmployeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "分析数据库异常", Content: "找出订单状态异常原因"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestDatabaseMember(dbEmployeeID),
			openAITestExecutorMember(genericEmployeeID),
		},
	})

	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, dbEmployeeID, plan.Tasks[0].SelectedEmployeeID)
	require.Equal(t, []string{"database.read", "sql.analysis", "data.quality.check", "business.metric.interpretation"}, plan.Tasks[0].MatchedCapabilities)
	require.Empty(t, plan.Tasks[0].MissingCapabilities)
	require.NotEmpty(t, plan.Tasks[0].PlanningProfileSnapshotHash)
}

func TestOpenAICompatiblePlannerAcceptsNormalizedSelectionScore(t *testing.T) {
	employeeID := uuid.New()
	client := &countingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"数据库分析需要具备 database.read 的员工",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库异常",
				"summary":"检查慢查询和异常状态",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备 database.read、sql.analysis 和 postgres.readonly",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":["database.read","sql.analysis"],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功","结果包含证据引用"],
				"selection_score":0.95,
				"expected_outputs":["execution_summary","evidence_refs"],
				"input_requirements":{"scope":"database_analysis"},
				"handoff_contract":{"completion_path":"project_task_attempt_writeback"},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{"mode":"planner"},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, employeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "分析数据库异常", Content: "找出订单状态异常原因"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestDatabaseMember(employeeID),
		},
	})

	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, employeeID, plan.Tasks[0].SelectedEmployeeID)
	require.Equal(t, 100, plan.Tasks[0].SelectionScore)
}

func TestOpenAICompatiblePlannerMarksProfileGapsForHumanReview(t *testing.T) {
	employeeID := uuid.New()
	client := &countingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"数据库写入操作需要人工确认",
			"requires_human_review":false,
			"tasks":[{
				"key":"write-db",
				"title":"处理数据库异常",
				"summary":"需要执行数据库写入修复",
				"selected_employee_id":%q,
				"employee_selection_reason":"数据库员工最接近需求",
				"required_capabilities":["database.write"],
				"matched_capabilities":["database.read"],
				"missing_capabilities":[],
				"permission_requirements":["database.write:dev_database"],
				"tool_requirements":["mcp:postgres.admin"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["人工确认写入风险"],
				"selection_score":0.7,
				"expected_outputs":["execution_summary","risk_summary"],
				"input_requirements":{"scope":"database_write"},
				"handoff_contract":{"completion_path":"project_task_attempt_writeback"},
				"blocked_by_keys":[],
				"risk_level":"high",
				"task_kind":"database_repair"
			}],
			"budget_estimate":{"mode":"planner"},
			"template_key":"database_repair",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, employeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "修复数据库异常", Content: "需要写入修复"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestDatabaseMember(employeeID),
		},
	})

	require.NoError(t, err)
	require.True(t, plan.RequiresHumanReview)
	require.True(t, plan.Tasks[0].RequiresHumanApproval)
	require.Equal(t, []string{"database.write"}, plan.Tasks[0].MissingCapabilities)
	require.NotEmpty(t, plan.Tasks[0].PlanningProfileSnapshotHash)
}

func TestOpenAICompatiblePlannerOverwritesDriftedSelectionEvidence(t *testing.T) {
	employeeID := uuid.New()
	client := &countingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"按能力选择数据库分析员工",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库",
				"summary":"检查慢查询",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备数据库分析经验",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":["model.guess"],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功"],
				"selection_score":100,
				"expected_outputs":["execution_summary"],
				"input_requirements":{},
				"handoff_contract":{},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, employeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "分析数据库", Content: "检查慢查询"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestDatabaseMember(employeeID),
		},
	})

	require.NoError(t, err)
	require.Equal(t, int32(1), client.calls.Load())
	require.Equal(t, []string{"database.read", "sql.analysis", "data.quality.check", "business.metric.interpretation"}, plan.Tasks[0].MatchedCapabilities)
	require.Empty(t, plan.Tasks[0].MissingCapabilities)
	require.Equal(t, 100, plan.Tasks[0].SelectionScore)
}

func TestOpenAICompatiblePlannerOverwritesPolicyReviewSelectionEvidence(t *testing.T) {
	employeeID := uuid.New()
	client := &countingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"策略要求人工审核但模型证据不一致",
			"requires_human_review":true,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库",
				"summary":"检查慢查询",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备数据库分析经验",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":["model.guess"],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功"],
				"selection_score":100,
				"expected_outputs":["execution_summary"],
				"input_requirements":{},
				"handoff_contract":{},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"requires_human_approval":true,
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, employeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "分析数据库", Content: "检查慢查询"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestDatabaseMember(employeeID),
		},
		CoordinationPolicy: map[string]any{"require_human_review_for_new_demands": true},
	})

	require.NoError(t, err)
	require.Equal(t, int32(1), client.calls.Load())
	require.Equal(t, []string{"database.read", "sql.analysis", "data.quality.check", "business.metric.interpretation"}, plan.Tasks[0].MatchedCapabilities)
	require.Empty(t, plan.Tasks[0].MissingCapabilities)
	require.Equal(t, 100, plan.Tasks[0].SelectionScore)
}

func TestOpenAICompatiblePlannerDecodesSelectionEvidence(t *testing.T) {
	employeeID := uuid.New()
	content := fmt.Sprintf(`{
		"reason":"按能力选择",
		"requires_human_review":false,
		"tasks":[{
			"key":"analyze-db",
			"title":"分析数据库",
			"summary":"检查慢查询",
			"selected_employee_id":%q,
			"employee_selection_reason":"具备 database.read 和 sql.analysis",
			"required_capabilities":["database.read","sql.analysis"],
			"matched_capabilities":["database.read","sql.analysis"],
			"missing_capabilities":[],
			"permission_requirements":["database.read:dev_database"],
			"tool_requirements":["mcp:postgres.readonly"],
			"runtime_requirements":["provider:codex"],
			"verification_requirements":["只读查询成功"],
			"selection_score":100,
			"expected_outputs":["execution_summary"],
			"input_requirements":{},
			"handoff_contract":{},
			"blocked_by_keys":[],
			"risk_level":"medium",
			"task_kind":"database_analysis"
		}],
		"budget_estimate":{},
		"template_key":"database_analysis",
		"planner_metadata":{}
	}`, employeeID.String())

	plan, err := decodePlannerJSON(content)

	require.NoError(t, err)
	task := plan.Tasks[0]
	require.Equal(t, "具备 database.read 和 sql.analysis", task.EmployeeSelectionReason)
	require.Equal(t, []string{"database.read", "sql.analysis"}, task.RequiredCapabilities)
	require.Equal(t, []string{"database.read", "sql.analysis"}, task.MatchedCapabilities)
	require.Empty(t, task.MissingCapabilities)
	require.Equal(t, []string{"database.read:dev_database"}, task.PermissionRequirements)
	require.Equal(t, []string{"mcp:postgres.readonly"}, task.ToolRequirements)
	require.Equal(t, []string{"provider:codex"}, task.RuntimeRequirements)
	require.Equal(t, []string{"只读查询成功"}, task.VerificationRequirements)
	require.Equal(t, 100, task.SelectionScore)
}

func TestSanitizePlannerMetadataRemovesPromptAndRawVariants(t *testing.T) {
	metadata := sanitizePlannerMetadata(map[string]any{
		"provider":     "openai-compatible",
		"model":        "planner-model",
		"rawResponse":  "model text",
		"raw-response": "model text",
		"raw_model":    "model text",
		"prompt":       "prompt text",
		"systemPrompt": "prompt text",
	})

	require.Equal(t, "openai-compatible", metadata["provider"])
	require.Equal(t, "planner-model", metadata["model"])
	require.NotContains(t, metadata, "rawResponse")
	require.NotContains(t, metadata, "raw-response")
	require.NotContains(t, metadata, "raw_model")
	require.NotContains(t, metadata, "prompt")
	require.NotContains(t, metadata, "systemPrompt")
}

func TestActivitiesPlanDemandRouteSurfacesPlannerErrorWithoutFallback(t *testing.T) {
	// Planning is reasoning-only: there is no non-reasoning fallback, so a planner
	// error must surface instead of degrading to a heuristic fan-out.
	activities := NewActivities(nil, failingRoutePlanner{err: errors.New("planner failed")})

	plan, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: uuid.New(), ProjectRole: "executor", Status: "active"},
		},
	})

	require.Error(t, err)
	require.Empty(t, plan.Tasks)
}

func TestActivitiesPlanDemandRouteRequiresConfiguredPlanner(t *testing.T) {
	activities := NewActivities(nil)

	_, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: uuid.New(), ProjectRole: "executor", Status: "active"},
		},
	})

	require.ErrorIs(t, err, ErrRoutePlannerRequired)
}

func TestActivitiesPlanDemandRouteSurfacesDeadlineWithoutFallback(t *testing.T) {
	activities := NewActivities(nil, failingRoutePlanner{err: context.DeadlineExceeded})

	plan, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: uuid.New(), ProjectRole: "executor", Status: "active"},
		},
	})

	require.Error(t, err)
	require.Empty(t, plan.Tasks)
}

func TestOpenAICompatibleRoutePlannerTimesOutRequestBeforeActivityDeadline(t *testing.T) {
	employeeID := uuid.New()
	client := &blockingChatCompletionClient{}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:         "test-key",
		BaseURL:        "https://planner.example",
		Model:          "planner-model",
		MaxAttempts:    3,
		RequestTimeout: 10 * time.Millisecond,
	}, client)

	started := time.Now()
	_, err := planner.Plan(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: employeeID, ProjectRole: "executor", Status: "active"},
		},
	})

	require.ErrorIs(t, err, ErrPlannerRequestTimeout)
	require.Equal(t, int32(1), client.calls.Load())
	require.Less(t, time.Since(started), time.Second)
}

func TestPlannerContextErrorClassifiesHTTPTimeoutAsPlannerTimeout(t *testing.T) {
	err, terminal := plannerContextError(context.Background(), context.Background(), timeoutTestError{})

	require.False(t, terminal)
	require.ErrorIs(t, err, ErrPlannerRequestTimeout)
}

func TestActivitiesPlanDemandRouteDoesNotFallBackWhenActivityContextDone(t *testing.T) {
	employeeID := uuid.New()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			activities := NewActivities(nil, failingRoutePlanner{err: tc.err})
			ctx, cancel := context.WithCancel(context.Background())
			if errors.Is(tc.err, context.DeadlineExceeded) {
				ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
				<-ctx.Done()
			} else {
				cancel()
			}
			defer cancel()

			plan, err := activities.PlanDemandRoute(ctx, CoordinationSnapshot{
				Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
				DigitalEmployeePool: []ProjectMemberSnapshot{
					{PrincipalID: employeeID, ProjectRole: "executor", Status: "active"},
				},
			})

			require.ErrorIs(t, err, tc.err)
			require.Empty(t, plan.Tasks)
		})
	}
}

type fakeChatCompletionClient struct {
	content string
	err     error
}

func (f fakeChatCompletionClient) CreateChatCompletion(ctx context.Context, req OpenAICompatibleChatRequest) (string, error) {
	_ = ctx
	_ = req
	return f.content, f.err
}

type countingChatCompletionClient struct {
	content string
	err     error
	calls   atomic.Int32
}

func (f *countingChatCompletionClient) CreateChatCompletion(ctx context.Context, req OpenAICompatibleChatRequest) (string, error) {
	_ = ctx
	_ = req
	f.calls.Add(1)
	return f.content, f.err
}

type blockingChatCompletionClient struct {
	calls atomic.Int32
}

func (f *blockingChatCompletionClient) CreateChatCompletion(ctx context.Context, req OpenAICompatibleChatRequest) (string, error) {
	_ = req
	f.calls.Add(1)
	<-ctx.Done()
	return "", ctx.Err()
}

type timeoutTestError struct{}

func (timeoutTestError) Error() string {
	return "client timeout"
}

func (timeoutTestError) Timeout() bool {
	return true
}

type capturingChatCompletionClient struct {
	content string
	req     OpenAICompatibleChatRequest
}

func (f *capturingChatCompletionClient) CreateChatCompletion(ctx context.Context, req OpenAICompatibleChatRequest) (string, error) {
	_ = ctx
	f.req = req
	return f.content, nil
}

func openAITestExecutorMember(employeeID uuid.UUID) ProjectMemberSnapshot {
	return ProjectMemberSnapshot{
		PrincipalID:     employeeID,
		ProjectRole:     "executor",
		Status:          "active",
		PlanningProfile: openAITestExecutorProfile(employeeID),
	}
}

func openAITestExecutorProfile(employeeID uuid.UUID) *DigitalEmployeePlanningProfile {
	return &DigitalEmployeePlanningProfile{
		DigitalEmployeeID: employeeID,
		RoleProfile:       PlanningRoleProfile{PrimaryRole: "executor"},
		Capabilities: []PlanningCapability{{
			Key:        "execution",
			Level:      "strong",
			Source:     "test",
			Confidence: 1,
		}},
		RuntimeRequirements: PlanningRuntimeRequirements{
			ProviderTypes:  []string{"codex"},
			ProviderStatus: "ready",
		},
		LoadState:        PlanningLoadState{AvailableSlots: 1, Lendable: true},
		ProfileFreshness: PlanningProfileFreshness{SourceState: "ready"},
	}
}

func openAITestDatabaseMember(employeeID uuid.UUID) ProjectMemberSnapshot {
	return ProjectMemberSnapshot{
		PrincipalID:     employeeID,
		ProjectRole:     "executor",
		Status:          "active",
		DisplayName:     "数据库员工",
		PlanningProfile: openAITestDatabaseProfile(employeeID),
	}
}

func openAITestDatabaseProfile(employeeID uuid.UUID) *DigitalEmployeePlanningProfile {
	return &DigitalEmployeePlanningProfile{
		DigitalEmployeeID: employeeID,
		RoleProfile:       PlanningRoleProfile{PrimaryRole: "data_analyst"},
		Capabilities: []PlanningCapability{
			{Key: "database.read", Level: "strong", Source: "test", Confidence: 1},
			{Key: "sql.analysis", Level: "strong", Source: "test", Confidence: 1},
			{Key: "data.quality.check", Level: "strong", Source: "test", Confidence: 1},
			{Key: "business.metric.interpretation", Level: "strong", Source: "test", Confidence: 1},
		},
		Skills:       []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
		ToolBindings: []PlanningToolBinding{{Type: "mcp", Key: "postgres.readonly", Status: "available"}},
		RuntimeRequirements: PlanningRuntimeRequirements{
			ProviderTypes:  []string{"codex"},
			ProviderStatus: "ready",
		},
		Permissions:      []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}},
		LoadState:        PlanningLoadState{AvailableSlots: 1, Lendable: true},
		ProfileFreshness: PlanningProfileFreshness{SourceState: "ready"},
	}
}

// TestOpenAICompatiblePlannerDatabaseAnalysisRejectsUnderCapableEmployee is the
// spec §12 reverse-case integration test: a database_analysis task assigned to an
// employee without database capabilities must NOT be silently auto-dispatched.
// The model emits no required_capabilities; platform defaults fill them in, scoring
// records the gaps, and the plan is upgraded to human review instead of dispatching.
func TestOpenAICompatiblePlannerDatabaseAnalysisRejectsUnderCapableEmployee(t *testing.T) {
	underCapableID := uuid.New()
	client := &countingChatCompletionClient{
		content: fmt.Sprintf(`{
			"reason":"按角色分配数据库分析",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库异常",
				"summary":"检查慢查询",
				"selected_employee_id":%q,
				"employee_selection_reason":"该员工当前空闲",
				"required_capabilities":[],
				"matched_capabilities":[],
				"missing_capabilities":[],
				"permission_requirements":[],
				"tool_requirements":[],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":[],
				"selection_score":80,
				"expected_outputs":["execution_summary"],
				"input_requirements":{"scope":"database_analysis"},
				"handoff_contract":{"completion_path":"project_task_attempt_writeback"},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{"mode":"planner"},
			"template_key":"database_analysis",
			"planner_metadata":{}
		}`, underCapableID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "test-key",
		BaseURL:     "https://planner.example",
		Model:       "planner-model",
		MaxAttempts: 1,
	}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "分析数据库异常", Content: "找出订单状态异常原因"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			// Only a generic executor with no database capabilities is available.
			openAITestExecutorMember(underCapableID),
		},
	})

	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	task := plan.Tasks[0]
	// Defaults were applied even though the model emitted none.
	require.Contains(t, task.RequiredCapabilities, "database.read")
	require.Contains(t, task.RequiredCapabilities, "sql.analysis")
	// Scoring detected the gaps against the under-capable employee.
	require.Contains(t, task.MissingCapabilities, "database.read")
	require.Contains(t, task.MissingCapabilities, "sql.analysis")
	// The plan must route to human review rather than silently auto-dispatching.
	require.True(t, plan.RequiresHumanReview, "under-capable selection must require human review")
	require.True(t, task.RequiresHumanApproval, "under-capable task must require human approval")
	require.NotEmpty(t, task.PlanningProfileSnapshotHash)
}

type failingRoutePlanner struct {
	err error
}

func (p failingRoutePlanner) Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	_ = ctx
	_ = snapshot
	return RouteDecisionPlan{}, p.err
}
