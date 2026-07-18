package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/capability"
	"github.com/superteam/control-plane/internal/feishu"
	"github.com/superteam/control-plane/internal/cost"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/prompttemplate"
	"github.com/superteam/control-plane/internal/scenariotemplate"
	"github.com/superteam/control-plane/internal/serviceauth"
	"github.com/superteam/control-plane/internal/skill"
	"github.com/superteam/control-plane/internal/teamlending"
	"github.com/superteam/control-plane/internal/tenant"
)

type Server struct {
	router                         *chi.Mux
	taskHandler                    *handlers.TaskHandler
	runtimeHandler                 *handlers.RuntimeHandler
	runtimeCommandWritebackHandler *handlers.RuntimeCommandWritebackHandler
	runtimeAuthService             middleware.AuthService
	runtimeSessionAuth             middleware.RuntimeSessionAuthService
	authService                    *auth.Service
	authorizer                     authz.Authorizer
	auditHandler                   *audit.HTTPHandler
	authzCenterHandler             *authzcenter.HTTPHandler
	capabilityHandler              *capability.HTTPHandler
	employeeHandler                *employee.HTTPHandler
	costHandler                    *cost.HTTPHandler
	inboxHandler                   *inbox.HTTPHandler
	projectHandler                 *project.HTTPHandler
	promptTemplateHandler          *prompttemplate.HTTPHandler
	scenarioTemplateHandler        *scenariotemplate.HTTPHandler
	skillHandler                   *skill.HTTPHandler
	tenantHandler                  *tenant.HTTPHandler
	teamLendingHandler             *teamlending.HTTPHandler
	serviceAuthService             middleware.ServiceAuthService
	onBehalfOfResolver             middleware.OnBehalfOfResolver
	serviceTokenHandler            *serviceauth.HTTPHandler
	feishuConnectorHandler         *feishu.ConnectorHTTPHandler
	feishuAdminHandler             *feishu.AdminHTTPHandler
	feishuOAuthHandler             *feishu.OAuthHTTPHandler
}

func NewServer(taskHandler *handlers.TaskHandler, runtimeHandler *handlers.RuntimeHandler, runtimeAuthService ...middleware.AuthService) *Server {
	var authService middleware.AuthService
	if len(runtimeAuthService) > 0 {
		authService = runtimeAuthService[0]
	}

	s := &Server{
		taskHandler:        taskHandler,
		runtimeHandler:     runtimeHandler,
		runtimeAuthService: authService,
	}

	s.registerRoutes()
	return s
}

func NewServerWithRuntimeSessionAuth(
	taskHandler *handlers.TaskHandler,
	runtimeHandler *handlers.RuntimeHandler,
	runtimeAuthService middleware.AuthService,
	runtimeSessionAuth middleware.RuntimeSessionAuthService,
) *Server {
	server := NewServer(taskHandler, runtimeHandler, runtimeAuthService)
	server.runtimeSessionAuth = runtimeSessionAuth
	server.registerRoutes()
	return server
}

func NewServerWithAuth(taskHandler *handlers.TaskHandler, runtimeHandler *handlers.RuntimeHandler, authService *auth.Service, runtimeAuthService ...middleware.AuthService) *Server {
	var runtimeAuth middleware.AuthService
	if len(runtimeAuthService) > 0 {
		runtimeAuth = runtimeAuthService[0]
	}
	return NewServerWithAuthz(taskHandler, runtimeHandler, authService, runtimeAuth, nil)
}

func NewServerWithAuthz(
	taskHandler *handlers.TaskHandler,
	runtimeHandler *handlers.RuntimeHandler,
	authService *auth.Service,
	runtimeAuthService middleware.AuthService,
	authorizer authz.Authorizer,
	authzCenterHandlers ...*authzcenter.HTTPHandler,
) *Server {
	server := NewServer(taskHandler, runtimeHandler, runtimeAuthService)
	server.authService = authService
	server.authorizer = authorizer
	if len(authzCenterHandlers) > 0 {
		server.authzCenterHandler = authzCenterHandlers[0]
	}
	if authorizer != nil && runtimeHandler != nil {
		runtimeHandler.SetAuthorizer(authorizer)
	}
	if authorizer != nil && taskHandler != nil {
		taskHandler.SetAuthorizer(authorizer)
	}
	server.registerRoutes()
	return server
}

