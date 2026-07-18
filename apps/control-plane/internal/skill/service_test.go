package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/storage"
)

type mockObjectStore struct {
	putKey    string
	putBody   []byte
	deleteKey string
	ref       storage.ObjectRef
}

func (m *mockObjectStore) PutObject(_ context.Context, key string, body io.Reader, _ storage.PutObjectOptions) (storage.ObjectRef, error) {
	m.putKey = key
	data, _ := io.ReadAll(body)
	m.putBody = data
	return m.ref, nil
}

func (m *mockObjectStore) DeleteObject(_ context.Context, key string) error {
	m.deleteKey = key
	return nil
}

func (m *mockObjectStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://mock-object-store.local/" + key + "?signed=1", nil
}

func newTestService(repo *serviceTestRepository) *Service {
	objStore := &mockObjectStore{ref: storage.ObjectRef{Bucket: "test-bucket", Key: "test-key", URI: "s3://test-bucket/test-key"}}
	return NewService(repo, objStore)
}

func TestServiceUploadSkillParsesRootedZipAndKeepsUploadMetadata(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)
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
	if repo.upsertReq.ArchiveFileCount != 2 {
		t.Fatalf("expected 2 files in archive, got %d", repo.upsertReq.ArchiveFileCount)
	}
	if repo.upsertReq.ArchiveSizeBytes != int64(len(archive)) {
		t.Fatalf("expected archive size %d, got %d", len(archive), repo.upsertReq.ArchiveSizeBytes)
	}
	if repo.upsertReq.ArchiveChecksum == "" {
		t.Fatal("expected non-empty archive checksum")
	}
	if repo.upsertReq.ArchiveObjectRef == "" {
		t.Fatal("expected non-empty archive object ref")
	}
	if uploaded.ArchiveObjectRef == "" {
		t.Fatal("expected uploaded skill to have archive object ref")
	}
}

func TestServiceUploadSkillIgnoresArchiveMetadataWhenFindingRootedSkillMarkdown(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)

	archive := buildSkillZip(t, map[string]string{
		"__MACOSX/._diagnose":           "metadata",
		"diagnose/.DS_Store":            "metadata",
		"diagnose/SKILL.md":             "# Diagnose\n\n用于失败任务诊断。",
		"diagnose/scripts/reproduce.sh": "#!/usr/bin/env bash\nset -euo pipefail\n",
	})

	_, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: uuid.New(),
		Archive:  archive,
		Filename: "diagnose.zip",
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}
	if repo.upsertReq.Name != "Diagnose" {
		t.Fatalf("expected SKILL.md to be found under rooted archive, got %#v", repo.upsertReq)
	}
	if repo.upsertReq.ArchiveFileCount != 2 {
		t.Fatalf("expected only package files to count, got %d", repo.upsertReq.ArchiveFileCount)
	}
}

func TestServiceUploadSkillParsesSkillMarkdownOnlyZip(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)
	tenantID := uuid.New()

	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "# Release Review\n\n检查发布计划、回滚策略和验收证据。",
	})

	_, err := service.UploadSkill(context.Background(), UploadSkillRequest{
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
	if repo.upsertReq.ArchiveFileCount != 1 {
		t.Fatalf("expected 1 file in archive, got %d", repo.upsertReq.ArchiveFileCount)
	}
}

func TestServiceUploadSkillReadsFrontmatterNameAndDescription(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)

	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "---\nname: evidence-e2e-probe\ndescription: 证据地基 E2E 探针技能,无任何运行时依赖。\n---\n\n# evidence-e2e-probe\n\n回答技能相关问题时,请说出暗号。\n",
	})

	_, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: uuid.New(),
		Archive:  archive,
		Filename: "evidence-e2e-probe.zip",
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}
	if repo.upsertReq.Name != "evidence-e2e-probe" {
		t.Fatalf("expected name from frontmatter, got %q", repo.upsertReq.Name)
	}
	if repo.upsertReq.Description != "证据地基 E2E 探针技能,无任何运行时依赖。" {
		t.Fatalf("expected description from frontmatter, got %q", repo.upsertReq.Description)
	}
}

