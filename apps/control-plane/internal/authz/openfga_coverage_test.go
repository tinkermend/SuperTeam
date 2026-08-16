package authz

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// 本文件是 OpenFGA 迁移债务的护栏,不修任何债,只让债停止隐形增长。
//
// 背景:authz action 表每周在长,openfga_mapping.go 自试验性接入后基本没动过。
// 这个差距在默认配置下完全不可见——
//   - AUTHZ_ENGINE=db(默认):根本不查 FGA;
//   - openfga_shadow:未映射 action 被静默跳过,不查不记不算背离;
//   - openfga(纯执行):未映射 action 直接拒绝。
// 于是形成最坏的组合:平时零感知,切换瞬间大面积 403。
//
// 详见 docs/OPENFGA_MIGRATION_DEBT.md。

// knownUnmappedActions 是"已知未接入 OpenFGA"的显式清单。
//
// 新增 authz action 时二选一:
//  1. 在 openFGARelationForAction 里映射它。同时必须确认 openFGAObjectForRequest
//     能为它的资源类型产出对象,且该对象类型在 model.fga 里存在——否则等于没映射,
//     甚至更糟(见台账 D2/D4 与 TestFGAEmittableObjectTypesExistInModel)。
//  2. 若本期不接入(当前多数情况),加进本清单,并更新
//     docs/OPENFGA_MIGRATION_DEBT.md 的 D1。
//
// 不允许静默跳过——那正是本护栏要杜绝的事。
// 债还清后请删掉对应行,否则反向断言会失败并提醒你。
var knownUnmappedActions = map[string]string{
	// audit
	ActionAuditRead: "audit",

	// credential
	ActionCredentialCreate: "credential",
	ActionCredentialDelete: "credential",
	ActionCredentialRead:   "credential",

	// employee
	ActionEmployeeCapabilityEdit: "employee",
	ActionEmployeeConfigApprove:  "employee",
	ActionEmployeeConfigCreate:   "employee",
	ActionEmployeeConfigPreview:  "employee",
	ActionEmployeeExecutionBind:  "employee",
	ActionEmployeeProfileUpdate:  "employee",
	ActionEmployeeRead:           "employee",
	ActionEmployeeRunCreate:      "employee",
	ActionEmployeeRunLogRead:     "employee",
	ActionEmployeeRunStop:        "employee",
	ActionEmployeeStatusUpdate:   "employee",
	ActionEmployeeTeamUpdate:     "employee",

	// mcp_registry
	ActionMCPRegistryManage: "mcp_registry",
	ActionMCPRegistryRead:   "mcp_registry",

	// project——整域缺失,且 ResourceProject 也产不出 FGA 对象(台账 D2/D3)
	ActionProjectAcceptanceCreate: "project",
	ActionProjectAcceptanceRead:   "project",
	ActionProjectArchive:          "project",
	ActionProjectArtifactRead:     "project",
	ActionProjectBudgetRead:       "project",
	ActionProjectConfigEdit:       "project",
	ActionProjectConfigRead:       "project",
	ActionProjectCreate:           "project",
	ActionProjectDecisionRead:     "project",
	ActionProjectDecisionResolve:  "project",
	ActionProjectDelete:           "project",
	ActionProjectDemandRead:       "project",
	ActionProjectDemandSubmit:     "project",
	ActionProjectEventRead:        "project",
	ActionProjectEvidenceCreate:   "project",
	ActionProjectEvidenceRead:     "project",
	ActionProjectEvidenceUpdate:   "project",
	ActionProjectMemberManage:     "project",
	ActionProjectMemberRead:       "project",
	ActionProjectRead:             "project",
	ActionProjectReportRead:       "project",
	ActionProjectTaskRead:         "project",
	ActionProjectUpdate:           "project",

	// scenario_template
	ActionScenarioTemplateManage: "scenario_template",
	ActionScenarioTemplateRead:   "scenario_template",

	// skill——刻意不映射。model.fga 没有 skill 类型,贸然映射会拿 skill:{id}
	// 去查一个不存在的类型(台账 D4)。skill.install 是唯一例外,它靠把对象
	// 改写成 tenant:{id} 绕过去。
	ActionSkillArchiveReplace: "skill",
	ActionSkillDelete:         "skill",
	ActionSkillRead:           "skill",
	ActionSkillUpload:         "skill",

	// system_config
	ActionSystemConfigManage: "system_config",
	ActionSystemConfigRead:   "system_config",

	// task

	// team——其余 team action 均已映射,只剩这一个
	ActionTeamGovernanceEdit: "team",

	// misc
	ActionManageSystemTemplates: "misc",
}

