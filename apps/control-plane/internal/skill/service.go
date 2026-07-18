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
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage"
	"gopkg.in/yaml.v3"
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
	DeleteSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) error
}

type RequiredToolsRepository interface {
	ListRequiredToolsForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]string, error)
}

type ObjectStore interface {
	PutObject(ctx context.Context, key string, body io.Reader, options storage.PutObjectOptions) (storage.ObjectRef, error)
	DeleteObject(ctx context.Context, key string) error
	// PresignGet 为 runtime 的 skill 归档直取签发短时 URL(证据地基 spec §8
	// 修订 1:runtime 零对象存储凭证);完整性由归档 sha256 复核保证。
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type Service struct {
	repository  Repository
	objectStore ObjectStore
}

func NewService(repository Repository, objectStore ObjectStore) *Service {
	return &Service{repository: repository, objectStore: objectStore}
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
		if _, err := s.BindSkillToTeam(ctx, BindTeamSkillRequest{TenantID: req.TenantID, TeamID: req.TeamID, SkillID: req.SkillID}); err != nil {
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
			if errors.Is(err, ErrTeamAlreadyInherited) {
				result.AlreadyBound = true
				return result, nil
			}
			return InstallSkillResult{}, err
		}
	default:
		return InstallSkillResult{}, fmt.Errorf("%w: target_scope must be team or employee", ErrInvalidInput)
	}
	return result, nil
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
	if err := s.repository.DeleteSkillMCPDependencies(ctx, req.TenantID, req.SkillID); err != nil {
		return fmt.Errorf("cleanup skill mcp dependencies: %w", err)
	}
	return nil
}

const archiveDownloadPresignTTL = 15 * time.Minute

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
	url, err := s.objectStore.PresignGet(ctx, key, archiveDownloadPresignTTL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign skill archive get: %w", err)
	}
	return url, time.Now().Add(archiveDownloadPresignTTL), nil
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
		rawPath := strings.TrimPrefix(file.Name, rootPrefix)
		if file.FileInfo().IsDir() || isIgnoredArchiveEntry(rawPath) {
			continue
		}
		fileCount++
		normalizedPath := normalizeFilePath(rawPath)
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
	normalized := normalizeFilePath(value)
	if normalized == "" {
		return true
	}
	parts := strings.Split(normalized, "/")
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
	Body                   string
}

// parseSkillMarkdown 剥离 SKILL.md 顶部的 YAML frontmatter 并提取 name/description。
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
		}
		if err := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "\n")), &meta); err == nil {
			doc.FrontmatterName = strings.TrimSpace(meta.Name)
			doc.FrontmatterDescription = strings.TrimSpace(meta.Description)
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