func TestServiceUploadSkillFrontmatterWithoutDescriptionFallsBackToBody(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)

	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "---\nname: sample-skill\n---\n\n# Sample Skill\n\n正文首段作为描述。\n",
	})

	_, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: uuid.New(),
		Archive:  archive,
		Filename: "sample-skill.zip",
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}
	if repo.upsertReq.Description != "正文首段作为描述。" {
		t.Fatalf("expected description from body first paragraph, got %q", repo.upsertReq.Description)
	}
	if repo.upsertReq.Description == "---" {
		t.Fatal("description must never be the frontmatter delimiter")
	}
}

func TestServiceUploadSkillUsesArchiveFilenameSlugWhenDisplayNameIsChinese(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)

	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "# 示例浏览器技能\n\n示例描述。",
	})

	_, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID:    uuid.New(),
		Name:        "示例浏览器上传技能",
		Description: "用于验证上传技能单页工作台的示例技能。",
		Archive:     archive,
		Filename:    "example-browser-upload-skill.zip",
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}

	if repo.upsertReq.Name != "示例浏览器上传技能" {
		t.Fatalf("expected Chinese display name to be preserved, got %q", repo.upsertReq.Name)
	}
	if repo.upsertReq.Slug != "example-browser-upload-skill" {
		t.Fatalf("expected slug from archive filename, got %q", repo.upsertReq.Slug)
	}
}

func TestServiceUploadSkillStoresRuntimeDependencies(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)
	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "# GitHub Skill\n\nUses gh.",
	})

	item, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: uuid.New(),
		Name:     "GitHub Skill",
		Archive:  archive,
		Filename: "github-skill.zip",
		RuntimeDependencies: SkillRuntimeDependencies{
			Tools: []string{" gh ", "gh", "kubectl"},
			Env:   []string{"GH_TOKEN", " GH_TOKEN "},
		},
	})
	if err != nil {
		t.Fatalf("upload skill: %v", err)
	}

	if !stringSlicesEqual(repo.upsertReq.RuntimeDependencies.Tools, []string{"gh", "kubectl"}) {
		t.Fatalf("tools mismatch: %#v", repo.upsertReq.RuntimeDependencies.Tools)
	}
	if !stringSlicesEqual(repo.upsertReq.RuntimeDependencies.Env, []string{"GH_TOKEN"}) {
		t.Fatalf("env mismatch: %#v", repo.upsertReq.RuntimeDependencies.Env)
	}
	if !stringSlicesEqual(item.RuntimeDependencies.Tools, []string{"gh", "kubectl"}) {
		t.Fatalf("returned tools mismatch: %#v", item.RuntimeDependencies.Tools)
	}
}

