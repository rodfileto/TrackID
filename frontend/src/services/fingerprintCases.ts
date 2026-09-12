import { getToken } from "./auth";

const apiBaseUrl = import.meta.env.VITE_API_URL ?? "/api";

export type FingerprintCase = {
  sourceDataset: string;
  sourceRow: number;
  caseId: string;
  description: string;
  responsibleUser?: string;
  comparisonType?: string;
  relatedReference?: string;
  relatedReferenceKind?: string;
};

export type FingerprintCasePage = {
  items: FingerprintCase[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
};

export async function listFingerprintCases(params: {
  page: number;
  pageSize: number;
  caseId?: string;
  query?: string;
  comparisonType?: string;
  reference?: string;
}): Promise<FingerprintCasePage> {
  const search = new URLSearchParams({ page: String(params.page), page_size: String(params.pageSize) });
  if (params.caseId) search.set("case_id", params.caseId);
  if (params.query) search.set("q", params.query);
  if (params.comparisonType) search.set("comparison_type", params.comparisonType);
  if (params.reference) search.set("reference", params.reference);
  const response = await fetch(`${apiBaseUrl}/toolkit/fingerprint-cases?${search}`, {
    headers: { Authorization: `Bearer ${getToken() ?? ""}` },
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error ?? "Não foi possível carregar os casos criminais");
  return body as FingerprintCasePage;
}