type resourceFGAStatus struct {
	emitsObject bool
	note        string
}

// resourceFGAObjectStatus 必须覆盖**每一个**已声明的资源类型,逐个表态能否产出
// FGA 对象。这是台账 D2:即使补了 relation,资源产不出对象也等于没映射。
//
// 刻意做成全覆盖而非只列"能产出的":只列能产出的那种写法下,新增一个未处理的
// 资源类型会让"代码没处理"和"清单没记录"两个 false 相互抵消,守卫恒绿——
// 这个缺陷是实现期被反例验证抓出来的,勿改回去。
var resourceFGAObjectStatus = map[string]resourceFGAStatus{
	ResourceConsole: {true, "改写成 tenant:{tenantID}"},
	ResourceSkill:   {true, "install 改写成 tenant:{tenantID};其余产出 skill:{id}(该类型不在 model 里,见 D4)"},
	ResourceTeam:    {true, "team:{id}"},
	ResourceTenant:  {true, "tenant:{id}"},

	ResourceCredential: {false, "台账 D2:未接入 OpenFGA"},
	ResourceEmployee:   {false, "台账 D2:未接入 OpenFGA"},
	ResourceProject:    {false, "台账 D2/D3:model.fga 有 project 类型,但没有任何代码产出 project:{id};项目团队范围实际走 team:{id} + project_scope_user"},
}

// knownObjectTypesMissingFromModel 记录"代码能产出、但 model.fga 里不存在"的对象类型。
// 每一项都是一颗地雷:只要有任何 action 被映射且走到这个分支,FGA Check 就会拿一个
// 模型里不存在的类型去查——shadow 下记成假背离,enforcing 下报错拒绝。
var knownObjectTypesMissingFromModel = map[string]string{
	ResourceSkill: "台账 D4:model 无 skill 类型。skill.install 靠对象改写成 tenant 绕过;" +
		"其余 skill action 一律不得映射,否则立刻踩雷。",
}

func TestEveryAuthzActionIsMappedOrExplicitlyDeferred(t *testing.T) {
	actions := scanPackageStringConsts(t, "Action")

	mappedCount := 0
	for ident, action := range actions {
		_, mapped := openFGARelationForAction(action)
		_, deferred := knownUnmappedActions[action]

		switch {
		case mapped && deferred:
			t.Errorf(`action %q(%s)既已接入 OpenFGA,又留在 knownUnmappedActions 里。

债已还清就把它从 knownUnmappedActions 删掉,并更新 docs/OPENFGA_MIGRATION_DEBT.md 的 D1。
留着过期条目会让清单退化成没人信的名单。`, action, ident)
		case mapped:
			mappedCount++
		case deferred:
			// 已显式登记,放行。
		default:
			t.Errorf(`action %q(%s)未接入 OpenFGA,也不在 knownUnmappedActions 清单里。

新增 authz action 时必须二选一:
  1) 在 openfga_mapping.go 的 openFGARelationForAction 里映射它;
     注意同时确认 openFGAObjectForRequest 能为它的资源类型产出对象,
     且该对象类型在 authz/openfga/model.fga 里存在,否则 FGA 会拿一个
     模型里不存在的对象类型去查(见台账 D2/D4)。
  2) 若本期不接入(当前多数情况),把它加进本文件的 knownUnmappedActions,
     并更新 docs/OPENFGA_MIGRATION_DEBT.md 的 D1。

背景:AUTHZ_ENGINE=openfga 时未映射 action 一律拒绝,
     而 shadow 模式会静默跳过它们、不产生任何告警——
     所以"shadow 跑了很久没告警"不能作为可以切换的依据。`, action, ident)
		}
	}

	// 反向:清单里的每一项都必须仍是真实存在的 action。
	actionValues := map[string]struct{}{}
	for _, action := range actions {
		actionValues[action] = struct{}{}
	}
	for action := range knownUnmappedActions {
		if _, ok := actionValues[action]; !ok {
			t.Errorf(`knownUnmappedActions 里的 %q 已不是真实存在的 authz action。

该 action 可能已被重命名或删除,请同步删掉这一行。`, action)
		}
	}

	t.Logf("OpenFGA action 覆盖率: %d/%d,未映射 %d(明细见 docs/OPENFGA_MIGRATION_DEBT.md D1)",
		mappedCount, len(actions), len(actions)-mappedCount)
}