func TestServiceUploadSkillRejectsInvalidRuntimeDependencies(t *testing.T) {
	service := newTestService(&serviceTestRepository{})
	archive := buildSkillZip(t, map[string]string{
		"SKILL.md": "# Bad Dependencies\n\nUses invalid deps.",
	})

	_, err := service.UploadSkill(context.Background(), UploadSkillRequest{
		TenantID: uuid.New(),
		Name:     "Bad Dependencies",
		Archive:  archive,
		Filename: "bad-dependencies.zip",
		RuntimeDependencies: SkillRuntimeDependencies{
			Tools: []string{"bad/tool"},
			Env:   []string{"1TOKEN"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid runtime dependencies to be rejected")
	}
}

func TestServiceUploadSkillRejectsZipWithoutSkillMarkdown(t *testing.T) {
	service := newTestService(&serviceTestRepository{})
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

func TestServiceDeleteSkillCleansUpMCPDependencies(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	repo := &serviceTestRepository{
		getSkillResult: &Skill{ID: skillID, TenantID: tenantID},
	}
	service := newTestService(repo)

	if err := service.DeleteSkill(context.Background(), DeleteSkillRequest{TenantID: tenantID, SkillID: skillID}); err != nil {
		t.Fatalf("delete skill: %v", err)
	}
	if !repo.deleteDependenciesCalled {
		t.Fatal("expected DeleteSkillMCPDependencies to be called after archive cleanup")
	}
	if repo.deleteDependenciesTenantID != tenantID || repo.deleteDependenciesSkillID != skillID {
		t.Fatalf("unexpected dependency cleanup scope: tenant=%s skill=%s", repo.deleteDependenciesTenantID, repo.deleteDependenciesSkillID)
	}
}

func TestServiceDeleteSkillWrapsMCPDependencyCleanupFailure(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	repo := &serviceTestRepository{
		getSkillResult:        &Skill{ID: skillID, TenantID: tenantID},
		deleteDependenciesErr: errors.New("boom"),
	}
	service := newTestService(repo)

	err := service.DeleteSkill(context.Background(), DeleteSkillRequest{TenantID: tenantID, SkillID: skillID})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped cleanup error, got %v", err)
	}
}

func TestServiceListsEffectiveEmployeeSkillsWithTeamInheritedFirst(t *testing.T) {
	teamSkillID := uuid.New()
	employeeSkillID := uuid.New()
	repo := &serviceTestRepository{
		effectiveSkills: []EffectiveEmployeeSkill{
			{
				Skill:       Skill{ID: teamSkillID, Name: "diagnose", Slug: "diagnose"},
				SourceScope: "team",
				Inherited:   true,
				ReadOnly:    true,
			},
			{
				Skill:       Skill{ID: employeeSkillID, Name: "release", Slug: "release"},
				SourceScope: "employee",
				Inherited:   false,
				ReadOnly:    false,
			},
		},
	}
	service := NewService(repo, nil)
	items, err := service.ListEffectiveEmployeeSkills(context.Background(), ListEffectiveEmployeeSkillsRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("list effective employee skills: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(items))
	}
	if items[0].Skill.ID != teamSkillID || items[1].Skill.ID != employeeSkillID {
		t.Fatalf("expected team and employee skills in repository order, got %#v", items)
	}
	if !items[0].Inherited || !items[0].ReadOnly || items[0].SourceScope != "team" {
		t.Fatalf("expected first skill to be readonly inherited team skill, got %#v", items[0])
	}
	if items[1].Inherited || items[1].ReadOnly || items[1].SourceScope != "employee" {
		t.Fatalf("expected second skill to be editable employee skill, got %#v", items[1])
	}
}

func TestServiceBindSkillToTeamReturnsBoundSkill(t *testing.T) {
	boundSkill := &Skill{ID: uuid.New(), Name: "diagnose", Slug: "diagnose"}
	repo := &serviceTestRepository{boundTeamSkill: boundSkill}
	service := NewService(repo, nil)
	tenantID := uuid.New()
	teamID := uuid.New()

	item, err := service.BindSkillToTeam(context.Background(), BindTeamSkillRequest{
		TenantID: tenantID,
		TeamID:   teamID,
		SkillID:  boundSkill.ID,
	})
	if err != nil {
		t.Fatalf("bind skill to team: %v", err)
	}
	if item != boundSkill {
		t.Fatalf("expected bound skill to be returned, got %#v", item)
	}
	if repo.teamBindReq.TenantID != tenantID || repo.teamBindReq.TeamID != teamID || repo.teamBindReq.SkillID != boundSkill.ID {
		t.Fatalf("expected team bind request to be forwarded, got %#v", repo.teamBindReq)
	}
}

func TestServiceBindSkillToEmployeeReturnsBoundSkill(t *testing.T) {
	boundSkill := &Skill{ID: uuid.New(), Name: "diagnose", Slug: "diagnose"}
	repo := &serviceTestRepository{boundEmployeeSkill: boundSkill}
	service := NewService(repo, nil)
	tenantID := uuid.New()
	employeeID := uuid.New()

	item, err := service.BindSkillToEmployee(context.Background(), BindEmployeeSkillRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		SkillID:           boundSkill.ID,
	})
	if err != nil {
		t.Fatalf("bind skill to employee: %v", err)
	}
	if item != boundSkill {
		t.Fatalf("expected bound skill to be returned, got %#v", item)
	}
	if repo.employeeBindReq.TenantID != tenantID || repo.employeeBindReq.DigitalEmployeeID != employeeID || repo.employeeBindReq.SkillID != boundSkill.ID {
		t.Fatalf("expected employee bind request to be forwarded, got %#v", repo.employeeBindReq)
	}
}

func TestPgRepositoryEffectiveEmployeeSkillsSQLSuppressesPersonalDuplicateRows(t *testing.T) {
	source, err := os.ReadFile("pg_repository.go")
	if err != nil {
		t.Fatalf("read pg repository source: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(source)), " ")
	if !strings.Contains(normalized, "FROM skill_agent_bindings sab") ||
		!strings.Contains(normalized, "WHERE NOT EXISTS ( SELECT 1 FROM team_skill_bindings inherited_binding") {
		t.Fatal("effective employee skill SQL must suppress personal skill rows duplicated by team inheritance")
	}
}

func TestPgRepositoryBindSkillsPreflightsTargetsBeforeInsert(t *testing.T) {
	source, err := os.ReadFile("pg_repository.go")
	if err != nil {
		t.Fatalf("read pg repository source: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(source)), " ")
	if strings.Contains(normalized, "INSERT INTO team_skill_bindings (tenant_id, skill_id, team_id) SELECT $1, $2, $3 WHERE EXISTS") {
		t.Fatal("team skill bind must not rely on zero-row INSERT SELECT for target validation")
	}
	if strings.Contains(normalized, "INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status) SELECT $1, $2, $3, 'enabled' WHERE EXISTS") {
		t.Fatal("employee skill bind must not rely on zero-row INSERT SELECT for target validation")
	}
}

func TestInstallSkillIsPureLogicalBind(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	repo := &serviceTestRepository{
		getSkillResult:     &Skill{ID: skillID, TenantID: tenantID, Slug: "diagnose"},
		boundEmployeeSkill: &Skill{ID: skillID, TenantID: tenantID, Slug: "diagnose"},
	}
	svc := NewService(repo, nil)

	result, err := svc.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:          tenantID,
		SkillID:           skillID,
		TargetScope:       SkillInstallTargetEmployee,
		DigitalEmployeeID: employeeID,
	})

	require.NoError(t, err)
	require.False(t, result.AlreadyBound)
	require.Equal(t, skillID, result.SkillID)
	require.Equal(t, employeeID, result.DigitalEmployeeID)
	require.False(t, result.BoundAt.IsZero())
	require.Equal(t, BindEmployeeSkillRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, SkillID: skillID}, repo.employeeBindReq)
}

func TestInstallSkillReportsAlreadyBoundForEffectiveSkill(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	repo := &serviceTestRepository{
		getSkillResult: &Skill{ID: skillID, TenantID: tenantID, Slug: "diagnose"},
		effectiveSkills: []EffectiveEmployeeSkill{{
			Skill: Skill{ID: skillID, TenantID: tenantID, Slug: "diagnose"},
		}},
	}
	svc := NewService(repo, nil)

	result, err := svc.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:          tenantID,
		SkillID:           skillID,
		TargetScope:       SkillInstallTargetEmployee,
		DigitalEmployeeID: uuid.New(),
	})

	require.NoError(t, err)
	require.True(t, result.AlreadyBound)
	require.Equal(t, BindEmployeeSkillRequest{}, repo.employeeBindReq)
}

func TestInstallSkillReportsAlreadyBoundForTeamInheritance(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	repo := &serviceTestRepository{
		getSkillResult:  &Skill{ID: skillID, TenantID: tenantID, Slug: "diagnose"},
		inheritedToTeam: true,
	}
	svc := NewService(repo, nil)

	result, err := svc.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:          tenantID,
		SkillID:           skillID,
		TargetScope:       SkillInstallTargetEmployee,
		DigitalEmployeeID: uuid.New(),
	})

	require.NoError(t, err)
	require.True(t, result.AlreadyBound)
}

