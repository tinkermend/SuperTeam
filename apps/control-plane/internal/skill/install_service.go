package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	cpruntime "github.com/superteam/control-plane/internal/runtime"
)

const (
	defaultInstallTimeout      = 15 * time.Second
	defaultInstallPollInterval = 100 * time.Millisecond
)

var supportedInstallProviders = map[string]struct{}{
	"opencode":    {},
	"codex":       {},
	"claude-code": {},
}

type InstallRepository interface {
	GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error)
	ListInstallTargets(ctx context.Context, req ListSkillInstallTargetsRequest) ([]SkillInstallTarget, error)
	CreateInstallCommandReceipt(ctx context.Context, req CreateSkillInstallCommandReceiptRequest) error
	MarkInstallCommandFailed(ctx context.Context, tenantID uuid.UUID, commandID string, message string) error
	MarkInstallCommandTimedOut(ctx context.Context, tenantID uuid.UUID, commandID string, message string) error
	WaitForInstallCommand(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeInstallCommandReceipt, error)
	PersistInstallSuccess(ctx context.Context, req PersistSkillInstallSuccessRequest) (InstallSkillResult, error)
	RecordInstallFailure(ctx context.Context, req SkillInstallFailureLog) error
}

type InstallRuntimeDispatcher interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command cpruntime.RuntimeCommand) error
}

type InstallServiceOptions struct {
	Timeout      time.Duration
	PollInterval time.Duration
}

type InstallService struct {
	repository   InstallRepository
	dispatcher   InstallRuntimeDispatcher
	timeout      time.Duration
	pollInterval time.Duration
}

func NewInstallService(repository InstallRepository, dispatcher InstallRuntimeDispatcher, options InstallServiceOptions) *InstallService {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultInstallTimeout
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultInstallPollInterval
	}
	return &InstallService{repository: repository, dispatcher: dispatcher, timeout: timeout, pollInterval: pollInterval}
}

