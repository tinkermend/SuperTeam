import { describe, expect, it, vi } from "vitest";
import {
  approveDigitalEmployeeEffectiveConfig,
  createDigitalEmployee,
  createDigitalEmployeeConfigRevision,
  getDigitalEmployeeCreateOptions,
  getDigitalEmployeeOverview,
  createDigitalEmployeeRun,
  getDigitalEmployee,
  getDigitalEmployeeExecutionInstance,
  getDigitalEmployeeRun,
  listDigitalEmployeeRunEvents,
  listDigitalEmployeeRuns,
  listDigitalEmployees,
  previewDigitalEmployeeEffectiveConfig,
  stopDigitalEmployeeRun,
  type DigitalEmployee,
  type DigitalEmployeeCreateOptions,
  type DigitalEmployeeOverview,
} from "./employees";

describe("digital employee API", () => {
  it("lists digital employees with cookie credentials", async () => {
    const employees = [
      {
        id: "11111111-1111-4111-8111-111111111111",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        team_id: "99999999-9999-4999-8999-999999999999",
        owner_user_id: "22222222-2222-4222-8222-222222222222",
        employee_type: "database_admin",
        name: "数据库管理员工",
        role: "database_admin",
        status: "draft",
        permission_policy: { mode: "least_privilege" },
        context_policy: { mode: "task_slice" },
        approval_policy: { high_risk: "required" },
        risk_level: "medium",
      },
    ] satisfies DigitalEmployee[];
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(employees), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listDigitalEmployees({
        baseUrl: "http://control-plane.local",
        fetcher,
      }),
    ).resolves.toEqual(employees);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("lists digital employees filtered by team id", async () => {
    const teamId = "99999999-9999-4999-8999-999999999999";
    const employees = [
      {
        id: "11111111-1111-4111-8111-111111111111",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        team_id: teamId,
        owner_user_id: "22222222-2222-4222-8222-222222222222",
        employee_type: "database_admin",
        name: "数据库管理员工",
        role: "database_admin",
        status: "draft",
        permission_policy: { mode: "least_privilege" },
        context_policy: { mode: "task_slice" },
        approval_policy: { high_risk: "required" },
        risk_level: "medium",
      },
    ] satisfies DigitalEmployee[];
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(employees), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listDigitalEmployees(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        { team_id: teamId },
      ),
    ).resolves.toEqual(employees);

    expect(fetcher).toHaveBeenCalledWith(
      `http://control-plane.local/api/v1/digital-employees?team_id=${teamId}`,
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("gets digital employee overview with all filters and cookie credentials", async () => {
    const overview = {
      summary: {
        total_count: 1,
        runnable_count: 1,
        running_count: 1,
        waiting_runtime_count: 0,
        error_count: 0,
        high_risk_count: 0,
      },
      items: [],
      filters: {
        teams: [{ value: "team-1", label: "产品组" }],
        employee_types: [],
        statuses: [],
        providers: [],
        runtime_nodes: [],
        risk_levels: [],
        execution_statuses: [{ value: "missing", label: "未绑定 Runtime" }],
        run_statuses: [{ value: "none", label: "暂无运行" }],
      },
      pagination: { limit: 25, offset: 5, total_count: 1 },
    } satisfies DigitalEmployeeOverview;
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(overview), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getDigitalEmployeeOverview(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        {
          q: "需求",
          team_id: "team-1",
          status: "active",
          employee_type: "requirements_analyst",
          provider_type: "codex",
          runtime_node_id: "runtime-1",
          risk_level: "medium",
          execution_status: "missing",
          run_status: "none",
          limit: 25,
          offset: 5,
        },
      ),
    ).resolves.toEqual(overview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/overview?q=%E9%9C%80%E6%B1%82&team_id=team-1&status=active&employee_type=requirements_analyst&provider_type=codex&runtime_node_id=runtime-1&risk_level=medium&execution_status=missing&run_status=none&limit=25&offset=5",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("gets digital employee create options with encoded team id", async () => {
    const createOptions = {
      team_config: {
        id: "55555555-5555-4555-8555-555555555555",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        team_id: "team 1/ops",
        revision_number: 3,
        status: "approved",
        allowed_employee_types: ["database_admin"],
        allowed_provider_types: ["codex"],
        allowed_skills: ["incident-diagnosis"],
        allowed_mcp_servers: ["github"],
        allowed_external_capabilities: ["jira.search"],
        capability_policy: { mode: "allow_list" },
        context_policy: { max_refs: 8 },
        approval_policy: { high_risk: "required" },
        artifact_contract: { required: ["summary"] },
        internal_collaboration_policy: { handoff: "structured" },
        runtime_scope_policy: { allowed_nodes: ["runtime-1"] },
      },
      employee_types: [
        {
          type: "database_admin",
          label: "数据库管理",
          description: "负责数据库变更、备份、性能诊断和恢复验证",
          default_role: "database_admin",
          recommended_skills: ["incident-diagnosis"],
          recommended_mcp_servers: ["github"],
          recommended_provider_types: ["codex"],
          default_capability_selection: { skills: ["incident-diagnosis"] },
          default_context_policy_override: { max_refs: 8 },
          default_approval_policy: { high_risk: "required" },
          metadata: { pinned: true },
        },
      ],
      capability_options: {
        provider_types: ["codex"],
        skills: ["incident-diagnosis"],
        mcp_servers: ["github"],
        external_capabilities: ["jira.search"],
      },
      runtime_provider_options: [
        {
          runtime_node_id: "33333333-3333-4333-8333-333333333333",
          node_id: "runtime-a",
          runtime_name: "客户侧执行机",
          provider_type: "codex",
          runtime_status: "online",
          provider_status: "healthy",
          health_status: "healthy",
          current_load: 0,
          max_slots: 2,
          agent_home_dir: "/Users/wangpei/.codex",
          agent_home_dir_available: true,
          available: true,
        },
      ],
      policy_defaults: {
        permission_policy: { mode: "least_privilege" },
        context_policy_override: { max_refs: 8 },
        approval_policy: { high_risk: "required" },
        capability_selection: { provider_types: ["codex"] },
        runtime_selector: { strategy: "pinned" },
        workspace_policy: { mode: "ephemeral" },
        session_policy: { reuse: false },
        metadata: { source: "team_config" },
      },
    } satisfies DigitalEmployeeCreateOptions;
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(createOptions), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getDigitalEmployeeCreateOptions(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/ops",
      ),
    ).resolves.toEqual(createOptions);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/create-options?team_id=team+1%2Fops",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("creates digital employee with ready creation body and cookie credentials", async () => {
    const employee = {
      id: "11111111-1111-4111-8111-111111111111",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: "99999999-9999-4999-8999-999999999999",
      owner_user_id: "33333333-3333-4333-8333-333333333333",
      employee_type: "database_admin",
      name: "数据库管理员工",
      role: "database_admin",
      status: "ready",
      permission_policy: { mode: "least_privilege" },
      context_policy: { mode: "task_slice" },
      approval_policy: { high_risk: "required" },
      risk_level: "medium",
    } satisfies DigitalEmployee;
    const fetcher = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify(employee), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );

    await expect(
      createDigitalEmployee(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        {
          team_id: "99999999-9999-4999-8999-999999999999",
          employee_type: "database_admin",
          name: "数据库管理员工",
          avatar_asset_id: "engineer-m-01",
          role: "database_admin",
          description: "负责数据库变更和恢复验证",
          permission_policy: { mode: "least_privilege" },
          context_policy: { mode: "task_slice" },
          approval_policy: { high_risk: "required" },
          risk_level: "medium",
          metadata: { source: "web" },
          role_profile: { title: "database administrator" },
          constitution_addendum: { principles: ["evidence_first"] },
          capability_selection: { skills: ["incident-diagnosis"] },
          context_policy_override: { max_refs: 8 },
          approval_policy_override: { high_risk: "required" },
          output_contract_addendum: { required: ["summary"] },
          runtime_node_id: "33333333-3333-4333-8333-333333333333",
          provider_type: "codex",
          session_policy: { reuse: false },
          workspace_policy: { mode: "ephemeral" },
        },
      ),
    ).resolves.toEqual(employee);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees",
      {
        body: JSON.stringify({
          team_id: "99999999-9999-4999-8999-999999999999",
          employee_type: "database_admin",
          name: "数据库管理员工",
          avatar_asset_id: "engineer-m-01",
          role: "database_admin",
          description: "负责数据库变更和恢复验证",
          permission_policy: { mode: "least_privilege" },
          context_policy: { mode: "task_slice" },
          approval_policy: { high_risk: "required" },
          risk_level: "medium",
          metadata: { source: "web" },
          role_profile: { title: "database administrator" },
          constitution_addendum: { principles: ["evidence_first"] },
          capability_selection: { skills: ["incident-diagnosis"] },
          context_policy_override: { max_refs: 8 },
          approval_policy_override: { high_risk: "required" },
          output_contract_addendum: { required: ["summary"] },
          runtime_node_id: "33333333-3333-4333-8333-333333333333",
          provider_type: "codex",
          session_policy: { reuse: false },
          workspace_policy: { mode: "ephemeral" },
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
    const [, requestInit] = fetcher.mock.calls[0] as [
      RequestInfo | URL,
      RequestInit,
    ];
    const requestBody = JSON.parse(String(requestInit.body));
    expect(requestBody).not.toHaveProperty("owner_user_id");
  });

  it("rejects legacy draft creation input before posting to the backend", async () => {
    const fetcher = vi.fn();

    await expect(
      createDigitalEmployee(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        {
          team_id: "99999999-9999-4999-8999-999999999999",
          name: "数据库管理员工",
          role: "database_admin",
          description: "旧内联表单草稿",
        } as Parameters<typeof createDigitalEmployee>[1],
      ),
    ).rejects.toThrow(
      "digital employee ready creation requires employee_type, avatar_asset_id, runtime_node_id, and provider_type",
    );

    expect(fetcher).not.toHaveBeenCalled();
  });

  it("gets one digital employee with encoded employee id", async () => {
    const employee = {
      id: "employee 1/primary",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: "99999999-9999-4999-8999-999999999999",
      owner_user_id: "22222222-2222-4222-8222-222222222222",
      employee_type: "database_admin",
      name: "数据库管理员工",
      role: "database_admin",
      status: "active",
      permission_policy: { mode: "least_privilege" },
      context_policy: { mode: "task_slice" },
      approval_policy: { high_risk: "required" },
      risk_level: "medium",
    } satisfies DigitalEmployee;
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(employee), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getDigitalEmployee(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "employee 1/primary",
      ),
    ).resolves.toEqual(employee);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("creates config revision with encoded employee id and JSON body", async () => {
    const revision = {
      id: "44444444-4444-4444-8444-444444444444",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      digital_employee_id: "11111111-1111-4111-8111-111111111111",
      revision_number: 1,
      role_profile: { title: "requirements analyst" },
      constitution_addendum: {},
      capability_selection: { enabled_skills: ["incident-diagnosis"] },
      context_policy_override: {},
      approval_policy_override: {},
      output_contract_addendum: {},
      status: "draft",
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(revision), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );

    await expect(
      createDigitalEmployeeConfigRevision(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "employee 1/primary",
        {
          role_profile: { title: "requirements analyst" },
          capability_selection: { enabled_skills: ["incident-diagnosis"] },
          status: "draft",
        },
      ),
    ).resolves.toEqual(revision);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/config-revisions",
      {
        body: JSON.stringify({
          role_profile: { title: "requirements analyst" },
          capability_selection: { enabled_skills: ["incident-diagnosis"] },
          status: "draft",
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("previews effective config with encoded employee id and revision refs", async () => {
    const preview = {
      team_config_revision_id: "55555555-5555-4555-8555-555555555555",
      employee_config_revision_id: "44444444-4444-4444-8444-444444444444",
      effective_config: {
        team_config_revision_id: "55555555-5555-4555-8555-555555555555",
      },
      validation: {
        blocking_errors: [
          {
            code: "capability_not_allowed",
            path: "capability_selection.enabled_skills[0]",
            message: "能力不在团队白名单中",
          },
        ],
        warnings: [],
      },
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(preview), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      previewDigitalEmployeeEffectiveConfig(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "employee 1/primary",
        {
          team_config: { id: "55555555-5555-4555-8555-555555555555" },
          employee_config: { id: "44444444-4444-4444-8444-444444444444" },
        },
      ),
    ).resolves.toEqual(preview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/effective-configs/preview",
      {
        body: JSON.stringify({
          team_config: { id: "55555555-5555-4555-8555-555555555555" },
          employee_config: { id: "44444444-4444-4444-8444-444444444444" },
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("approves effective config with encoded employee id and preview refs", async () => {
    const effectiveConfig = {
      id: "66666666-6666-4666-8666-666666666666",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      digital_employee_id: "11111111-1111-4111-8111-111111111111",
      team_config_revision_id: "55555555-5555-4555-8555-555555555555",
      employee_config_revision_id: "44444444-4444-4444-8444-444444444444",
      effective_config: {
        team_config_revision_id: "55555555-5555-4555-8555-555555555555",
      },
      validation_result: {
        blocking_errors: [],
        warnings: [],
      },
      status: "approved",
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(effectiveConfig), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );

    await expect(
      approveDigitalEmployeeEffectiveConfig(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "employee 1/primary",
        {
          preview: {
            team_config: { id: "55555555-5555-4555-8555-555555555555" },
            employee_config: { id: "44444444-4444-4444-8444-444444444444" },
          },
        },
      ),
    ).resolves.toEqual(effectiveConfig);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/effective-configs/approve",
      {
        body: JSON.stringify({
          preview: {
            team_config: { id: "55555555-5555-4555-8555-555555555555" },
            employee_config: { id: "44444444-4444-4444-8444-444444444444" },
          },
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("gets execution instance with encoded employee id and cookie credentials", async () => {
    const instance = {
      id: "22222222-2222-4222-8222-222222222222",
      digital_employee_id: "11111111-1111-4111-8111-111111111111",
      runtime_node_id: "33333333-3333-4333-8333-333333333333",
      provider_type: "codex",
      status: "ready",
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(instance), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getDigitalEmployeeExecutionInstance(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "employee 1/primary",
      ),
    ).resolves.toEqual(instance);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/execution-instance",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("creates a digital employee run with encoded employee id and JSON body", async () => {
    const run = {
      id: "run 1/primary",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      task_id: "task-1",
      digital_employee_id: "employee 1/primary",
      execution_instance_id: "instance-1",
      runtime_node_id: "runtime-1",
      node_id: "node-a",
      command_id: "cmd-1",
      provider_type: "codex",
      status: "dispatching",
      result: {},
      diagnostic: {},
      work_products: [],
      session_state: {},
      timed_out: false,
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(run), {
          headers: { "content-type": "application/json" },
          status: 201,
        }),
    );

    await expect(
      createDigitalEmployeeRun(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        {
          objective: "整理上线风险",
          prompt: "检查最近失败任务",
          allowed_actions: ["artifact.read"],
          timeout_sec: 900,
        },
      ),
    ).resolves.toEqual(run);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/runs",
      {
        body: JSON.stringify({
          objective: "整理上线风险",
          prompt: "检查最近失败任务",
          allowed_actions: ["artifact.read"],
          timeout_sec: 900,
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("lists digital employee runs with encoded employee id and pagination", async () => {
    const runs = [
      {
        id: "run-1",
        tenant_id: "tenant-1",
        task_id: "task-1",
        digital_employee_id: "employee 1/primary",
        execution_instance_id: "instance-1",
        runtime_node_id: "runtime-1",
        node_id: "node-a",
        command_id: "cmd-1",
        provider_type: "codex",
        status: "running",
        result: {},
        diagnostic: {},
        work_products: [],
        session_state: {},
        timed_out: false,
      },
    ];
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(runs), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listDigitalEmployeeRuns(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        { limit: 10, offset: 20 },
      ),
    ).resolves.toEqual(runs);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/runs?limit=10&offset=20",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("gets a digital employee run with encoded employee and run ids", async () => {
    const run = {
      id: "run 1/primary",
      tenant_id: "tenant-1",
      task_id: "task-1",
      digital_employee_id: "employee 1/primary",
      execution_instance_id: "instance-1",
      runtime_node_id: "runtime-1",
      node_id: "node-a",
      command_id: "cmd-1",
      provider_type: "codex",
      status: "completed",
      result: { summary: "完成" },
      diagnostic: {},
      work_products: [],
      session_state: {},
      timed_out: false,
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(run), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      getDigitalEmployeeRun(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        "run 1/primary",
      ),
    ).resolves.toEqual(run);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/runs/run%201%2Fprimary",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("lists digital employee run events with encoded ids and pagination", async () => {
    const events = [
      {
        event_type: "provider.stdout",
        sequence_number: 7,
        payload: { text: "分析中" },
        provider_session_external_id: "session-ext-1",
        session_state_patch: { cursor: 7 },
        log_ref: "s3://logs/run-1",
        raw_event_ref: "s3://events/run-1/7",
        metadata: { source: "runtime" },
      },
    ];
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(events), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listDigitalEmployeeRunEvents(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        "run 1/primary",
        { limit: 25, offset: 5 },
      ),
    ).resolves.toEqual(events);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/runs/run%201%2Fprimary/events?limit=25&offset=5",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("stops a digital employee run with encoded ids and reason", async () => {
    const run = {
      id: "run 1/primary",
      tenant_id: "tenant-1",
      task_id: "task-1",
      digital_employee_id: "employee 1/primary",
      execution_instance_id: "instance-1",
      runtime_node_id: "runtime-1",
      node_id: "node-a",
      command_id: "cmd-1",
      provider_type: "codex",
      status: "cancelling",
      result: {},
      diagnostic: {},
      work_products: [],
      session_state: {},
      timed_out: false,
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(run), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      stopDigitalEmployeeRun(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        "run 1/primary",
        { reason: "用户从 Web 停止" },
      ),
    ).resolves.toEqual(run);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/runs/run%201%2Fprimary/stop",
      {
        body: JSON.stringify({ reason: "用户从 Web 停止" }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });
});
