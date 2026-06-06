package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/skill"
)

func TestSkillRoutesUseConsoleTenantAndMultipartUpload(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeSkillService{}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	server.SetSkillHandler(skill.NewHandler(service))
	cookie := routeLogin(t, server, "admin", "admin")
	expectedTenantID := uuid.MustParse(auth.DefaultTenantID)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list skills to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listReq.TenantID != expectedTenantID {
		t.Fatalf("expected list tenant %s, got %s", expectedTenantID, service.listReq.TenantID)
	}
	var listed []struct {
		Name   string   `json:"name"`
		Tags   []string `json:"tags"`
		Rating *string  `json:"rating"`
		Files  []struct {
			Path string `json:"path"`
		} `json:"files"`
		AgentBindings []struct {
			AgentName string `json:"agent_name"`
		} `json:"agent_bindings"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list skills: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "diagnose" || listed[0].Tags[0] != "诊断" || listed[0].Files[0].Path != "SKILL.md" || listed[0].AgentBindings[0].AgentName != "需求澄清 Agent" {
		t.Fatalf("expected skill response with tags, file and agent binding, got %#v", listed)
	}
	if listed[0].Rating != nil {
		t.Fatalf("skill marketplace responses must not expose rating, got %#v", listed[0].Rating)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/skills/"+service.skillID.String()+"/files/scripts/collect.py", strings.NewReader(`{"content":"# updated"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(cookie)
	updateResp := httptest.NewRecorder()
	server.ServeHTTP(updateResp, updateReq)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected update skill file to succeed, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	if service.updateReq.TenantID != expectedTenantID || service.updateReq.SkillID != service.skillID || service.updateReq.Path != "scripts/collect.py" || service.updateReq.Content != "# updated" {
		t.Fatalf("expected update request to preserve tenant/skill/path/content, got %#v", service.updateReq)
	}

	teamID := uuid.New()
	body, contentType := buildSkillUploadMultipart(t, map[string]string{
		"name":        "diagnose-plus",
		"description": "上传诊断技能",
		"tags":        "诊断,自动化",
		"team_ids":    teamID.String(),
	}, map[string]string{
		"diagnose-plus/SKILL.md":            "# diagnose-plus\n",
		"diagnose-plus/scripts/collect.py":  "print('collect')\n",
		"diagnose-plus/references/check.md": "检查项\n",
	})
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/skills/uploads", bytes.NewReader(body))
	uploadReq.Header.Set("Content-Type", contentType)
	uploadReq.AddCookie(cookie)
	uploadResp := httptest.NewRecorder()
	server.ServeHTTP(uploadResp, uploadReq)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("expected upload skill to succeed, got %d: %s", uploadResp.Code, uploadResp.Body.String())
	}
	if service.uploadReq.TenantID != expectedTenantID || service.uploadReq.ActorUserID != user.ID || service.uploadReq.Name != "diagnose-plus" {
		t.Fatalf("expected upload tenant actor and name, got %#v", service.uploadReq)
	}
	if !stringSlicesEqual(service.uploadReq.Tags, []string{"诊断", "自动化"}) {
		t.Fatalf("expected parsed upload tags, got %#v", service.uploadReq.Tags)
	}
	if len(service.uploadReq.TeamIDs) != 1 || service.uploadReq.TeamIDs[0] != teamID {
		t.Fatalf("expected parsed team binding, got %#v", service.uploadReq.TeamIDs)
	}
	if len(service.uploadReq.Archive) == 0 || service.uploadReq.Filename != "skill.zip" {
		t.Fatalf("expected uploaded archive bytes and filename, got filename=%q size=%d", service.uploadReq.Filename, len(service.uploadReq.Archive))
	}
}

type routeSkillService struct {
	listReq   skill.ListSkillsRequest
	updateReq skill.UpdateSkillFileRequest
	uploadReq skill.UploadSkillRequest
	skillID   uuid.UUID
}

func (s *routeSkillService) ListSkills(_ context.Context, req skill.ListSkillsRequest) ([]*skill.Skill, error) {
	s.listReq = req
	s.skillID = uuid.New()
	return []*skill.Skill{{
		ID:          s.skillID,
		TenantID:    req.TenantID,
		Slug:        "diagnose",
		Name:        "diagnose",
		Description: "诊断流程",
		Version:     "v1.0.0",
		Status:      skill.SkillStatusInstalled,
		Tags:        []string{"诊断", "测试"},
		Files: []*skill.SkillFile{{
			Path:     "SKILL.md",
			FileType: skill.SkillFileTypeFile,
			Content:  "# diagnose",
		}},
		AgentBindings: []*skill.SkillAgentBinding{{
			AgentID:   uuid.New(),
			AgentName: "需求澄清 Agent",
			TeamName:  "产品与需求团队",
			Status:    "enabled",
		}},
	}}, nil
}

func (s *routeSkillService) GetSkill(context.Context, skill.GetSkillRequest) (*skill.Skill, error) {
	return nil, nil
}

func (s *routeSkillService) UpdateSkillFile(_ context.Context, req skill.UpdateSkillFileRequest) (*skill.SkillFile, error) {
	s.updateReq = req
	return &skill.SkillFile{
		Path:     req.Path,
		FileType: skill.SkillFileTypeFile,
		Content:  req.Content,
	}, nil
}

func (s *routeSkillService) UploadSkill(_ context.Context, req skill.UploadSkillRequest) (*skill.Skill, error) {
	s.uploadReq = req
	return &skill.Skill{
		ID:          uuid.New(),
		TenantID:    req.TenantID,
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Status:      skill.SkillStatusInstalled,
	}, nil
}

func buildSkillUploadMultipart(t *testing.T, fields map[string]string, files map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	for path, content := range files {
		w, err := zipWriter.Create(path)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", path, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", path, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	part, err := writer.CreateFormFile("file", "skill.zip")
	if err != nil {
		t.Fatalf("create upload file part: %v", err)
	}
	if _, err := part.Write(archive.Bytes()); err != nil {
		t.Fatalf("write upload file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
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