func TestFGAObjectMappingCoversDeclaredResourceTypes(t *testing.T) {
	resources := scanPackageStringConsts(t, "Resource")

	for ident, resource := range resources {
		status, recorded := resourceFGAObjectStatus[resource]
		if !recorded {
			t.Errorf(`资源类型 %q(%s)未在 resourceFGAObjectStatus 里表态。

新增资源类型时必须登记它能否产出 FGA 对象:
  - 已在 openFGAObjectForRequest 里处理 → 记 {true, "产出什么对象"};
  - 尚未处理 → 记 {false, "台账 D2:未接入 OpenFGA"},
    同时该资源域的 action 必须留在 knownUnmappedActions 里。

不登记就等于默认"两边都没做"而守卫恒绿——正是本护栏要杜绝的事。`, resource, ident)
			continue
		}

		req := CheckRequest{
			Actor:    ActorRef{Type: ActorUser, ID: "00000000-0000-4000-8000-000000000001"},
			Action:   ActionTenantAccess,
			Resource: ResourceRef{Type: resource, ID: "00000000-0000-0000-0000-000000000001"},
			TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		}
		_, produced := openFGAObjectForRequest(req)

		if produced != status.emitsObject {
			t.Errorf(`资源类型 %q(%s)能否产出 FGA 对象与 resourceFGAObjectStatus 记录不符(实际 %v,记录 %v:%s)。

接入或退出 OpenFGA 时请同步更新这条记录,并核对 docs/OPENFGA_MIGRATION_DEBT.md 的 D2。`,
				resource, ident, produced, status.emitsObject, status.note)
		}
	}

	// 反向:登记的每一项都必须仍是真实存在的资源类型。
	resourceValues := map[string]struct{}{}
	for _, resource := range resources {
		resourceValues[resource] = struct{}{}
	}
	for resource := range resourceFGAObjectStatus {
		if _, ok := resourceValues[resource]; !ok {
			t.Errorf(`resourceFGAObjectStatus 里的 %q 已不是真实存在的资源类型,请删掉这一行。`, resource)
		}
	}
}

