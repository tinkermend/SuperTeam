package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage"
)

type Repository interface {
	ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error)
	GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error)
	UpsertSkillPackage(ctx context.Context, req UpsertSkillPackageRequest) (*Skill, error)
	DeleteSkill(ctx context.Context, req DeleteSkillRequest) error
	BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error)
	UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error
	ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error)
	BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error)
	UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error
	ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error)
	ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]SkillRuntimeRecord, error)
	IsSkillBoundToEmployeeTeam(ctx context.Context, req BindEmployeeSkillRequest) (bool, error)
}

type RequiredToolsRepository interface {
	ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error)
}

type SkillInstallationsRepository interface {
	ListSkillInstallations(ctx context.Context, req ListSkillInstallationsRequest) ([]SkillInstallation, error)
}

type ObjectStore interface {
	PutObject(ctx context.Context, key string, body io.Reader, options storage.PutObjectOptions) (storage.ObjectRef, error)
	DeleteObject(ctx context.Context, key string) error
}

type Installer interface {
	InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error)
}

type Service struct {
	repository  Repository
	objectStore ObjectStore
	installer   Installer
}

func NewService(repository Repository, objectStore ObjectStore) *Service {
	return &Service{repository: repository, objectStore: objectStore}
}

func (s *Service) SetInstallService(installer Installer) {
	s.installer = installer
}

func (s *Service) InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	if s == nil || s.installer == nil {
		return InstallSkillResult{}, fmt.Errorf("%w: skill install service is not configured", ErrInvalidInput)
	}
	return s.installer.InstallSkill(ctx, req)
}

func (s *Service) ListSkillInstallations(ctx context.Context, req ListSkillInstallationsRequest) ([]SkillInstallation, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.SkillID == uuid.Nil {
		return nil, fmt.Errorf("%w: skill_id is required", ErrInvalidInput)
	}
	repository, ok := s.repository.(SkillInstallationsRepository)
	if !ok {
		return nil, fmt.Errorf("%w: skill installation repository is not configured", ErrInvalidInput)
	}
	return repository.ListSkillInstallations(ctx, req)
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

	rootPrefix := commonRootPrefix(reader.File)
	skillMarkdownContent, fileCount, err := extractSkillMarkdown(reader, rootPrefix)
	if err != nil {
		return nil, err
	}
	if skillMarkdownContent == "" {
		return nil, fmt.Errorf("%w: zip archive must include SKILL.md", ErrInvalidInput)
	}

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
	slug := slugify(name)
	if slug == "" {
		slug = slugify(skillNameFromMarkdown(skillMarkdownContent))
	}
	if slug == "" {
		slug = slugify(strings.TrimSuffix(path.Base(req.Filename), path.Ext(req.Filename)))
	}
	if slug == "" {
		return nil, fmt.Errorf("%w: skill slug is required", ErrInvalidInput)
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
		Version:             "v0.1.0",
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
	return nil
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
		return nil, ErrTeamAlreadyInherited
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

func (s *Service) ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]SkillRuntimeRecord, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if digitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	return s.repository.ListSkillsForRuntime(ctx, tenantID, digitalEmployeeID)
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

func extractSkillMarkdown(reader *zip.Reader, rootPrefix string) (string, int, error) {
	var skillMarkdownContent string
	fileCount := 0
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		fileCount++
		normalizedPath := normalizeFilePath(strings.TrimPrefix(file.Name, rootPrefix))
		if normalizedPath == "SKILL.md" {
			rc, err := file.Open()
			if err != nil {
				return "", 0, fmt.Errorf("%w: cannot read SKILL.md", ErrInvalidInput)
			}
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(rc); err != nil {
				_ = rc.Close()
				return "", 0, fmt.Errorf("%w: cannot read SKILL.md", ErrInvalidInput)
			}
			_ = rc.Close()
			skillMarkdownContent = buf.String()
		}
	}
	return skillMarkdownContent, fileCount, nil
}

func commonRootPrefix(files []*zip.File) string {
	root := ""
	for _, file := range files {
		if file.FileInfo().IsDir() {
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

func skillNameFromMarkdown(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func firstParagraphFromMarkdown(content string) string {
	for _, line := range strings.Split(content, "\n") {
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
	return clean
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
	return SkillRuntimeDependencies{Tools: tools, Env: env}, nil
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
