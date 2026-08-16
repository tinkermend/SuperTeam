package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/apierror"
	"github.com/superteam/control-plane/internal/oplog"
	"github.com/superteam/control-plane/internal/storage"
	"github.com/superteam/control-plane/internal/systemconfig"
	"github.com/superteam/control-plane/internal/teamguard"
	"gopkg.in/yaml.v3"
)

type Repository interface {
	ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error)
	GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error)
	UpsertSkillPackage(ctx context.Context, req UpsertSkillPackageRequest) (*Skill, error)
	ReplaceSkillArchive(ctx context.Context, req ReplaceSkillArchiveRequest) (*ReplaceSkillArchiveResult, error)
	FindSkillBySlug(ctx context.Context, req FindSkillBySlugRequest) (*Skill, error)
	DeleteSkill(ctx context.Context, req DeleteSkillRequest) error
	BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error)
	UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error
	ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error)
	BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error)
	UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error
	ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error)
	ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, projectID *uuid.UUID) (RuntimeSkillsResult, error)
	ListProjectSkillBindings(ctx context.Context, req ListProjectSkillBindingsRequest) ([]ProjectSkillBinding, error)
	ReplaceProjectSkillBindings(ctx context.Context, req PutProjectSkillBindingsRequest) ([]ProjectSkillBinding, error)
	ListSkillIDsWithAnyProjectBinding(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	IsSkillBoundToEmployeeTeam(ctx context.Context, req BindEmployeeSkillRequest) (bool, error)
	DeleteSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) error
}

type RequiredToolsRepository interface {
	ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error)
}