func (s *InstallService) InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	if s == nil || s.repository == nil || s.dispatcher == nil {
		return InstallSkillResult{}, fmt.Errorf("%w: install service is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil || req.SkillID == uuid.Nil {
		return InstallSkillResult{}, fmt.Errorf("%w: tenant_id and skill_id are required", ErrInvalidInput)
	}
	if req.TargetScope != SkillInstallTargetTeam && req.TargetScope != SkillInstallTargetEmployee {
		return InstallSkillResult{}, fmt.Errorf("%w: target_scope must be team or employee", ErrInvalidInput)
	}

	skill, err := s.repository.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
	if err != nil {
		return InstallSkillResult{}, err
	}
	targets, err := s.repository.ListInstallTargets(ctx, ListSkillInstallTargetsRequest{
		TenantID:          req.TenantID,
		SkillID:           req.SkillID,
		TargetScope:       req.TargetScope,
		TeamID:            req.TeamID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return InstallSkillResult{}, err
	}

	blockers := s.preflight(req.TargetScope, skill, targets)
	result := InstallSkillResult{
		SkillID:           req.SkillID,
		TargetScope:       req.TargetScope,
		TeamID:            req.TeamID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		BlockedTargets:    blockers,
	}
	if len(blockers) > 0 {
		message := "skill install preflight failed"
		_ = s.recordFailure(ctx, req, InstallFailurePhasePreflight, blockers[0].ReasonCode, message, "", blockers)
		return result, &InstallSkillError{Phase: InstallFailurePhasePreflight, Message: message, BlockedTargets: blockers}
	}

	installations, err := s.installTargetsSequentially(ctx, req, skill, targets)
	if err != nil {
		return result, err
	}
	return s.repository.PersistInstallSuccess(ctx, PersistSkillInstallSuccessRequest{
		TenantID:      req.TenantID,
		SkillID:       req.SkillID,
		TargetScope:   req.TargetScope,
		TeamID:        req.TeamID,
		InstalledBy:   req.ActorUserID,
		Installations: installations,
	})
}

type dispatchedInstallCommand struct {
	CommandID     string
	RuntimeNodeID uuid.UUID
	NodeID        string
	Installations []SkillInstallation
}

type dispatchInstallCommandError struct {
	CommandID string
	Err       error
}

func (e *dispatchInstallCommandError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *dispatchInstallCommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (s *InstallService) installTargetsSequentially(ctx context.Context, req InstallSkillRequest, skill *Skill, targets []SkillInstallTarget) ([]SkillInstallation, error) {
	grouped, nodeOrder := groupInstallTargetsByNode(targets)
	installations := make([]SkillInstallation, 0, len(targets))
	appliedNodes := make([]string, 0, len(nodeOrder))
	for _, nodeID := range nodeOrder {
		command, err := s.dispatchInstallCommand(ctx, req, skill, nodeID, grouped[nodeID])
		if err != nil {
			var dispatchErr *dispatchInstallCommandError
			commandID := ""
			if errors.As(err, &dispatchErr) {
				commandID = dispatchErr.CommandID
			}
			s.recordPartialFailure(ctx, req, InstallFailurePhaseRuntimeInstall, "runtime_dispatch_failed", err.Error(), commandID, appliedNodes)
			return nil, err
		}
		waitCtx, cancel := context.WithTimeout(ctx, timeoutOrDefault(req.Timeout, s.timeout))
		receipt, waitErr := s.repository.WaitForInstallCommand(waitCtx, req.TenantID, command.CommandID, s.pollInterval)
		cancel()
		if waitErr != nil {
			if errors.Is(waitErr, context.DeadlineExceeded) {
				_ = s.repository.MarkInstallCommandTimedOut(ctx, req.TenantID, command.CommandID, waitErr.Error())
			}
			s.recordPartialFailure(ctx, req, InstallFailurePhaseTimeout, "runtime_install_timeout", waitErr.Error(), command.CommandID, appliedNodes)
			return nil, &InstallSkillError{Phase: InstallFailurePhaseTimeout, Message: waitErr.Error()}
		}
		if receipt.Status != "completed" {
			message := receipt.ErrorMessage
			if strings.TrimSpace(message) == "" {
				message = "runtime install failed"
			}
			s.recordPartialFailure(ctx, req, InstallFailurePhaseRuntimeInstall, "runtime_install_failed", message, command.CommandID, appliedNodes)
			return nil, &InstallSkillError{Phase: InstallFailurePhaseRuntimeInstall, Message: message}
		}
		installations = append(installations, command.Installations...)
		appliedNodes = append(appliedNodes, command.NodeID)
	}
	return installations, nil
}

func groupInstallTargetsByNode(targets []SkillInstallTarget) (map[string][]SkillInstallTarget, []string) {
	grouped := map[string][]SkillInstallTarget{}
	nodeOrder := make([]string, 0)
	for _, target := range targets {
		if _, seen := grouped[target.NodeID]; !seen {
			nodeOrder = append(nodeOrder, target.NodeID)
		}
		grouped[target.NodeID] = append(grouped[target.NodeID], target)
	}
	sort.Strings(nodeOrder)
	return grouped, nodeOrder
}

func (s *InstallService) dispatchInstallCommand(ctx context.Context, req InstallSkillRequest, skill *Skill, nodeID string, nodeTargets []SkillInstallTarget) (dispatchedInstallCommand, error) {
	commandID := newInstallCommandID()
	payload := buildInstallCommandPayload(commandID, req.TenantID, skill, nodeTargets)
	runtimeNodeID := nodeTargets[0].RuntimeNodeID
	if err := s.repository.CreateInstallCommandReceipt(ctx, CreateSkillInstallCommandReceiptRequest{
		TenantID:      req.TenantID,
		CommandID:     commandID,
		CommandType:   "install_skills",
		RuntimeNodeID: runtimeNodeID,
		NodeID:        nodeID,
		ResourceID:    req.SkillID,
		Payload:       payload,
	}); err != nil {
		return dispatchedInstallCommand{}, err
	}
	command, err := runtimeInstallCommand(commandID, payload)
	if err != nil {
		return dispatchedInstallCommand{}, err
	}
	if err := s.dispatcher.Dispatch(ctx, nodeID, command); err != nil {
		_ = s.repository.MarkInstallCommandFailed(ctx, req.TenantID, commandID, err.Error())
		return dispatchedInstallCommand{}, &dispatchInstallCommandError{CommandID: commandID, Err: err}
	}
	return dispatchedInstallCommand{
		CommandID:     commandID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        nodeID,
		Installations: expectedInstallations(req, skill, nodeTargets, commandID),
	}, nil
}

func buildInstallCommandPayload(commandID string, tenantID uuid.UUID, skill *Skill, targets []SkillInstallTarget) map[string]any {
	payloadTargets := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		payloadTargets = append(payloadTargets, map[string]any{
			"team_id":             target.TeamID.String(),
			"digital_employee_id": target.DigitalEmployeeID.String(),
			"provider_type":       target.ProviderType,
			"agent_home_dir":      target.AgentHomeDir,
		})
	}
	return map[string]any{
		"command_id": commandID,
		"tenant_id":  tenantID.String(),
		"skill": map[string]any{
			"skill_id":                skill.ID.String(),
			"skill_key":               skill.Slug,
			"archive_object_ref":      skill.ArchiveObjectRef,
			"archive_checksum_sha256": skill.ArchiveChecksum,
			"archive_size_bytes":      skill.ArchiveSizeBytes,
			"archive_file_count":      skill.ArchiveFileCount,
		},
		"targets":             payloadTargets,
		"rollback_on_failure": true,
	}
}

func expectedInstallations(req InstallSkillRequest, skill *Skill, targets []SkillInstallTarget, commandID string) []SkillInstallation {
	installations := make([]SkillInstallation, 0, len(targets))
	now := time.Now().UTC()
	for _, target := range targets {
		installations = append(installations, SkillInstallation{
			TenantID:              req.TenantID,
			SkillID:               skill.ID,
			TargetScope:           req.TargetScope,
			TeamID:                target.TeamID,
			DigitalEmployeeID:     target.DigitalEmployeeID,
			EmployeeName:          target.EmployeeName,
			RuntimeNodeID:         target.RuntimeNodeID,
			NodeID:                target.NodeID,
			ProviderType:          target.ProviderType,
			InstalledPath:         providerSkillPath(target.AgentHomeDir, target.ProviderType, skill.Slug),
			ArchiveChecksumSHA256: skill.ArchiveChecksum,
			InstalledBy:           req.ActorUserID,
			InstalledAt:           now,
			Metadata:              map[string]any{"command_id": commandID},
		})
	}
	return installations
}

func providerSkillPath(agentHomeDir, providerType, skillSlug string) string {
	base := strings.TrimRight(agentHomeDir, "/")
	switch providerType {
	case "opencode":
		return base + "/.opencode/skills/" + skillSlug
	case "codex":
		return base + "/.agents/skills/" + skillSlug
	case "claude-code":
		return base + "/.claude/skills/" + skillSlug
	default:
		return base + "/skills/" + skillSlug
	}
}

func timeoutOrDefault(requested, fallback time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return fallback
}

func (s *InstallService) recordPartialFailure(ctx context.Context, req InstallSkillRequest, phase InstallFailurePhase, reasonCode, message, commandID string, appliedNodes []string) {
	details := map[string]any{}
	if len(appliedNodes) > 0 {
		details["applied_nodes"] = appliedNodes
		details["requires_manual_cleanup"] = true
	}
	_ = s.repository.RecordInstallFailure(ctx, SkillInstallFailureLog{
		TenantID:          req.TenantID,
		SkillID:           req.SkillID,
		TargetScope:       req.TargetScope,
		TeamID:            req.TeamID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Phase:             phase,
		ReasonCode:        reasonCode,
		Message:           message,
		CommandID:         commandID,
		Details:           details,
	})
}

func newInstallCommandID() string {
	return "cmd-" + uuid.NewString()
}

func (s *InstallService) preflight(targetScope SkillInstallTargetScope, skill *Skill, targets []SkillInstallTarget) []SkillInstallBlockedTarget {
	if skill == nil || strings.TrimSpace(skill.ArchiveObjectRef) == "" || strings.TrimSpace(skill.ArchiveChecksum) == "" || skill.ArchiveSizeBytes <= 0 || skill.ArchiveFileCount <= 0 {
		return []SkillInstallBlockedTarget{{
			ReasonCode: "skill_archive_missing",
			Message:    "skill archive metadata is incomplete",
		}}
	}
	if len(targets) == 0 {
		return []SkillInstallBlockedTarget{{
			ReasonCode: "no_install_targets",
			Message:    "no digital employees matched the install target",
		}}
	}

	blockers := make([]SkillInstallBlockedTarget, 0)
	runtimeNodeIDs := make(map[uuid.UUID]struct{})
	for _, target := range targets {
		if target.RuntimeNodeID != uuid.Nil {
			runtimeNodeIDs[target.RuntimeNodeID] = struct{}{}
		}
		if target.RuntimeNodeID == uuid.Nil || strings.TrimSpace(target.NodeID) == "" {
			blockers = append(blockers, blockedTarget(target, "runtime_missing", "digital employee has no active Runtime"))
			continue
		}
		if strings.TrimSpace(target.AgentHomeDir) == "" {
			blockers = append(blockers, blockedTarget(target, "agent_home_dir_missing", "digital employee has no agent_home_dir"))
			continue
		}
		if _, ok := supportedInstallProviders[strings.TrimSpace(target.ProviderType)]; !ok {
			blockers = append(blockers, blockedTarget(target, "unsupported_provider", "provider_type must be one of opencode, codex, claude-code"))
			continue
		}
		if !s.dispatcher.IsConnected(target.NodeID) {
			blockers = append(blockers, blockedTarget(target, "runtime_not_connected", "绑定的 Runtime 节点已失活，请先重新 provision 数字员工"))
			continue
		}
	}
	if len(blockers) == 0 && targetScope == SkillInstallTargetTeam && len(runtimeNodeIDs) > 1 {
		for _, target := range targets {
			blockers = append(blockers, blockedTarget(target, "team_install_multiple_runtime_nodes", "team install requires all targets to share one connected Runtime node"))
		}
	}
	return blockers
}

func blockedTarget(target SkillInstallTarget, reasonCode, message string) SkillInstallBlockedTarget {
	return SkillInstallBlockedTarget{
		DigitalEmployeeID: target.DigitalEmployeeID,
		EmployeeName:      target.EmployeeName,
		ProviderType:      target.ProviderType,
		RuntimeNodeID:     target.RuntimeNodeID,
		NodeID:            target.NodeID,
		ReasonCode:        reasonCode,
		Message:           message,
	}
}

func (s *InstallService) recordFailure(ctx context.Context, req InstallSkillRequest, phase InstallFailurePhase, reasonCode, message, commandID string, blockers []SkillInstallBlockedTarget) error {
	details := map[string]any{"blocked_targets": blockers}
	return s.repository.RecordInstallFailure(ctx, SkillInstallFailureLog{
		TenantID:          req.TenantID,
		SkillID:           req.SkillID,
		TargetScope:       req.TargetScope,
		TeamID:            req.TeamID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Phase:             phase,
		ReasonCode:        reasonCode,
		Message:           message,
		CommandID:         commandID,
		Details:           details,
	})
}

func runtimeInstallCommand(id string, payload map[string]any) (cpruntime.RuntimeCommand, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return cpruntime.RuntimeCommand{}, fmt.Errorf("encode runtime install command payload: %w", err)
	}
	return cpruntime.RuntimeCommand{ID: id, Type: "install_skills", Payload: encoded}, nil
}