func NewServerWithAuthzAndRuntimeSessionAuth(
	taskHandler *handlers.TaskHandler,
	runtimeHandler *handlers.RuntimeHandler,
	authService *auth.Service,
	runtimeAuthService middleware.AuthService,
	runtimeSessionAuth middleware.RuntimeSessionAuthService,
	authorizer authz.Authorizer,
	authzCenterHandlers ...*authzcenter.HTTPHandler,
) *Server {
	server := NewServerWithAuthz(taskHandler, runtimeHandler, authService, runtimeAuthService, authorizer, authzCenterHandlers...)
	server.runtimeSessionAuth = runtimeSessionAuth
	server.registerRoutes()
	return server
}

func (s *Server) SetEmployeeHandler(employeeHandler *employee.HTTPHandler) {
	s.employeeHandler = employeeHandler
	if employeeHandler != nil {
		employeeHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetCostHandler(costHandler *cost.HTTPHandler) {
	s.costHandler = costHandler
	if costHandler != nil {
		costHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetInboxHandler(inboxHandler *inbox.HTTPHandler) {
	s.inboxHandler = inboxHandler
	if inboxHandler != nil {
		inboxHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetAuditHandler(auditHandler *audit.HTTPHandler) {
	s.auditHandler = auditHandler
	if auditHandler != nil {
		auditHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetProjectHandler(projectHandler *project.HTTPHandler) {
	s.projectHandler = projectHandler
	if projectHandler != nil {
		projectHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetTenantHandler(tenantHandler *tenant.HTTPHandler) {
	s.tenantHandler = tenantHandler
	if tenantHandler != nil {
		tenantHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetTeamLendingHandler(teamLendingHandler *teamlending.HTTPHandler) {
	s.teamLendingHandler = teamLendingHandler
	if teamLendingHandler != nil {
		teamLendingHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetSkillHandler(skillHandler *skill.HTTPHandler) {
	s.skillHandler = skillHandler
	if skillHandler != nil {
		skillHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetCapabilityHandler(capabilityHandler *capability.HTTPHandler) {
	s.capabilityHandler = capabilityHandler
	if capabilityHandler != nil {
		capabilityHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetPromptTemplateHandler(promptTemplateHandler *prompttemplate.HTTPHandler) {
	s.promptTemplateHandler = promptTemplateHandler
	s.registerRoutes()
}

func (s *Server) SetScenarioTemplateHandler(scenarioTemplateHandler *scenariotemplate.HTTPHandler) {
	s.scenarioTemplateHandler = scenarioTemplateHandler
	if scenarioTemplateHandler != nil {
		scenarioTemplateHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

// SetServiceAuth 配置外部服务凭据认证与 on-behalf-of 核验(/api/v1/connector/* 依赖)。
func (s *Server) SetServiceAuth(authService middleware.ServiceAuthService, resolver middleware.OnBehalfOfResolver) {
	s.serviceAuthService = authService
	s.onBehalfOfResolver = resolver
	s.registerRoutes()
}

func (s *Server) SetServiceTokenHandler(handler *serviceauth.HTTPHandler) {
	s.serviceTokenHandler = handler
	if s.authorizer != nil && handler != nil {
		handler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetFeishuHandlers(connectorHandler *feishu.ConnectorHTTPHandler, adminHandler *feishu.AdminHTTPHandler) {
	s.feishuConnectorHandler = connectorHandler
	s.feishuAdminHandler = adminHandler
	if s.authorizer != nil && adminHandler != nil {
		adminHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetFeishuOAuthHandler(handler *feishu.OAuthHTTPHandler) {
	s.feishuOAuthHandler = handler
	s.registerRoutes()
}

func (s *Server) SetRuntimeCommandWritebackHandler(runtimeCommandWritebackHandler *handlers.RuntimeCommandWritebackHandler) {
	s.runtimeCommandWritebackHandler = runtimeCommandWritebackHandler
	s.registerRoutes()
}

func (s *Server) registerRoutes() {
	s.router = chi.NewRouter()
	s.router.Use(middleware.Recovery())
	s.router.Use(middleware.Logger())
	s.router.Use(middleware.CORS())

	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeHealthResponse(w)
	})
	if s.authService != nil {
		auth.HandlerFromMux(auth.NewHandler(s.authService, s.authorizer), s.router)
	}
	if s.authzCenterHandler != nil {
		authzcenter.HandlerFromMux(s.authzCenterHandler, s.router)
	}

	s.router.Route("/api/v1", func(r chi.Router) {
		r.Route("/tasks", func(r chi.Router) {
			if s.authService != nil {
				r.Use(middleware.ConsoleUserAuth(s.authService))
			}
			r.Post("/", s.taskHandler.CreateTask)
			r.Get("/", s.taskHandler.ListTasks)
			r.Get("/{id}", s.taskHandler.GetTask)
			r.Put("/{id}/status", s.taskHandler.UpdateTaskStatus)
			r.Post("/{id}/cancel", s.taskHandler.CancelTask)
		})

		// 外部服务通道:仅服务凭据可达,业务动作以 on-behalf-of 绑定用户判权。
		if s.feishuConnectorHandler != nil && s.serviceAuthService != nil {
			r.Route("/connector", func(r chi.Router) {
				r.Use(middleware.ServiceAuth(s.serviceAuthService, s.onBehalfOfResolver))
				r.Get("/bootstrap", s.feishuConnectorHandler.Bootstrap)
				r.Get("/identity", s.feishuConnectorHandler.Identity)
				r.Get("/outbox", s.feishuConnectorHandler.ListOutbox)
				r.Post("/outbox/{outboxId}/ack", s.feishuConnectorHandler.AckOutbox)
				r.Get("/my-projects", s.feishuConnectorHandler.MyProjects)
				r.Post("/demands", s.feishuConnectorHandler.SubmitDemand)
				r.Post("/decisions/{decisionId}/resolve", s.feishuConnectorHandler.ResolveDecision)
			})
		}

		if s.serviceTokenHandler != nil && s.authService != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Post("/admin/service-tokens", s.serviceTokenHandler.IssueToken)
				r.Delete("/admin/service-tokens/{tokenId}", s.serviceTokenHandler.RevokeToken)
			})
		}

		if s.feishuAdminHandler != nil && s.authService != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Post("/admin/feishu/app-configs", s.feishuAdminHandler.UpsertAppConfig)
				r.Get("/admin/feishu/app-configs", s.feishuAdminHandler.ListAppConfigs)
				r.Post("/admin/feishu/contact-sync", s.feishuAdminHandler.ContactSync)
				r.Get("/admin/feishu/identities", s.feishuAdminHandler.ListIdentities)
			})
		}

		if s.feishuOAuthHandler != nil {
			if s.authService != nil {
				r.Group(func(r chi.Router) {
					r.Use(middleware.ConsoleUserAuth(s.authService))
					r.Get("/auth/feishu/oauth-start", s.feishuOAuthHandler.Start)
				})
			}
			// Callback 无会话中间件:一次性 state 即凭证(来源于 Start 的会话)。
			r.Get("/auth/feishu/oauth-callback", s.feishuOAuthHandler.Callback)
		}

		if s.employeeHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/digital-employee-avatar-assets", s.employeeHandler.ListDigitalEmployeeAvatarAssets)
				r.Get("/digital-employees", s.employeeHandler.ListDigitalEmployees)
				r.Post("/digital-employees", s.employeeHandler.CreateDigitalEmployee)
				r.Get("/digital-employees/create-options", s.employeeHandler.GetCreateOptions)
				r.Get("/digital-employees/overview", s.employeeHandler.GetOverview)
				r.Get("/digital-employees/activity", s.employeeHandler.GetActivity)
				r.Get("/digital-employees/activity/stream", s.employeeHandler.StreamActivity)
				r.Get("/digital-employees/{employeeId}", s.employeeHandler.GetDigitalEmployee)
				r.Delete("/digital-employees/{employeeId}", s.employeeHandler.DeleteDigitalEmployee)
				r.Get("/digital-employees/{employeeId}/scheduling-readiness", s.employeeHandler.GetSchedulingReadiness)
				r.Get("/digital-employees/{employeeId}/environment-variables", s.employeeHandler.ListEnvironmentVariables)
				r.Put("/digital-employees/{employeeId}/environment-variables/{envName}", s.employeeHandler.UpsertEnvironmentVariable)
				r.Delete("/digital-employees/{employeeId}/environment-variables/{envName}", s.employeeHandler.DeleteEnvironmentVariable)
				r.Put("/digital-employees/{employeeId}/status", s.employeeHandler.UpdateDigitalEmployeeStatus)
				r.Put("/digital-employees/{employeeId}/team", s.employeeHandler.ReassignDigitalEmployeeTeam)
				r.Get("/digital-employees/{employeeId}/execution-instance", s.employeeHandler.GetDigitalEmployeeExecutionInstance)
				r.Put("/digital-employees/{employeeId}/execution-instance", s.employeeHandler.UpsertDigitalEmployeeExecutionInstance)
				r.Post("/digital-employees/{employeeId}/config-revisions", s.employeeHandler.CreateDigitalEmployeeConfigRevision)
				r.Post("/digital-employees/{employeeId}/runs", s.employeeHandler.CreateDigitalEmployeeRun)
				r.Get("/digital-employees/{employeeId}/runs", s.employeeHandler.ListDigitalEmployeeRuns)
				r.Get("/digital-employees/{employeeId}/run-stats", s.employeeHandler.GetDigitalEmployeeRunStats)
				r.Get("/digital-employees/{employeeId}/runs/{runId}", s.employeeHandler.GetDigitalEmployeeRun)
				r.Get("/digital-employees/{employeeId}/runs/{runId}/events", s.employeeHandler.ListDigitalEmployeeRunEvents)
				r.Post("/digital-employees/{employeeId}/runs/{runId}/stop", s.employeeHandler.StopDigitalEmployeeRun)
				r.Get("/digital-employee-templates", s.employeeHandler.ListEmployeeTemplates)
				r.Post("/digital-employee-templates", s.employeeHandler.CreateEmployeeTemplate)
				r.Get("/digital-employee-templates/{templateId}", s.employeeHandler.GetEmployeeTemplate)
				r.Patch("/digital-employee-templates/{templateId}", s.employeeHandler.UpdateEmployeeTemplate)
				r.Patch("/digital-employee-templates/{templateId}/status", s.employeeHandler.SetEmployeeTemplateStatus)
				r.Delete("/digital-employee-templates/{templateId}", s.employeeHandler.DeleteEmployeeTemplate)
			})
		}

		if s.projectHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/workflow-instances", s.projectHandler.ListWorkflowInstances)
				r.Get("/projects", s.projectHandler.ListProjects)
				r.Post("/projects", s.projectHandler.CreateProject)
				r.Get("/projects/{projectId}", s.projectHandler.GetProject)
				r.Patch("/projects/{projectId}", s.projectHandler.UpdateProject)
				r.Post("/projects/{projectId}/archive", s.projectHandler.ArchiveProject)
				r.Get("/projects/{projectId}/delete-preview", s.projectHandler.GetProjectDeletePreview)
				r.Delete("/projects/{projectId}", s.projectHandler.DeleteProject)
				r.Get("/projects/{projectId}/overview", s.projectHandler.GetOverview)
				r.Get("/projects/{projectId}/runtime-placement", s.projectHandler.GetProjectRuntimePlacement)
				r.Put("/projects/{projectId}/runtime-placement", s.projectHandler.PutProjectRuntimePlacement)
				r.Delete("/projects/{projectId}/runtime-placement", s.projectHandler.ReleaseProjectRuntimePlacement)
				r.Get("/projects/{projectId}/runtime-readiness", s.projectHandler.GetProjectRuntimeReadiness)
				r.Get("/projects/{projectId}/runtime-nodes", s.projectHandler.ListProjectRuntimeNodes)
				r.Get("/projects/{projectId}/members", s.projectHandler.ListProjectMembers)
				r.Put("/projects/{projectId}/members", s.projectHandler.ReplaceProjectMembers)
				r.Get("/projects/{projectId}/tasks", s.projectHandler.ListProjectTasks)
				r.Get("/projects/{projectId}/tasks/{taskId}/liveness", s.projectHandler.GetProjectTaskLiveness)
				r.Get("/projects/{projectId}/task-graph", s.projectHandler.GetProjectTaskGraph)
				r.Get("/projects/{projectId}/events", s.projectHandler.ListProjectEvents)
				r.Post("/projects/{projectId}/events/{eventId}/retry-workflow-signal", s.projectHandler.RetryWorkflowSignal)
				r.Get("/projects/{projectId}/config", s.projectHandler.GetProjectConfig)
				r.Put("/projects/{projectId}/config", s.projectHandler.UpdateProjectConfig)
				r.Post("/projects/{projectId}/demands", s.projectHandler.SubmitDemand)
				r.Get("/projects/{projectId}/demands", s.projectHandler.ListProjectDemands)
				r.Get("/project-demands/{demandId}/launch-detail", s.projectHandler.GetDemandLaunchDetail)
				r.Get("/project-demands/{demandId}/acceptance-criteria", s.projectHandler.ListDemandAcceptanceCriteria)
				r.Post("/project-demands/{demandId}/criterion-verdicts", s.projectHandler.SignDemandCriterionVerdict)
				r.Get("/projects/{projectId}/route-decisions", s.projectHandler.ListRouteDecisions)
				r.Get("/projects/{projectId}/plan-revisions", s.projectHandler.ListPlanRevisions)
				r.Get("/projects/{projectId}/plan-revisions/{planRevisionId}", s.projectHandler.GetPlanRevision)
				r.Get("/projects/{projectId}/tasks/{taskId}/dispatch-gates", s.projectHandler.ListProjectTaskDispatchGates)
				r.Get("/projects/{projectId}/coordination-jobs", s.projectHandler.ListCoordinationJobs)
				r.Get("/projects/{projectId}/decisions", s.projectHandler.ListDecisionRequests)
				r.Post("/projects/{projectId}/decisions/{decisionId}/resolve", s.projectHandler.ResolveDecision)
				r.Get("/projects/{projectId}/execution-summaries", s.projectHandler.ListExecutionSummaries)
				r.Get("/projects/{projectId}/execution-trace", s.projectHandler.GetExecutionTrace)
				r.Get("/projects/{projectId}/transfer-requests", s.projectHandler.ListTransferRequests)
				r.Get("/projects/{projectId}/evidence", s.projectHandler.ListEvidence)
				r.Post("/projects/{projectId}/evidence", s.projectHandler.CreateEvidence)
				r.Patch("/projects/{projectId}/evidence/{evidenceId}", s.projectHandler.PatchEvidence)
				r.Get("/projects/{projectId}/artifacts", s.projectHandler.ListArtifacts)
				r.Get("/artifacts/{artifactRefId}/content", s.projectHandler.GetArtifactContent)
				r.Get("/projects/{projectId}/reports", s.projectHandler.ListReports)
				r.Get("/projects/{projectId}/budget-ledger", s.projectHandler.ListBudgetLedger)
				r.Get("/projects/{projectId}/budget-summary", s.projectHandler.GetBudgetSummary)
				r.Post("/projects/{projectId}/acceptance", s.projectHandler.CreateAcceptance)
				r.Get("/projects/{projectId}/acceptance", s.projectHandler.GetAcceptance)
				r.Get("/projects/{projectId}/archive-preview", s.projectHandler.GetArchivePreview)
				r.Post("/projects/{projectId}/archive-snapshot", s.projectHandler.CreateArchiveSnapshot)
				r.Get("/projects/{projectId}/archive-snapshots", s.projectHandler.ListArchiveSnapshots)
				r.Get("/projects/{projectId}/config-revisions", s.projectHandler.ListConfigRevisions)
				r.Get("/projects/{projectId}/config-revisions/{revisionId}", s.projectHandler.GetConfigRevision)
			})
		}

		if s.inboxHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/inbox/items", s.inboxHandler.ListItems)
				r.Get("/inbox/badge", s.inboxHandler.GetBadge)
				r.Post("/inbox/items/{itemId}/actions", s.inboxHandler.ExecuteAction)
			})
		}

		if s.costHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/costs/summary", s.costHandler.GetSummary)
				r.Get("/costs/employees", s.costHandler.ListEmployees)
			})
		}

		if s.tenantHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/teams", s.tenantHandler.ListTeams)
				r.Post("/teams", s.tenantHandler.CreateTeam)
				r.Get("/teams/{teamId}/overview", s.tenantHandler.GetTeamOverview)
				r.Get("/teams/{teamId}/audit", s.tenantHandler.ListTeamAudit)
				r.Patch("/teams/{teamId}", s.tenantHandler.UpdateTeam)
				r.Get("/teams/{teamId}", s.tenantHandler.GetTeam)
				r.Delete("/teams/{teamId}", s.tenantHandler.DeleteTeam)
				r.Patch("/teams/{teamId}/constitution", s.tenantHandler.UpdateTeamConstitution)
				r.Get("/teams/{teamId}/members", s.tenantHandler.ListTeamMembers)
				r.Post("/teams/{teamId}/members", s.tenantHandler.AddTeamMember)
				r.Post("/teams/{teamId}/digital-employees", s.tenantHandler.BindTeamDigitalEmployee)
				r.Delete("/teams/{teamId}/members/{memberId}", s.tenantHandler.RemoveTeamMember)
				r.Get("/teams/{teamId}/member-role-requests", s.tenantHandler.ListTeamMemberRoleRequests)
				r.Post("/teams/{teamId}/member-role-requests", s.tenantHandler.CreateTeamMemberRoleRequest)
				r.Post("/teams/{teamId}/member-role-requests/{requestId}/approve", s.tenantHandler.ApproveTeamMemberRoleRequest)
				r.Post("/teams/{teamId}/member-role-requests/{requestId}/reject", s.tenantHandler.RejectTeamMemberRoleRequest)
			})
		}

		if s.teamLendingHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				// 团队供给侧：借调策略 + 请求审批/撤销。
				r.Get("/teams/{teamId}/lending-policy", s.teamLendingHandler.GetLendingPolicy)
				r.Put("/teams/{teamId}/lending-policy", s.teamLendingHandler.UpsertLendingPolicy)
				r.Get("/teams/{teamId}/lending-requests", s.teamLendingHandler.ListTeamLendingRequests)
				r.Post("/teams/{teamId}/lending-requests/{requestId}/approve", s.teamLendingHandler.ApproveLendingRequest)
				r.Post("/teams/{teamId}/lending-requests/{requestId}/reject", s.teamLendingHandler.RejectLendingRequest)
				r.Post("/teams/{teamId}/lending-requests/{requestId}/revoke", s.teamLendingHandler.RevokeLendingRequest)
				// 项目需求侧：发起借调 + 查看。
				r.Post("/projects/{projectId}/lending-requests", s.teamLendingHandler.CreateProjectLendingRequest)
				r.Get("/projects/{projectId}/lending-requests", s.teamLendingHandler.ListProjectLendingRequests)
			})
		}

		if s.auditHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/audit/events", s.auditHandler.ListEvents)
			})
		}

		if s.skillHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/skills", s.skillHandler.ListSkills)
				r.Post("/skills/uploads", s.skillHandler.UploadSkill)
				r.Get("/skills/{skillId}", s.skillHandler.GetSkill)
				r.Delete("/skills/{skillId}", s.skillHandler.DeleteSkill)
				r.Post("/skills/{skillId}/install", s.skillHandler.InstallSkill)
				r.Get("/skills/{skillId}/installations", s.skillHandler.ListSkillInstallations)
				r.Get("/teams/{teamId}/skills", s.skillHandler.ListTeamSkills)
				r.Post("/teams/{teamId}/skills", s.skillHandler.BindTeamSkill)
				r.Delete("/teams/{teamId}/skills/{skillId}", s.skillHandler.UnbindTeamSkill)
				r.Get("/digital-employees/{employeeId}/skills", s.skillHandler.ListEffectiveEmployeeSkills)
				r.Post("/digital-employees/{employeeId}/skills", s.skillHandler.BindEmployeeSkill)
				r.Delete("/digital-employees/{employeeId}/skills/{skillId}", s.skillHandler.UnbindEmployeeSkill)
			})
		}

		if s.capabilityHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/user-credentials", s.capabilityHandler.ListCredentials)
				r.Post("/user-credentials", s.capabilityHandler.CreateCredential)
				r.Get("/teams/{teamId}/mcp-servers", s.capabilityHandler.ListTeamMCPServers)
				r.Post("/teams/{teamId}/mcp-servers", s.capabilityHandler.CreateTeamMCPServer)
				r.Delete("/teams/{teamId}/mcp-servers/{serverId}", s.capabilityHandler.DeleteTeamMCPServer)
				r.Get("/digital-employees/{employeeId}/mcp-bindings", s.capabilityHandler.ListEmployeeMCPBindings)
				r.Post("/digital-employees/{employeeId}/mcp-bindings", s.capabilityHandler.CreateEmployeeMCPBinding)
				r.Delete("/digital-employees/{employeeId}/mcp-bindings/{bindingId}", s.capabilityHandler.DeleteEmployeeMCPBinding)
				r.Get("/digital-employees/{employeeId}/effective-mcp-servers", s.capabilityHandler.ListEffectiveMCPServers)

				// MCP HTTP capability registry (migration 037).
				r.Get("/mcp-servers", s.capabilityHandler.ListMCPServerDefinitions)
				r.Post("/mcp-servers", s.capabilityHandler.CreateMCPServerDefinition)
				r.Delete("/mcp-servers/{serverId}", s.capabilityHandler.DeleteMCPServerDefinition)
				r.Get("/skills/{skillId}/mcp-dependencies", s.capabilityHandler.ListSkillMCPDependencies)
				r.Put("/skills/{skillId}/mcp-dependencies", s.capabilityHandler.ReplaceSkillMCPDependencies)
				r.Get("/mcp-servers/{serverId}/dependent-skills", s.capabilityHandler.ListDependentSkills)
				r.Post("/teams/{teamId}/mcp-bindings", s.capabilityHandler.CreateTeamMCPBinding)
				r.Get("/teams/{teamId}/mcp-bindings", s.capabilityHandler.ListTeamMCPBindings)
				r.Delete("/teams/{teamId}/mcp-bindings/{bindingId}", s.capabilityHandler.DeleteTeamMCPBinding)
				r.Post("/digital-employees/{employeeId}/mcp-bindings-v2", s.capabilityHandler.CreateEmployeeMCPBindingV2)
				r.Get("/digital-employees/{employeeId}/mcp-bindings-v2", s.capabilityHandler.ListEmployeeMCPBindingsV2)
				r.Delete("/digital-employees/{employeeId}/mcp-bindings-v2/{bindingId}", s.capabilityHandler.DeleteEmployeeMCPBindingV2)
				// 项目级 MCP 绑定（迁移 072）：项目公共 MCP 的注册表正门。
				r.Get("/projects/{projectId}/mcp-bindings", s.capabilityHandler.ListProjectMCPBindings)
				r.Put("/projects/{projectId}/mcp-bindings", s.capabilityHandler.PutProjectMCPBindings)
				r.Get("/digital-employees/{employeeId}/effective-mcp-config", s.capabilityHandler.ListEffectiveMCPConfig)
				r.Get("/digital-employees/{employeeId}/skill-mcp-dependency-status", s.capabilityHandler.ListEmployeeSkillMCPDependencyStatus)
			})
		}

		if s.promptTemplateHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/templates", s.promptTemplateHandler.ListPromptTemplates)
				r.Post("/templates", s.promptTemplateHandler.CreatePromptTemplate)
				r.Post("/templates/{id}/apply", s.promptTemplateHandler.ApplyPromptTemplate)
			})
		}

		if s.scenarioTemplateHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				// Scenario template registry (migration 058); write endpoints
				// (migration 061 version history) require scenario_template.manage.
				r.Get("/scenario-templates", s.scenarioTemplateHandler.ListScenarioTemplates)
				r.Post("/scenario-templates", s.scenarioTemplateHandler.CreateScenarioTemplate)
				r.Get("/scenario-templates/{templateKey}", s.scenarioTemplateHandler.GetScenarioTemplate)
				r.Patch("/scenario-templates/{templateKey}", s.scenarioTemplateHandler.PatchScenarioTemplate)
				r.Post("/scenario-templates/{templateKey}/versions", s.scenarioTemplateHandler.CreateScenarioTemplateVersion)
				r.Get("/scenario-templates/{templateKey}/versions", s.scenarioTemplateHandler.ListScenarioTemplateVersions)
			})
		}

		r.Route("/runtime", func(r chi.Router) {
			r.Get("/nodes", s.runtimeHandler.ListNodes)
			r.Get("/nodes/{id}", s.runtimeHandler.GetNodeByID)
			r.Post("/enrollments/hello", s.runtimeHandler.EnrollHello)
			r.Post("/enroll/hello", s.runtimeHandler.EnrollHello)

			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/overview", s.runtimeHandler.GetOverview)
				r.Get("/events", s.runtimeHandler.ListRuntimeEvents)
				r.Get("/nodes/{nodeId}/capabilities", s.runtimeHandler.ListRuntimeCapabilitiesForNode)
				r.Get("/enrollments", s.runtimeHandler.ListRuntimeEnrollments)
				r.Post("/enrollments/{enrollmentId}/approve", s.runtimeHandler.ApproveEnrollment)
				r.Post("/enrollments/{enrollmentId}/reject", s.runtimeHandler.RejectEnrollment)
				r.Post("/enrollments/{enrollmentId}/revoke", s.runtimeHandler.RevokeEnrollment)
			})

			r.Group(func(r chi.Router) {
				if s.runtimeAuthService != nil {
					r.Use(middleware.RuntimeAuth(s.runtimeAuthService))
				}
				r.Post("/register", s.runtimeHandler.RegisterNode)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RuntimeSessionAuth(s.runtimeSessionAuth))
				r.Post("/session/renew", s.runtimeHandler.RenewRuntimeSession)
				r.Post("/sessions/{sessionId}/renew", s.runtimeHandler.RenewRuntimeSession)
				r.Put("/nodes/{nodeId}/capabilities", s.runtimeHandler.UpsertCapabilities)
				r.Post("/capabilities", s.runtimeHandler.UpsertCapabilities)
				if s.runtimeCommandWritebackHandler != nil {
					r.Post("/commands/{commandId}/events", s.runtimeCommandWritebackHandler.RecordEvent)
					r.Post("/commands/{commandId}/complete", s.runtimeCommandWritebackHandler.Complete)
					r.Post("/commands/{commandId}/fail", s.runtimeCommandWritebackHandler.Fail)
					r.Post("/commands/{commandId}/cancelled", s.runtimeCommandWritebackHandler.Cancel)
					r.Post("/commands/{commandId}/timed-out", s.runtimeCommandWritebackHandler.TimedOut)
				}
				if s.projectHandler != nil {
					r.Post("/project-task-attestations", s.projectHandler.CreateProjectTaskAttestation)
					r.Post("/artifacts/presign", s.projectHandler.PresignRuntimeArtifact)
					r.Post("/raw-logs/presign", s.projectHandler.PresignRuntimeRawLog)
					r.Route("/project-task-attempts/{attemptId}", func(r chi.Router) {
						r.Post("/started", s.projectHandler.StartProjectTaskAttempt)
						r.Post("/lease", s.projectHandler.RenewProjectTaskAttemptLease)
						r.Post("/budget-heartbeat", s.projectHandler.RecordProjectTaskAttemptBudgetHeartbeat)
						r.Post("/complete", s.projectHandler.CompleteProjectTaskAttempt)
						r.Post("/result", s.projectHandler.SubmitProjectTaskAttemptResult)
						r.Post("/fail", s.projectHandler.FailProjectTaskAttempt)
						r.Post("/wait-human", s.projectHandler.WaitHumanProjectTaskAttempt)
					})
				}
				if s.skillHandler != nil {
					r.Post("/skills/presign", s.skillHandler.PresignRuntimeSkillArchive)
				}
				r.Get("/ws", s.runtimeHandler.WebSocket)
			})

			r.Group(func(r chi.Router) {
				if s.runtimeAuthService != nil || s.runtimeSessionAuth != nil {
					r.Use(middleware.RuntimeSessionOrLegacyAuth(s.runtimeSessionAuth, s.runtimeAuthService))
				}
				r.Post("/heartbeat", s.runtimeHandler.Heartbeat)
				r.Post("/tasks/claim", s.runtimeHandler.ClaimTask)
				r.Put("/tasks/{id}/status", s.runtimeHandler.UpdateTaskStatus)
				r.Post("/tasks/{id}/events", s.runtimeHandler.PushEvents)
				r.Post("/tasks/{id}/complete", s.runtimeHandler.CompleteTask)
				r.Post("/tasks/{id}/fail", s.runtimeHandler.FailTask)
				r.Post("/tasks/{id}/lease", s.runtimeHandler.RenewLease)
			})
		})
	})
}

func (s *Server) Start(addr string) error {
	return s.ListenAndServe(context.Background(), addr)
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	httpServer := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		if err := httpServer.Shutdown(context.Background()); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}