func TestFGAEmittableObjectTypesExistInModel(t *testing.T) {
	modelTypes := scanFGAModelTypes(t)

	for resource, status := range resourceFGAObjectStatus {
		if !status.emitsObject {
			continue
		}
		req := CheckRequest{
			Actor:    ActorRef{Type: ActorUser, ID: "00000000-0000-4000-8000-000000000001"},
			Action:   ActionTenantAccess,
			Resource: ResourceRef{Type: resource, ID: "00000000-0000-0000-0000-000000000001"},
			TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		}
		object, ok := openFGAObjectForRequest(req)
		if !ok {
			continue
		}
		objectType, _, _ := strings.Cut(object, ":")

		_, inModel := modelTypes[objectType]
		_, knownMissing := knownObjectTypesMissingFromModel[objectType]

		switch {
		case inModel && knownMissing:
			t.Errorf(`对象类型 %q 已存在于 model.fga,但仍留在 knownObjectTypesMissingFromModel 里。

地雷已排除,请删掉这一行,并更新 docs/OPENFGA_MIGRATION_DEBT.md 的 D4。`, objectType)
		case inModel:
			// 正常。
		case knownMissing:
			// 已登记的已知地雷,放行。
		default:
			t.Errorf(`资源 %q 会产出对象类型 %q(%s),但 authz/openfga/model.fga 里没有这个类型。

FGA Check 会拿一个模型里不存在的类型去查:
  - shadow 模式下报错被记成 diff=true,产生假背离告警(台账 D6);
  - enforcing 模式下直接报错拒绝。

要么在 model.fga 里补这个类型,要么像 skill.install 那样把对象改写成已有类型,
要么把它登记进 knownObjectTypesMissingFromModel 并在台账里记一条。`, resource, objectType, status.note)
		}
	}

	// 反向:登记的地雷必须仍可被产出,否则是过期条目。
	emittable := map[string]struct{}{}
	for resource, status := range resourceFGAObjectStatus {
		if !status.emitsObject {
			continue
		}
		req := CheckRequest{
			Actor:    ActorRef{Type: ActorUser, ID: "00000000-0000-4000-8000-000000000001"},
			Action:   ActionTenantAccess,
			Resource: ResourceRef{Type: resource, ID: "00000000-0000-0000-0000-000000000001"},
			TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		}
		if object, ok := openFGAObjectForRequest(req); ok {
			objectType, _, _ := strings.Cut(object, ":")
			emittable[objectType] = struct{}{}
		}
	}
	for objectType := range knownObjectTypesMissingFromModel {
		if _, ok := emittable[objectType]; !ok {
			t.Errorf(`knownObjectTypesMissingFromModel 里的 %q 已不会被任何资源产出,请删掉这一行。`, objectType)
		}
	}
}

// scanPackageStringConsts 用 AST 收集本包内所有"标识符以 prefix 开头且值为字符串
// 字面量"的常量,返回 标识符 → 值。
//
// Go 常量无法反射枚举,而手工维护一份切片只是把"忘记同步"的问题挪个地方——
// 护栏必须自己去源码里数,才谈得上护栏。
//
// 用 os.ReadDir + parser.ParseFile 而非 parser.ParseDir:后者自 Go 1.22 起废弃。
func scanPackageStringConsts(t *testing.T, prefix string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读取包目录失败: %v", err)
	}

	fset := token.NewFileSet()
	found := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("解析 %s 失败: %v", name, parseErr)
		}
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range valueSpec.Names {
					// 必须 HasPrefix 而非 Contains:types.go 里有
					// ReasonUnsupportedAction = "unsupported action",
					// 用 Contains 判别会把它算成一个 action,分母凭空 +1。
					if !strings.HasPrefix(ident.Name, prefix) {
						continue
					}
					if i >= len(valueSpec.Values) {
						continue
					}
					lit, ok := valueSpec.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					value, unquoteErr := strconv.Unquote(lit.Value)
					if unquoteErr != nil {
						t.Fatalf("常量 %s 的字面量无法解析: %v", ident.Name, unquoteErr)
					}
					found[ident.Name] = value
				}
			}
		}
	}
	if len(found) == 0 {
		t.Fatalf("未在本包扫描到任何以 %q 开头的字符串常量——扫描逻辑或常量声明位置已变", prefix)
	}
	return found
}

// scanFGAModelTypes 从 openfga/model.fga 里收集已声明的类型名。
// model.fga 是 DSL,格式稳定(顶格 "type X"),用行扫描足够,不值得引 FGA 的解析器。
func scanFGAModelTypes(t *testing.T) map[string]struct{} {
	t.Helper()

	file, err := os.Open("openfga/model.fga")
	if err != nil {
		t.Fatalf("打开 model.fga 失败: %v", err)
	}
	defer func() { _ = file.Close() }()

	types := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// 顶格才是类型声明;关系定义有缩进。
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		name, found := strings.CutPrefix(strings.TrimSpace(line), "type ")
		if !found {
			continue
		}
		if name = strings.TrimSpace(name); name != "" {
			types[name] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("读取 model.fga 失败: %v", err)
	}
	if len(types) == 0 {
		t.Fatal("未从 model.fga 扫描到任何类型——扫描逻辑或文件格式已变")
	}

	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	t.Logf("model.fga 已声明类型: %s", strings.Join(names, ", "))
	return types
}