type ObjectStore interface {
	PutObject(ctx context.Context, key string, body io.Reader, options storage.PutObjectOptions) (storage.ObjectRef, error)
	DeleteObject(ctx context.Context, key string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	// PresignGet 为 runtime 的 skill 归档直取签发短时 URL(证据地基 spec §8
	// 修订 1:runtime 零对象存储凭证);完整性由归档 sha256 复核保证。
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// CapabilityBindingEventRecorder records project capability binding changes (best-effort).
type CapabilityBindingEventRecorder interface {
	RecordProjectCapabilityBindingChanged(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, bindingKind string, skillIDs []uuid.UUID) error
}

type Service struct {
	repository                Repository
	objectStore               ObjectStore
	systemConfig              systemconfig.Reader
	skillMCPDependencyChecker SkillMCPDependencyChecker
	eventRecorder             CapabilityBindingEventRecorder
	oplogLogger               oplog.Logger
}

func NewService(repository Repository, objectStore ObjectStore) *Service {
	return &Service{repository: repository, objectStore: objectStore}
}

// SetSystemConfigReader 注入配置中心读取器;未注入(测试)时使用注册表默认值。
func (s *Service) SetSystemConfigReader(reader systemconfig.Reader) {
	s.systemConfig = reader
}

func (s *Service) SetOperationLogger(log oplog.Logger) {
	s.oplogLogger = log
}

func (s *Service) archivePresignTTL(ctx context.Context, tenantID uuid.UUID) time.Duration {
	if s.systemConfig == nil {
		return systemconfig.DefaultDurationFor(systemconfig.KeySkillArchivePresignTTL)
	}
	return s.systemConfig.Duration(ctx, tenantID, systemconfig.KeySkillArchivePresignTTL)
}

// InstallSkill loads a skill onto a team or an employee as a pure logical
// binding. Physical materialization is deferred to dispatch time, where the
// runtime converges the employee home directory against the resolved
// capability manifest; no runtime node participates in this call. Repeat
// installs (including employee scope already covered by team inheritance)
// are idempotent and reported via AlreadyBound.
func (s *Service) InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	if s == nil || s.repository == nil {
		return InstallSkillResult{}, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return InstallSkillResult{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return InstallSkillResult{}, fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	if _, err := s.repository.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID}); err != nil {
		return InstallSkillResult{}, err
	}
	result := InstallSkillResult{
		SkillID:     req.SkillID,
		TargetScope: req.TargetScope,
		BoundAt:     time.Now().UTC(),
	}
	switch req.TargetScope {
	case SkillInstallTargetTeam:
		if req.TeamID == uuid.Nil {
			return InstallSkillResult{}, fmt.Errorf("%w: team_id is required for team scope", ErrInvalidInput)
		}
		result.TeamID = req.TeamID
		teamSkills, err := s.ListTeamSkills(ctx, ListTeamSkillsRequest{TenantID: req.TenantID, TeamID: req.TeamID})
		if err != nil {
			return InstallSkillResult{}, err
		}
		if containsSkillID(teamSkills, req.SkillID) {
			result.AlreadyBound = true
			return result, nil
		}
		if _, err := s.BindSkillToTeam(ctx, BindTeamSkillRequest{TenantID: req.TenantID, TeamID: req.TeamID, SkillID: req.SkillID, ActorUserID: req.ActorUserID}); err != nil {
			s.recordSkillOperation(ctx, req, "skill.install", oplog.ResultFailed, map[string]any{"error": err.Error()})
			return InstallSkillResult{}, err
		}
	case SkillInstallTargetEmployee:
		if req.DigitalEmployeeID == uuid.Nil {
			return InstallSkillResult{}, fmt.Errorf("%w: digital_employee_id is required for employee scope", ErrInvalidInput)
		}
		result.DigitalEmployeeID = req.DigitalEmployeeID
		effective, err := s.ListEffectiveEmployeeSkills(ctx, ListEffectiveEmployeeSkillsRequest{TenantID: req.TenantID, DigitalEmployeeID: req.DigitalEmployeeID})
		if err != nil {
			return InstallSkillResult{}, err
		}
		for _, item := range effective {
			if item.Skill.ID == req.SkillID {
				result.AlreadyBound = true
				return result, nil
			}
		}
		if _, err := s.BindSkillToEmployee(ctx, BindEmployeeSkillRequest{TenantID: req.TenantID, DigitalEmployeeID: req.DigitalEmployeeID, SkillID: req.SkillID}); err != nil {
			s.recordSkillOperation(ctx, req, "skill.install", oplog.ResultFailed, map[string]any{"error": err.Error()})
			return InstallSkillResult{}, err
		}
	default:
		return InstallSkillResult{}, fmt.Errorf("%w: target_scope must be team or employee", ErrInvalidInput)
	}
	s.recordSkillOperation(ctx, req, "skill.install", oplog.ResultSucceeded, map[string]any{
		"target_scope": req.TargetScope,
	})
	return result, nil
}

func (s *Service) recordSkillOperation(ctx context.Context, req InstallSkillRequest, action, result string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	details["target_scope"] = req.TargetScope
	resourceType := "skill"
	resourceID := req.SkillID.String()
	if req.TargetScope == SkillInstallTargetTeam && req.TeamID != uuid.Nil {
		resourceType = "team"
		resourceID = req.TeamID.String()
		details["skill_id"] = req.SkillID.String()
	}
	if req.TargetScope == SkillInstallTargetEmployee && req.DigitalEmployeeID != uuid.Nil {
		resourceType = "digital_employee"
		resourceID = req.DigitalEmployeeID.String()
		details["skill_id"] = req.SkillID.String()
	}
	oplog.WriteBestEffort(ctx, s.oplogLogger, oplog.Record{
		TenantID:     req.TenantID,
		UserID:       req.ActorUserID,
		Module:       oplog.ModuleSkills,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Result:       result,
		Details:      details,
	})
}

func containsSkillID(skills []*Skill, skillID uuid.UUID) bool {
	for _, item := range skills {
		if item != nil && item.ID == skillID {
			return true
		}
	}
	return false
}

func (s *Service) ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	return s.repository.ListSkills(ctx, req)
}

func (s *Service) GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	return s.repository.GetSkill(ctx, req)
}

