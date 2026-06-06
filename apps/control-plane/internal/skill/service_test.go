package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestServiceUploadSkillParsesRootedZipAndKeepsUploadMetadata(t *testing.T) {
	repo := &serviceTestRepository{}
	service := NewService(repo)
	tenantID := uuid.New()
	teamID := uuid.New()

	archive := buildSkillZip(t, map[string]string{
		"diagnose/SKILL.md":             "# diagnose\n\n用于失败任务诊断。",
		"diagnose/scripts/reproduce.sh": "#!/usr/bin/env bash\nset -euo pipefail\n",
	})

	uploaded, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID:    tenantID,
		Name:        "diagnose",
		Description: "系统化诊断流程",
		Tags:        []string{"诊断", "测试"},
		TeamIDs:     []uuid.UUID{teamID},
		Archive:     archive,
		Filename:    "diagnose.zip",
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}

	if repo.upsertReq.TenantID != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, repo.upsertReq.TenantID)
	}
	if repo.upsertReq.Name != "diagnose" || repo.upsertReq.Slug != "diagnose" {
		t.Fatalf("expected uploaded skill name/slug diagnose, got %#v", repo.upsertReq)
	}
	if repo.upsertReq.Description != "系统化诊断流程" {
		t.Fatalf("expected upload description to win, got %q", repo.upsertReq.Description)
	}
	if !stringSlicesEqual(repo.upsertReq.Tags, []string{"诊断", "测试"}) {
		t.Fatalf("expected upload tags, got %#v", repo.upsertReq.Tags)
	}
	if len(repo.upsertReq.TeamIDs) != 1 || repo.upsertReq.TeamIDs[0] != teamID {
		t.Fatalf("expected team binding %s, got %#v", teamID, repo.upsertReq.TeamIDs)
	}
	if len(repo.upsertReq.Files) != 2 {
		t.Fatalf("expected 2 files, got %#v", repo.upsertReq.Files)
	}
	if repo.upsertReq.Files[0].Path != "SKILL.md" || repo.upsertReq.Files[1].Path != "scripts/reproduce.sh" {
		t.Fatalf("expected normalized file paths, got %#v", repo.upsertReq.Files)
	}
	if uploaded.Files[0].Content != "# diagnose\n\n用于失败任务诊断。" {
		t.Fatalf("expected SKILL.md content in uploaded skill, got %#v", uploaded.Files[0])
	}
}

func TestServiceUploadSkillParsesSkillMarkdownOnlyZip(t *testing.T) {
	repo := &serviceTestRepository{}
	service := NewService(repo)
	tenantID := uuid.New()

	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "# Release Review\n\n检查发布计划、回滚策略和验收证据。",
	})

	uploaded, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: tenantID,
		Tags:     []string{"发布", "验收"},
		Archive:  archive,
		Filename: "release-review.zip",
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}

	if repo.upsertReq.Name != "Release Review" || repo.upsertReq.Slug != "release-review" {
		t.Fatalf("expected metadata derived from SKILL.md, got %#v", repo.upsertReq)
	}
	if repo.upsertReq.Description != "检查发布计划、回滚策略和验收证据。" {
		t.Fatalf("expected description derived from SKILL.md, got %q", repo.upsertReq.Description)
	}
	if len(repo.upsertReq.Files) != 1 || repo.upsertReq.Files[0].Path != "SKILL.md" {
		t.Fatalf("expected only SKILL.md file, got %#v", repo.upsertReq.Files)
	}
	if uploaded.Files[0].Path != "SKILL.md" {
		t.Fatalf("expected uploaded skill to include SKILL.md, got %#v", uploaded.Files)
	}
}

func TestServiceUploadSkillRejectsZipWithoutSkillMarkdown(t *testing.T) {
	service := NewService(&serviceTestRepository{})
	archive := buildSkillZip(t, map[string]string{
		"scripts/run.sh": "#!/usr/bin/env bash\n",
	})

	_, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: uuid.New(),
		Name:     "broken",
		Archive:  archive,
		Filename: "broken.zip",
	})
	if err == nil {
		t.Fatal("expected zip without SKILL.md to be rejected")
	}
}

type serviceTestRepository struct {
	upsertReq UpsertSkillPackageRequest
}

func (r *serviceTestRepository) ListSkills(context.Context, ListSkillsRequest) ([]*Skill, error) {
	return nil, nil
}

func (r *serviceTestRepository) GetSkill(context.Context, GetSkillRequest) (*Skill, error) {
	return nil, nil
}

func (r *serviceTestRepository) UpsertSkillPackage(_ context.Context, req UpsertSkillPackageRequest) (*Skill, error) {
	r.upsertReq = req
	return &Skill{
		ID:          uuid.New(),
		TenantID:    req.TenantID,
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		TeamIDs:     req.TeamIDs,
		Files:       req.Files,
	}, nil
}

func (r *serviceTestRepository) UpdateSkillFile(context.Context, UpdateSkillFileRequest) (*SkillFile, error) {
	return nil, nil
}

func buildSkillZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for path, content := range files {
		w, err := zw.Create(path)
		if err != nil {
			t.Fatalf("create zip file %s: %v", path, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip file %s: %v", path, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