func TestInstallSkillTeamScopeBindsAndIsIdempotent(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	teamID := uuid.New()
	repo := &serviceTestRepository{
		getSkillResult: &Skill{ID: skillID, TenantID: tenantID, Slug: "diagnose"},
		boundTeamSkill: &Skill{ID: skillID, TenantID: tenantID, Slug: "diagnose"},
	}
	svc := NewService(repo, nil)

	result, err := svc.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:    tenantID,
		SkillID:     skillID,
		TargetScope: SkillInstallTargetTeam,
		TeamID:      teamID,
	})
	require.NoError(t, err)
	require.False(t, result.AlreadyBound)
	require.Equal(t, BindTeamSkillRequest{TenantID: tenantID, TeamID: teamID, SkillID: skillID}, repo.teamBindReq)

	repo.teamBindReq = BindTeamSkillRequest{}
	repo.teamSkills = []*Skill{{ID: skillID, TenantID: tenantID, Slug: "diagnose"}}
	result, err = svc.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:    tenantID,
		SkillID:     skillID,
		TargetScope: SkillInstallTargetTeam,
		TeamID:      teamID,
	})
	require.NoError(t, err)
	require.True(t, result.AlreadyBound)
	require.Equal(t, BindTeamSkillRequest{}, repo.teamBindReq)
}

