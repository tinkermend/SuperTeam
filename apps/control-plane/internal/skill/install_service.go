package skill

import (
	"context"
	"encoding/json"
	"fmt"
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

	blockers := s.preflight(skill, targets)
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

	return result, fmt.Errorf("%w: runtime install dispatch not implemented", ErrInvalidInput)
}

func (s *InstallService) preflight(skill *Skill, targets []SkillInstallTarget) []SkillInstallBlockedTarget {
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
	for _, target := range targets {
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
			blockers = append(blockers, blockedTarget(target, "runtime_not_connected", "Runtime is not connected"))
			continue
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
