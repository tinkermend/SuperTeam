package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

type Repository interface {
	ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error)
	GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error)
	UpsertSkillPackage(ctx context.Context, req UpsertSkillPackageRequest) (*Skill, error)
	UpdateSkillFile(ctx context.Context, req UpdateSkillFileRequest) (*SkillFile, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
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
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if len(req.Archive) == 0 {
		return nil, fmt.Errorf("%w: zip archive is required", ErrInvalidInput)
	}

	files, err := filesFromZip(req.Archive)
	if err != nil {
		return nil, err
	}
	if !hasSkillMarkdown(files) {
		return nil, fmt.Errorf("%w: zip archive must include SKILL.md", ErrInvalidInput)
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = skillNameFromMarkdown(files)
	}
	if name == "" {
		name = strings.TrimSuffix(path.Base(req.Filename), path.Ext(req.Filename))
	}
	if name == "" {
		return nil, fmt.Errorf("%w: skill name is required", ErrInvalidInput)
	}

	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = firstParagraphFromMarkdown(files)
	}
	slug := slugify(name)
	if slug == "" {
		return nil, fmt.Errorf("%w: skill slug is required", ErrInvalidInput)
	}
	return s.repository.UpsertSkillPackage(ctx, UpsertSkillPackageRequest{
		TenantID:    req.TenantID,
		ActorUserID: req.ActorUserID,
		Slug:        slug,
		Name:        name,
		Description: description,
		Version:     "v0.1.0",
		Source:      "upload",
		RiskLevel:   riskLevelOrDefault(req.RiskLevel),
		Status:      SkillStatusInstalled,
		IconKey:     iconKeyForSkill(slug),
		ColorToken:  colorTokenForSkill(slug),
		Tags:        normalizeStringList(req.Tags),
		TeamIDs:     req.TeamIDs,
		Files:       files,
	})
}

func (s *Service) UpdateSkillFile(ctx context.Context, req UpdateSkillFileRequest) (*SkillFile, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("%w: skill repository is not configured", ErrInvalidInput)
	}
	req.Path = normalizeFilePath(req.Path)
	if req.Path == "" {
		return nil, fmt.Errorf("%w: file path is required", ErrInvalidInput)
	}
	return s.repository.UpdateSkillFile(ctx, req)
}

func filesFromZip(archive []byte) ([]*SkillFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid zip archive", ErrInvalidInput)
	}
	rootPrefix := commonRootPrefix(reader.File)
	files := make([]*SkillFile, 0, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		filePath := normalizeFilePath(strings.TrimPrefix(file.Name, rootPrefix))
		if filePath == "" || strings.HasPrefix(filePath, "../") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: cannot read %s", ErrInvalidInput, file.Name)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("%w: cannot read %s", ErrInvalidInput, file.Name)
		}
		_ = rc.Close()
		sum := sha256.Sum256(buf.Bytes())
		files = append(files, &SkillFile{
			Path:           filePath,
			FileType:       SkillFileTypeFile,
			Content:        buf.String(),
			SizeBytes:      int64(buf.Len()),
			ChecksumSHA256: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path == "SKILL.md" {
			return true
		}
		if files[j].Path == "SKILL.md" {
			return false
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func commonRootPrefix(files []*zip.File) string {
	root := ""
	for _, file := range files {
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

func hasSkillMarkdown(files []*SkillFile) bool {
	for _, file := range files {
		if file.Path == "SKILL.md" {
			return true
		}
	}
	return false
}

func skillNameFromMarkdown(files []*SkillFile) string {
	for _, file := range files {
		if file.Path != "SKILL.md" {
			continue
		}
		for _, line := range strings.Split(file.Content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "# "))
			}
		}
	}
	return ""
}

func firstParagraphFromMarkdown(files []*SkillFile) string {
	for _, file := range files {
		if file.Path != "SKILL.md" {
			continue
		}
		for _, line := range strings.Split(file.Content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			return line
		}
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
