import { apiBaseUrl, requestApi } from "./api";
import { getToken } from "./auth";

export interface Case {
  caseId: string;
  caseType: string;
  description: string;
}

export interface Evidence {
  id: number;
  category: string;
  mediaType?: string;
  filename: string;
  storageRef?: string;
  contentType?: string;
  sizeBytes: number;
  createdAt: string;
}

export interface CaseDetail {
  caseId: string;
  caseType: string;
  description: string;
  evidences: Evidence[];
}

export interface ListCasesResponse {
  items: Case[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}

export interface ListCasesParams {
  page?: number;
  pageSize?: number;
  caseType?: string;
  q?: string;
}

export interface CreateCaseInput {
  caseType: string;
  description: string;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export async function listCases(
  params: ListCasesParams = {},
): Promise<ListCasesResponse> {
  const query = new URLSearchParams();
  if (params.page) query.set("page", String(params.page));
  if (params.pageSize) query.set("page_size", String(params.pageSize));
  if (params.caseType) query.set("case_type", params.caseType);
  if (params.q) query.set("q", params.q);

  const qs = query.toString();
  return requestApi<ListCasesResponse>(`/cases${qs ? `?${qs}` : ""}`, {
    headers: authHeaders(),
  });
}

export async function createCase(input: CreateCaseInput): Promise<Case> {
  return requestApi<Case>("/cases", {
    method: "POST",
    body: JSON.stringify(input),
    headers: authHeaders(),
  });
}

export async function getCase(caseId: string): Promise<CaseDetail> {
  return requestApi<CaseDetail>(`/cases/${encodeURIComponent(caseId)}`, {
    headers: authHeaders(),
  });
}

export async function addEvidence(
  caseId: string,
  file: File,
): Promise<Evidence> {
  const form = new FormData();
  form.append("file", file);

  const token = getToken();
  const response = await fetch(
    `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/evidences`,
    {
      method: "POST",
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: form,
    },
  );

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error ?? "Could not upload evidence");
  }

  return (await response.json()) as Evidence;
}

export async function getEvidenceObjectUrl(
  caseId: string,
  evidenceId: number,
): Promise<string> {
  const token = getToken();
  const response = await fetch(
    `${apiBaseUrl}/cases/${encodeURIComponent(caseId)}/evidences/${evidenceId}/download`,
    {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    },
  );

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };
    throw new Error(body.error ?? "Could not load evidence");
  }

  const blob = await response.blob();
  return URL.createObjectURL(blob);
}