func (s *Service) UploadSkill(ctx context.Context, req UploadSkillRequest) (*Skill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if s.objectStore == nil {
		return nil, fmt.Errorf("%w: object store is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if len(req.Archive) == 0 {
		return nil, fmt.Errorf("%w: zip archive is required", ErrInvalidInput)
	}
	runtimeDependencies, err := normalizeRuntimeDependencies(req.RuntimeDependencies)
	if err != nil {
		return nil, err
	}

	reader, err := zip.NewReader(bytes.NewReader(req.Archive), int64(len(req.Archive)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid zip archive", ErrInvalidInput)
	}
	inspected, err := inspectSkillArchive(reader)
	if err != nil {
		return nil, err
	}
	if inspected.Markdown == "" {
		return nil, fmt.Errorf("%w: zip archive must include SKILL.md", ErrInvalidInput)
	}
	skillMarkdownContent := inspected.Markdown
	fileCount := inspected.FileCount

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = skillNameFromMarkdown(skillMarkdownContent)
	}
	if name == "" {
		name = strings.TrimSuffix(path.Base(req.Filename), path.Ext(req.Filename))
	}
	if name == "" {
		return nil, fmt.Errorf("%w: skill name is required", ErrInvalidInput)
	}

	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = firstParagraphFromMarkdown(skillMarkdownContent)
	}
	slug := deriveSkillSlug(req.Slug, skillNameFromMarkdown(skillMarkdownContent), req.Filename)
	if slug == "" {
		return nil, fmt.Errorf("%w: skill slug is required (set slug, or use an ASCII SKILL.md name / zip filename)", ErrInvalidInput)
	}
	doc := parseSkillMarkdown(skillMarkdownContent)
	version, err := resolveSkillVersion(req.Version, doc.FrontmatterVersion, "v0.1.0")
	if err != nil {
		return nil, err
	}

	existing, err := s.repository.FindSkillBySlug(ctx, FindSkillBySlugRequest{TenantID: req.TenantID, Slug: slug})
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, &SlugConflictError{SkillID: existing.ID, Slug: existing.Slug, Name: existing.Name}
	}

	sum := sha256.Sum256(req.Archive)
	checksum := hex.EncodeToString(sum[:])
	sizeBytes := int64(len(req.Archive))

	objectKey := fmt.Sprintf("skills/%s/%s/%s.zip", req.TenantID, slug, checksum)
	ref, err := s.objectStore.PutObject(ctx, objectKey, bytes.NewReader(req.Archive), storage.PutObjectOptions{
		ContentType: "application/zip",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to store skill archive: %v", ErrInvalidInput, err)
	}

	skill, err := s.repository.UpsertSkillPackage(ctx, UpsertSkillPackageRequest{
		TenantID:            req.TenantID,
		ActorUserID:         req.ActorUserID,
		Slug:                slug,
		Name:                name,
		Description:         description,
		Version:             version,
		Source:              "upload",
		RiskLevel:           riskLevelOrDefault(req.RiskLevel),
		IconKey:             iconKeyForSkill(slug),
		ColorToken:          colorTokenForSkill(slug),
		Tags:                normalizeStringList(req.Tags),
		TeamIDs:             req.TeamIDs,
		RuntimeDependencies: runtimeDependencies,
		ArchiveObjectRef:    ref.URI,
		ArchiveFilename:     req.Filename,
		ArchiveSizeBytes:    sizeBytes,
		ArchiveChecksum:     checksum,
		ArchiveFileCount:    fileCount,
	})
	if err != nil {
		_ = s.objectStore.DeleteObject(ctx, objectKey)
		return nil, err
	}
	return skill, nil
}

