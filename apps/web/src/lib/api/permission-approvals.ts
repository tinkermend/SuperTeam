import type { ApiClientOptions } from "./client";
import { getJson, postJson } from "./client";

export type PermissionApprovalView = "mine" | "team";

export type PermissionApprovalStatus =
  | "pending"
  | "approved"
  | "rejected"
  | "needs_more_evidence"
  | "cancelled";

export type PermissionApprovalDecision = "approved" | "rejected" | "needs_more_evidence";

export type PermissionApprovalCategory = "permission" | "project_task";

export type PrivilegedRole = "approver" | "admin" | "owner";

export type PermissionApprovalAction = {
  key: string;
  label: string;
  tone?: string;
};

export type PermissionApproval = {
  id: string;
  tenant_id: string;
  category: PermissionApprovalCategory;
  resource_type: string;
  resource_id: string;
  requester_type: string;
  requester_id?: string;
  requester_name?: string;
  target_user_id?: string;
  decision_type: string;
  title: string;
  summary?: string;
  risk_level?: string;
  status: PermissionApprovalStatus;
  actions: PermissionApprovalAction[];
  context: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
};

export type PermissionApprovalListFilters = {
  view?: PermissionApprovalView;
  status?: PermissionApprovalStatus;
  risk_level?: string;
  resource_type?: string;
  limit?: number;
  offset?: number;
};

export type PermissionApprovalListPagination = {
  limit: number;
  offset: number;
  has_more: boolean;
};

export type PermissionApprovalListSummary = {
  open_count: number;
  high_risk_count: number;
  blocked_count: number;
};

export type PermissionApprovalListResponse = {
  items: PermissionApproval[];
  pagination: PermissionApprovalListPagination;
  summary: PermissionApprovalListSummary;
};

export type PermissionApprovalDecisionInput = {
  decision: PermissionApprovalDecision;
  note?: string;
  evidence_refs?: string[];
};

export type RequestPrivilegedRoleInput = {
  target_user_id: string;
  requested_role: PrivilegedRole;
  reason?: string;
};

function permissionApprovalsPath(filters: PermissionApprovalListFilters): string {
  const params = new URLSearchParams();

  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined) {
      params.set(key, String(value));
    }
  }

  const query = params.toString();
  return `/api/v1/permission-approvals${query ? `?${query}` : ""}`;
}

export function listPermissionApprovals(
  options: ApiClientOptions,
  filters: PermissionApprovalListFilters = {},
): Promise<PermissionApprovalListResponse> {
  return getJson<PermissionApprovalListResponse>(
    options,
    permissionApprovalsPath(filters),
    "permission approvals",
  );
}

export function decidePermissionApproval(
  options: ApiClientOptions,
  id: string,
  body: PermissionApprovalDecisionInput,
): Promise<PermissionApproval> {
  return postJson<PermissionApproval>(
    options,
    `/api/v1/permission-approvals/${id}/decision`,
    {
      decision: body.decision,
      note: body.note ?? "",
      evidence_refs: body.evidence_refs ?? [],
    },
    "permission approval decision",
  );
}

export function requestTeamPrivilegedRole(
  options: ApiClientOptions,
  teamId: string,
  body: RequestPrivilegedRoleInput,
): Promise<PermissionApproval> {
  return postJson<PermissionApproval>(
    options,
    `/api/v1/teams/${teamId}/privileged-role-requests`,
    {
      target_user_id: body.target_user_id,
      requested_role: body.requested_role,
      reason: body.reason ?? "",
    },
    "privileged role request",
  );
}