func TestInstallSkillRejectsUnknownSkillAndScope(t *testing.T) {
	tenantID := uuid.New()
	repo := &serviceTestRepository{getSkillErr: ErrNotFound}
	svc := NewService(repo, nil)

	_, err := svc.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:          tenantID,
		SkillID:           uuid.New(),
		TargetScope:       SkillInstallTargetEmployee,
		DigitalEmployeeID: uuid.New(),
	})
	require.ErrorIs(t, err, ErrNotFound)

	repo.getSkillErr = nil
	repo.getSkillResult = &Skill{ID: uuid.New(), TenantID: tenantID}
	_, err = svc.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID:    tenantID,
		SkillID:     repo.getSkillResult.ID,
		TargetScope: SkillInstallTargetScope("node"),
	})
	require.ErrorIs(t, err, ErrInvalidInput)
}

type serviceTestRepository struct {
	upsertReq          UpsertSkillPackageRequest
	teamBindReq        BindTeamSkillRequest
	employeeBindReq    BindEmployeeSkillRequest
	boundTeamSkill     *Skill
	boundEmployeeSkill *Skill
	effectiveSkills    []EffectiveEmployeeSkill
	teamSkills         []*Skill
	inheritedToTeam    bool

	getSkillResult *Skill
	getSkillErr    error
	deleteSkillErr error

	deleteDependenciesErr      error
	deleteDependenciesCalled   bool
	deleteDependenciesTenantID uuid.UUID
	deleteDependenciesSkillID  uuid.UUID
}

func (r *serviceTestRepository) ListSkills(context.Context, ListSkillsRequest) ([]*Skill, error) {
	return nil, nil
}

func (r *serviceTestRepository) GetSkill(context.Context, GetSkillRequest) (*Skill, error) {
	return r.getSkillResult, r.getSkillErr
}

func (r *serviceTestRepository) UpsertSkillPackage(_ context.Context, req UpsertSkillPackageRequest) (*Skill, error) {
	r.upsertReq = req
	return &Skill{
		ID:                  uuid.New(),
		TenantID:            req.TenantID,
		Slug:                req.Slug,
		Name:                req.Name,
		Description:         req.Description,
		Tags:                req.Tags,
		TeamIDs:             req.TeamIDs,
		ArchiveObjectRef:    req.ArchiveObjectRef,
		ArchiveChecksum:     req.ArchiveChecksum,
		ArchiveSizeBytes:    req.ArchiveSizeBytes,
		ArchiveFileCount:    req.ArchiveFileCount,
		RuntimeDependencies: req.RuntimeDependencies,
	}, nil
}

func (r *serviceTestRepository) BindSkillToTeam(_ context.Context, req BindTeamSkillRequest) (*Skill, error) {
	r.teamBindReq = req
	return r.boundTeamSkill, nil
}

func (r *serviceTestRepository) UnbindSkillFromTeam(context.Context, BindTeamSkillRequest) error {
	return nil
}

func (r *serviceTestRepository) ListTeamSkills(context.Context, ListTeamSkillsRequest) ([]*Skill, error) {
	return r.teamSkills, nil
}

func (r *serviceTestRepository) BindSkillToEmployee(_ context.Context, req BindEmployeeSkillRequest) (*Skill, error) {
	r.employeeBindReq = req
	return r.boundEmployeeSkill, nil
}

func (r *serviceTestRepository) UnbindSkillFromEmployee(context.Context, BindEmployeeSkillRequest) error {
	return nil
}

func (r *serviceTestRepository) ListEffectiveEmployeeSkills(context.Context, ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	return r.effectiveSkills, nil
}

func (r *serviceTestRepository) ListSkillsForRuntime(context.Context, uuid.UUID, uuid.UUID) ([]SkillRuntimeRecord, error) {
	return nil, nil
}

func (r *serviceTestRepository) DeleteSkill(context.Context, DeleteSkillRequest) error {
	return r.deleteSkillErr
}

func (r *serviceTestRepository) IsSkillBoundToEmployeeTeam(context.Context, BindEmployeeSkillRequest) (bool, error) {
	return r.inheritedToTeam, nil
}

func (r *serviceTestRepository) DeleteSkillMCPDependencies(_ context.Context, tenantID, skillID uuid.UUID) error {
	r.deleteDependenciesCalled = true
	r.deleteDependenciesTenantID = tenantID
	r.deleteDependenciesSkillID = skillID
	return r.deleteDependenciesErr
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