func (s *Service) ReplaceSkillArchive(ctx context.Context, req ReplaceSkillRequest) (*Skill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if s.objectStore == nil {
		return nil, fmt.Errorf("%w: object store is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	if len(req.Archive) == 0 {
		return nil, fmt.Errorf("%w: zip archive is required", ErrInvalidInput)
	}
	current, err := s.repository.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
	if err != nil {
		return nil, err
	}
	runtimeDependencies := current.RuntimeDependencies
	if req.RuntimeDependencies != nil {
		normalized, err := normalizeRuntimeDependencies(*req.RuntimeDependencies)
		if err != nil {
			return nil, err
		}
		runtimeDependencies = normalized
	}
	reader, err := zip.NewReader(bytes.NewReader(req.Archive), int64(len(req.Archive)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid zip archive", ErrInvalidInput)
	}
	inspected, err := inspectSkillArchive(reader)
	if err != nil {
		return nil, err
	}
	if inspected.Markdown == "" {
		return nil, fmt.Errorf("%w: zip archive must include SKILL.md", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = skillNameFromMarkdown(inspected.Markdown)
	}
	if name == "" {
		name = current.Name
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = firstParagraphFromMarkdown(inspected.Markdown)
	}
	if description == "" {
		description = current.Description
	}
	derivedSlug := deriveSkillSlug("", skillNameFromMarkdown(inspected.Markdown), req.Filename)
	if derivedSlug != "" && derivedSlug != current.Slug {
		return nil, apierror.New("skill_slug_immutable", http.StatusBadRequest, "更新不得改 slug；若这是另一个技能请走新建")
	}
	doc := parseSkillMarkdown(inspected.Markdown)
	version, err := resolveSkillVersion(req.Version, doc.FrontmatterVersion, current.Version)
	if err != nil {
		return nil, err
	}
	riskLevel := current.RiskLevel
	if req.RiskLevel != nil {
		riskLevel = riskLevelOrDefault(*req.RiskLevel)
	}
	tags := current.Tags
	if req.Tags != nil {
		tags = normalizeStringList(*req.Tags)
	}
	sum := sha256.Sum256(req.Archive)
	checksum := hex.EncodeToString(sum[:])
	objectKey := fmt.Sprintf("skills/%s/%s/%s.zip", req.TenantID, current.Slug, checksum)
	ref, err := s.objectStore.PutObject(ctx, objectKey, bytes.NewReader(req.Archive), storage.PutObjectOptions{
		ContentType: "application/zip",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to store skill archive: %v", ErrInvalidInput, err)
	}
	result, err := s.repository.ReplaceSkillArchive(ctx, ReplaceSkillArchiveRequest{
		TenantID:            req.TenantID,
		ActorUserID:         req.ActorUserID,
		SkillID:             req.SkillID,
		Name:                name,
		Description:         description,
		Version:             version,
		RiskLevel:           &riskLevel,
		Tags:                &tags,
		RuntimeDependencies: &runtimeDependencies,
		ArchiveObjectRef:    ref.URI,
		ArchiveFilename:     req.Filename,
		ArchiveSizeBytes:    int64(len(req.Archive)),
		ArchiveChecksum:     checksum,
		ArchiveFileCount:    inspected.FileCount,
	})
	if err != nil {
		_ = s.objectStore.DeleteObject(ctx, objectKey)
		return nil, err
	}
	skill := result.Skill
	s.recordArchiveReplace(ctx, req.TenantID, req.ActorUserID, skill, result.OldVersion, result.OldChecksum)
	return skill, nil
}

func (s *Service) recordArchiveReplace(ctx context.Context, tenantID, actor uuid.UUID, skill *Skill, oldVersion, oldChecksum string) {
	if skill == nil {
		return
	}
	details := map[string]any{
		"slug":                   skill.Slug,
		"name":                   skill.Name,
		"old_version":            oldVersion,
		"new_version":            skill.Version,
		"old_checksum":           oldChecksum,
		"new_checksum":           skill.ArchiveChecksum,
		"archive_filename":       skill.ArchiveFilename,
		"archive_size_bytes":     skill.ArchiveSizeBytes,
		"binding_team_count":     len(skill.TeamBindings),
		"binding_employee_count": len(skill.AgentBindings),
		"binding_project_count":  len(skill.ProjectBindings),
	}
	oplog.WriteBestEffort(ctx, s.oplogLogger, oplog.Record{
		TenantID:     tenantID,
		UserID:       actor,
		Module:       oplog.ModuleSkills,
		ResourceType: "skill",
		ResourceID:   skill.ID.String(),
		Action:       "skill.archive.replace",
		Result:       oplog.ResultSucceeded,
		Details:      details,
	})
}

func (s *Service) ListSkillArchiveEntries(ctx context.Context, req GetSkillRequest) ([]ArchiveEntry, error) {
	inspected, err := s.openSkillArchiveForPreview(ctx, req)
	if err != nil {
		return nil, err
	}
	return inspected.Entries, nil
}

func (s *Service) GetSkillArchiveContent(ctx context.Context, req GetSkillRequest, entryPath string) (map[string]any, error) {
	inspected, err := s.openSkillArchiveForPreview(ctx, req)
	if err != nil {
		return nil, err
	}
	normalized := normalizeFilePath(entryPath)
	var entry *ArchiveEntry
	for i := range inspected.Entries {
		if inspected.Entries[i].Path == normalized {
			entry = &inspected.Entries[i]
			break
		}
	}
	if entry == nil || entry.Kind != "file" {
		return nil, apierror.New("skill_archive_path_not_found", http.StatusNotFound, "技能包中没有该文件")
	}
	file := inspected.filesByPath[normalized]
	if file == nil {
		return nil, apierror.New("skill_archive_path_not_found", http.StatusNotFound, "技能包中没有该文件")
	}
	if !entry.Previewable {
		return nil, apierror.New("skill_archive_not_previewable", http.StatusUnsupportedMediaType, "该文件不是文本，无法在线预览")
	}
	raw, err := readZipFile(file)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read archive file", ErrInvalidInput)
	}
	maxBytes := s.previewMaxBytes(ctx, req.TenantID)
	content, truncated := truncatePreview(raw, maxBytes)
	return map[string]any{
		"path":         normalized,
		"content":      content,
		"truncated":    truncated,
		"size_bytes":   entry.SizeBytes,
		"content_type": entry.ContentType,
	}, nil
}

func (s *Service) openSkillArchiveForPreview(ctx context.Context, req GetSkillRequest) (*archiveInspection, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if s.objectStore == nil {
		return nil, fmt.Errorf("%w: object store is not configured", ErrInvalidInput)
	}
	skill, err := s.repository.GetSkill(ctx, req)
	if err != nil {
		return nil, err
	}
	maxArchive := s.previewMaxArchiveBytes(ctx, req.TenantID)
	if skill.ArchiveSizeBytes > maxArchive {
		return nil, apierror.New("skill_archive_preview_too_large", http.StatusRequestEntityTooLarge, "技能包过大，平台不提供在线预览，请在本地解压查看")
	}
	archiveBytes, err := s.readArchiveObject(ctx, req.TenantID, skill.ArchiveObjectRef)
	if err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid zip archive", ErrInvalidInput)
	}
	maxFiles := s.unpackMaxFileCount(ctx, req.TenantID)
	if int64(len(reader.File)) > maxFiles {
		return nil, fmt.Errorf("%w: zip archive exceeds file count limit", ErrInvalidInput)
	}
	return inspectSkillArchive(reader)
}

func (s *Service) readArchiveObject(ctx context.Context, tenantID uuid.UUID, archiveObjectRef string) ([]byte, error) {
	ref := strings.TrimSpace(archiveObjectRef)
	if ref == "" {
		return nil, fmt.Errorf("%w: archive_object_ref is required", ErrInvalidInput)
	}
	key := ref
	if strings.HasPrefix(ref, "s3://") {
		key = extractObjectKeyFromURI(ref)
	}
	expectedPrefix := fmt.Sprintf("skills/%s/", tenantID)
	if !strings.HasPrefix(key, expectedPrefix) {
		return nil, fmt.Errorf("%w: archive_object_ref is outside the tenant's skills prefix", ErrInvalidInput)
	}
	body, err := s.objectStore.GetObject(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get skill archive: %w", err)
	}
	defer body.Close()
	return io.ReadAll(body)
}

func (s *Service) previewMaxBytes(ctx context.Context, tenantID uuid.UUID) int64 {
	if s.systemConfig == nil {
		return systemconfig.DefaultFor(systemconfig.KeySkillArchivePreviewMaxBytes)
	}
	return s.systemConfig.Int64(ctx, tenantID, systemconfig.KeySkillArchivePreviewMaxBytes)
}

func (s *Service) previewMaxArchiveBytes(ctx context.Context, tenantID uuid.UUID) int64 {
	if s.systemConfig == nil {
		return systemconfig.DefaultFor(systemconfig.KeySkillArchivePreviewMaxArchiveBytes)
	}
	return s.systemConfig.Int64(ctx, tenantID, systemconfig.KeySkillArchivePreviewMaxArchiveBytes)
}

func (s *Service) unpackMaxFileCount(ctx context.Context, tenantID uuid.UUID) int64 {
	if s.systemConfig == nil {
		return systemconfig.DefaultFor(systemconfig.KeySkillArchiveUnpackMaxFileCount)
	}
	return s.systemConfig.Int64(ctx, tenantID, systemconfig.KeySkillArchiveUnpackMaxFileCount)
}

func (s *Service) DeleteSkill(ctx context.Context, req DeleteSkillRequest) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	skill, err := s.repository.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
	if err != nil {
		return err
	}
	if err := s.repository.DeleteSkill(ctx, req); err != nil {
		return err
	}
	if s.objectStore != nil && skill.ArchiveObjectRef != "" {
		objectKey := extractObjectKeyFromURI(skill.ArchiveObjectRef)
		if objectKey != "" {
			_ = s.objectStore.DeleteObject(ctx, objectKey)
		}
	}
	if err := s.repository.DeleteSkillMCPDependencies(ctx, req.TenantID, req.SkillID); err != nil {
		return fmt.Errorf("cleanup skill mcp dependencies: %w", err)
	}
	return nil
}

// PresignArchiveDownload 为 runtime 即将物化的 skill 归档签发短时 GET URL。
// key 必须落在调用方租户的 skills/ 前缀内——这是跨租户读取的唯一闸门;
// runtime 侧随后按 archive_checksum_sha256 复核字节完整性。
func (s *Service) PresignArchiveDownload(ctx context.Context, tenantID uuid.UUID, archiveObjectRef string) (string, time.Time, error) {
	if s == nil || s.objectStore == nil {
		return "", time.Time{}, fmt.Errorf("%w: skill object store is not configured", ErrInvalidInput)
	}
	if tenantID == uuid.Nil {
		return "", time.Time{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	ref := strings.TrimSpace(archiveObjectRef)
	if ref == "" {
		return "", time.Time{}, fmt.Errorf("%w: archive_object_ref is required", ErrInvalidInput)
	}
	key := ref
	if strings.HasPrefix(ref, "s3://") {
		key = extractObjectKeyFromURI(ref)
	}
	expectedPrefix := fmt.Sprintf("skills/%s/", tenantID)
	if !strings.HasPrefix(key, expectedPrefix) {
		return "", time.Time{}, fmt.Errorf("%w: archive_object_ref is outside the tenant's skills prefix", ErrInvalidInput)
	}
	presignTTL := s.archivePresignTTL(ctx, tenantID)
	url, err := s.objectStore.PresignGet(ctx, key, presignTTL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign skill archive get: %w", err)
	}
	return url, time.Now().Add(presignTTL), nil
}

func extractObjectKeyFromURI(uri string) string {
	stripped := uri
	if prefix, found := strings.CutPrefix(uri, "s3://"); found {
		stripped = prefix
	}
	if idx := strings.Index(stripped, "/"); idx >= 0 {
		return stripped[idx+1:]
	}
	return stripped
}

func (s *Service) BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	return s.repository.BindSkillToTeam(ctx, req)
}

func (s *Service) UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	return s.repository.UnbindSkillFromTeam(ctx, req)
}

func (s *Service) ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	return s.repository.ListTeamSkills(ctx, req)
}

