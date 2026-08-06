package employee

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/skill"
)

// fakeClosureResolver 把注册表定义原样返回，模拟「MCP 在注册表里存在但没绑给员工/项目」。
type fakeClosureResolver struct {
	byID  map[string]RuntimeMCPServerPayload
	calls int
}

func (f *fakeClosureResolver) ResolveRuntimeMCPServer(_ context.Context, _, serverID uuid.UUID) (*RuntimeMCPServerPayload, error) {
	f.calls++
	if payload, ok := f.byID[serverID.String()]; ok {
		out := payload
		return &out, nil
	}
	return nil, nil
}

type fakeEnvLister struct {
	records []RuntimeEnvironmentVariablePayload
}

func (f *fakeEnvLister) ListRuntimeEnvironmentVariablesForRuntime(_ context.Context, _, _ uuid.UUID) ([]RuntimeEnvironmentVariablePayload, error) {
	return f.records, nil
}

func closureTestService(t *testing.T) (*DigitalEmployeeRunService, *fakeRunServiceRepository, *fakeRunServiceDispatcher) {
	t.Helper()
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	return mustNewRunService(t, repo, dispatcher), repo, dispatcher
}

// 闭包补全的正向：MCP 在注册表里、没绑给员工，但员工已配齐它要的环境变量
// → 闭包补上（source=dependency_closure），派发放行。技能不会半残。
func TestDependencyClosureGrantsWhenEnvProvisioned(t *testing.T) {
	service, repo, dispatcher := closureTestService(t)

	skillID := uuid.New()
	serverID := uuid.New()
	service.SetSkillLister(&fakeRuntimeSkillLister{records: []skill.SkillRuntimeRecord{{
		ID:   skillID,
		Slug: "deploy-helper",
	}}})
	service.SetMCPLister(&fakeRuntimeMCPLister{})
	service.SetEnvironmentLister(&fakeEnvLister{records: []RuntimeEnvironmentVariablePayload{
		{Name: "GITHUB_TOKEN", Value: "x"},
	}})
	service.SetSkillMCPDependencyLister(&fakeSkillMCPDependencyLister{records: []SkillMCPDependencyRecord{
		{SkillID: skillID, MCPServerID: serverID.String(), ServerKey: "github-mcp"},
	}})
	service.SetMCPDefinitionResolver(&fakeClosureResolver{byID: map[string]RuntimeMCPServerPayload{
		serverID.String(): {
			ServerID:        serverID.String(),
			ServerKey:       "github-mcp",
			Name:            "GitHub MCP",
			RequiredEnvVars: []string{"GITHUB_TOKEN"},
		},
	}})

	if _, err := createLegacyStandaloneRun(service, repo.preflight, validCreateRunServiceRequest()); err != nil {
		t.Fatalf("env-satisfied dependency must be closed over, got %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected dispatch, got %#v", dispatcher.commands)
	}
}

// 承重反向：MCP 在注册表里，但员工**没有**配它要的环境变量。
// 闭包不得授予——否则紧随其后的 validateSkillMCPDependencies（契约是
// "dependency validates, never grants"）会因为闭包刚把它塞进去而永真，
// 凭据缺失就从派发期拦截退化成运行期莫名失败。
func TestDependencyClosureDoesNotGrantWhenEnvMissing(t *testing.T) {
	service, repo, dispatcher := closureTestService(t)

	skillID := uuid.New()
	serverID := uuid.New()
	service.SetSkillLister(&fakeRuntimeSkillLister{records: []skill.SkillRuntimeRecord{{
		ID:   skillID,
		Slug: "deploy-helper",
	}}})
	service.SetMCPLister(&fakeRuntimeMCPLister{})
	// 员工只有无关的环境变量，缺 GITHUB_TOKEN。
	service.SetEnvironmentLister(&fakeEnvLister{records: []RuntimeEnvironmentVariablePayload{
		{Name: "UNRELATED", Value: "x"},
	}})
	service.SetSkillMCPDependencyLister(&fakeSkillMCPDependencyLister{records: []SkillMCPDependencyRecord{
		{SkillID: skillID, MCPServerID: serverID.String(), ServerKey: "github-mcp"},
	}})
	resolver := &fakeClosureResolver{byID: map[string]RuntimeMCPServerPayload{
		serverID.String(): {
			ServerID:        serverID.String(),
			ServerKey:       "github-mcp",
			Name:            "GitHub MCP",
			RequiredEnvVars: []string{"GITHUB_TOKEN"},
		},
	}}
	service.SetMCPDefinitionResolver(resolver)

	_, err := createLegacyStandaloneRun(service, repo.preflight, validCreateRunServiceRequest())
	if err == nil {
		t.Fatal("env-unsatisfied dependency must NOT be granted by closure; the gate must block dispatch")
	}
	if !strings.Contains(err.Error(), "skill_mcp_dependencies_not_satisfied") {
		t.Fatalf("expected the dependency gate to fire, got %v", err)
	}
	if !strings.Contains(err.Error(), "github-mcp") {
		t.Fatalf("expected error to name server_key, got %v", err)
	}
	if resolver.calls == 0 {
		t.Fatal("resolver must have been consulted (otherwise this test would pass vacuously)")
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected no dispatch, got %#v", dispatcher.commands)
	}
}

func TestMissingProvisionedEnvVars(t *testing.T) {
	provisioned := map[string]struct{}{"A": {}, "B": {}}
	if got := missingProvisionedEnvVars([]string{"A", "B"}, provisioned); len(got) != 0 {
		t.Fatalf("all provisioned → none missing, got %v", got)
	}
	if got := missingProvisionedEnvVars([]string{"A", " ", "C"}, provisioned); len(got) != 1 || got[0] != "C" {
		t.Fatalf("blank names must be ignored, only C missing; got %v", got)
	}
	if got := missingProvisionedEnvVars(nil, provisioned); len(got) != 0 {
		t.Fatalf("no requirement → none missing, got %v", got)
	}
}