func (s *Service) BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	inherited, err := s.repository.IsSkillBoundToEmployeeTeam(ctx, req)
	if err != nil {
		return nil, err
	}
	if inherited {
		// 显式冲突：技能已由团队提供。此前调用方（InstallSkill）把它吞成
		// AlreadyBound=true 静默"成功"，界面表现得像装上了、实际什么也没发生。
		// 双 %w：既保留 ErrTeamAlreadyInherited 供既有判别，又让 apierror.Write
		// 认出 coded error 输出 {code, message}。
		name := ""
		if item, getErr := s.repository.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID}); getErr == nil && item != nil {
			name = item.Name
		}
		return nil, fmt.Errorf("%w: %w", ErrTeamAlreadyInherited, teamguard.CapabilityProvidedByTeamError("技能", name))
	}
	return s.repository.BindSkillToEmployee(ctx, req)
}

func (s *Service) UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	return s.repository.UnbindSkillFromEmployee(ctx, req)
}

func (s *Service) ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	return s.repository.ListEffectiveEmployeeSkills(ctx, req)
}

func (s *Service) ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, projectID *uuid.UUID) (RuntimeSkillsResult, error) {
	if s == nil || s.repository == nil {
		return RuntimeSkillsResult{}, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if tenantID == uuid.Nil {
		return RuntimeSkillsResult{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if digitalEmployeeID == uuid.Nil {
		return RuntimeSkillsResult{}, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	return s.repository.ListSkillsForRuntime(ctx, tenantID, digitalEmployeeID, projectID)
}

func (s *Service) ListProjectSkillBindings(ctx context.Context, req ListProjectSkillBindingsRequest) ([]ProjectSkillBinding, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and project_id are required", ErrInvalidInput)
	}
	return s.repository.ListProjectSkillBindings(ctx, req)
}

// PutProjectSkillBindings validates each skill (exists, artifact present, MCP deps
// registered) then declaratively replaces the project's skill bindings.
func (s *Service) PutProjectSkillBindings(ctx context.Context, req PutProjectSkillBindingsRequest) ([]ProjectSkillBinding, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil || req.UserID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, user_id and project_id are required", ErrInvalidInput)
	}
	seen := map[uuid.UUID]struct{}{}
	for _, item := range req.Items {
		if item.SkillID == uuid.Nil {
			return nil, fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
		}
		if _, dup := seen[item.SkillID]; dup {
			return nil, fmt.Errorf("%w: duplicate skill_id %s", ErrInvalidInput, item.SkillID)
		}
		seen[item.SkillID] = struct{}{}
		skill, err := s.repository.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: item.SkillID})
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("%w: skill %s not found", ErrInvalidInput, item.SkillID)
			}
			return nil, err
		}
		if strings.TrimSpace(skill.ArchiveObjectRef) == "" {
			return nil, fmt.Errorf("%w: skill %s has no installable archive", ErrInvalidInput, item.SkillID)
		}
	}
	// Dependency closure validation: every declared MCP dependency must exist.
	// Missing deps are named in the error (GATE S2).
	if err := s.validateSkillMCPDependenciesExist(ctx, req.TenantID, req.Items); err != nil {
		return nil, err
	}
	items, err := s.repository.ReplaceProjectSkillBindings(ctx, req)
	if err != nil {
		return nil, err
	}
	if s.eventRecorder != nil {
		ids := make([]uuid.UUID, 0, len(req.Items))
		for _, item := range req.Items {
			ids = append(ids, item.SkillID)
		}
		_ = s.eventRecorder.RecordProjectCapabilityBindingChanged(ctx, req.TenantID, req.ProjectID, req.UserID, "skill", ids)
	}
	return items, nil
}

// SkillMCPDependencyChecker is optionally wired for bind-time MCP dependency validation.
type SkillMCPDependencyChecker interface {
	ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDepRef, error)
}

// SkillMCPDepRef is a minimal dependency row used at bind-time validation.
type SkillMCPDepRef struct {
	SkillID     uuid.UUID
	MCPServerID uuid.UUID
	ServerKey   string
	ServerName  string
	Missing     bool // true when MCP definition is absent/deleted
}

func (s *Service) SetSkillMCPDependencyChecker(checker SkillMCPDependencyChecker) {
	s.skillMCPDependencyChecker = checker
}

func (s *Service) SetCapabilityBindingEventRecorder(r CapabilityBindingEventRecorder) {
	s.eventRecorder = r
}

func (s *Service) validateSkillMCPDependenciesExist(ctx context.Context, tenantID uuid.UUID, items []ProjectSkillBindingInput) error {
	if s.skillMCPDependencyChecker == nil || len(items) == 0 {
		return nil
	}
	skillIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		skillIDs = append(skillIDs, item.SkillID)
	}
	deps, err := s.skillMCPDependencyChecker.ListSkillMCPDependenciesForSkills(ctx, tenantID, skillIDs)
	if err != nil {
		return err
	}
	missing := make([]string, 0)
	seenMiss := map[string]struct{}{}
	for _, dep := range deps {
		if !dep.Missing {
			continue
		}
		label := strings.TrimSpace(dep.ServerKey)
		if label == "" {
			label = dep.MCPServerID.String()
		}
		key := dep.SkillID.String() + ":" + label
		if _, ok := seenMiss[key]; ok {
			continue
		}
		seenMiss[key] = struct{}{}
		missing = append(missing, fmt.Sprintf("skill %s missing MCP dependency %s", dep.SkillID, label))
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("%w: %s", ErrInvalidInput, strings.Join(missing, "; "))
}

func (s *Service) ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(nodeID) == "" {
		return nil, fmt.Errorf("%w: node_id is required", ErrInvalidInput)
	}
	repository, ok := s.repository.(RequiredToolsRepository)
	if !ok {
		return nil, fmt.Errorf("%w: required tools repository is not configured", ErrInvalidInput)
	}
	return repository.ListRequiredToolsForNode(ctx, tenantID, nodeID)
}

func commonRootPrefix(files []*zip.File) string {
	root := ""
	for _, file := range files {
		if file.FileInfo().IsDir() || isIgnoredArchiveEntry(file.Name) {
			continue
		}
		parts := strings.Split(strings.Trim(file.Name, "/"), "/")
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
			continue
		}
		if parts[0] != root {
			return ""
		}
	}
	if root == "" {
		return ""
	}
	return root + "/"
}

func isIgnoredArchiveEntry(value string) bool {
	cleaned := path.Clean(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	for _, part := range parts {
		if part == "__MACOSX" || part == ".DS_Store" || strings.HasPrefix(part, "._") {
			return true
		}
	}
	return false
}

type skillMarkdownDoc struct {
	FrontmatterName        string
	FrontmatterDescription string
	FrontmatterVersion     string
	Body                   string
}

// parseSkillMarkdown 剥离 SKILL.md 顶部的 YAML frontmatter 并提取 name/description/version。
// frontmatter 解析失败时仍剥离该块,保证兜底启发式不会把 `---` 当正文。
func parseSkillMarkdown(content string) skillMarkdownDoc {
	doc := skillMarkdownDoc{Body: content}
	lines := strings.Split(strings.TrimPrefix(content, "\ufeff"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return doc
	}
	for i := 1; i < len(lines); i++ {
		marker := strings.TrimSpace(lines[i])
		if marker != "---" && marker != "..." {
			continue
		}
		doc.Body = strings.Join(lines[i+1:], "\n")
		var meta struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Version     string `yaml:"version"`
		}
		if err := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "\n")), &meta); err == nil {
			doc.FrontmatterName = strings.TrimSpace(meta.Name)
			doc.FrontmatterDescription = strings.TrimSpace(meta.Description)
			doc.FrontmatterVersion = strings.TrimSpace(meta.Version)
		}
		return doc
	}
	return doc
}

func skillNameFromMarkdown(content string) string {
	doc := parseSkillMarkdown(content)
	if doc.FrontmatterName != "" {
		return doc.FrontmatterName
	}
	for _, line := range strings.Split(doc.Body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func firstParagraphFromMarkdown(content string) string {
	doc := parseSkillMarkdown(content)
	if doc.FrontmatterDescription != "" {
		return doc.FrontmatterDescription
	}
	for _, line := range strings.Split(doc.Body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

func normalizeFilePath(value string) string {
	clean := path.Clean(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
	if clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return ""
	}
	if isWindowsDrivePath(clean) {
		return ""
	}
	return clean
}

func isWindowsDrivePath(value string) bool {
	if len(value) >= 2 && value[1] == ':' {
		c := value[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return false
}

func normalizeStringList(values []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

var (
	skillToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	skillEnvNamePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func normalizeRuntimeDependencies(input SkillRuntimeDependencies) (SkillRuntimeDependencies, error) {
	tools, err := normalizeDependencyList(input.Tools, skillToolNamePattern, "tool")
	if err != nil {
		return SkillRuntimeDependencies{}, err
	}
	env, err := normalizeDependencyList(input.Env, skillEnvNamePattern, "env")
	if err != nil {
		return SkillRuntimeDependencies{}, err
	}
	mcpServers := input.MCPServers
	if mcpServers == nil {
		mcpServers = []SkillRuntimeMCPServerRef{}
	}
	return SkillRuntimeDependencies{Tools: tools, Env: env, MCPServers: mcpServers}, nil
}

func normalizeDependencyList(values []string, pattern *regexp.Regexp, label string) ([]string, error) {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !pattern.MatchString(value) {
			return nil, fmt.Errorf("%w: invalid runtime dependency %s %q", ErrInvalidInput, label, value)
		}
		seen[value] = struct{}{}
	}
	normalized := make([]string, 0, len(seen))
	for value := range seen {
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized, nil
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

// deriveSkillSlug picks the durable identity. Display names (often Chinese)
// must not participate: "ECC 编码规范" would otherwise collapse to "ecc".
func deriveSkillSlug(explicit, markdownName, filename string) string {
	if slug := slugify(explicit); slug != "" {
		return slug
	}
	if slug := slugify(markdownName); slug != "" {
		return slug
	}
	return slugify(strings.TrimSuffix(path.Base(filename), path.Ext(filename)))
}

func riskLevelOrDefault(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "medium"
	}
	return value
}

func iconKeyForSkill(slug string) string {
	switch {
	case strings.Contains(slug, "diagnose"):
		return "stethoscope"
	case strings.Contains(slug, "test") || strings.Contains(slug, "tdd"):
		return "flask"
	case strings.Contains(slug, "review"):
		return "shield-check"
	case strings.Contains(slug, "runtime"):
		return "server-cog"
	default:
		return "blocks"
	}
}

func colorTokenForSkill(slug string) string {
	switch {
	case strings.Contains(slug, "diagnose"):
		return "cyan"
	case strings.Contains(slug, "test") || strings.Contains(slug, "tdd"):
		return "emerald"
	case strings.Contains(slug, "review"):
		return "violet"
	case strings.Contains(slug, "runtime"):
		return "blue"
	default:
		return "teal"
	}
}
