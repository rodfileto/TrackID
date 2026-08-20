export interface ConceptDefinition {
  name: string;
  description: string;
}

export interface ConceptRelation {
  source: string;
  target: string;
}

export interface TargetTypeTemplate {
  name: string;
  description: string;
  source: "builtin" | "user-defined";
  created_by: string | null;
  concepts: ConceptDefinition[];
  relations: ConceptRelation[];
}

export interface TemplateSummary {
  name: string;
  description: string;
  source: "builtin" | "user-defined";
  concept_count: number;
}

export interface TemplateListResponse {
  items: TemplateSummary[];
}

async function parseErrorDetail(response: Response): Promise<string> {
  const detail = await response.json().catch(() => null);
  if (Array.isArray(detail?.detail)) {
    return detail.detail.map((d: { msg?: string }) => d.msg).join("; ");
  }
  return detail?.detail || `Request failed with status ${response.status}`;
}

export async function fetchTemplateList(): Promise<TemplateListResponse> {
  const response = await fetch("/api/v1/ontology/templates");
  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }
  return response.json();
}

export async function fetchTemplate(name: string): Promise<TargetTypeTemplate> {
  const response = await fetch(`/api/v1/ontology/templates/${encodeURIComponent(name)}`);
  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }
  return response.json();
}

export interface CreateTemplatePayload {
  name: string;
  description: string;
  concepts: ConceptDefinition[];
  relations: ConceptRelation[];
}

export async function createTemplate(
  payload: CreateTemplatePayload,
): Promise<TargetTypeTemplate> {
  const response = await fetch("/api/v1/ontology/templates", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }
  return response.json();
}

export async function updateTemplate(
  name: string,
  payload: CreateTemplatePayload,
): Promise<TargetTypeTemplate> {
  const response = await fetch(`/api/v1/ontology/templates/${encodeURIComponent(name)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await parseErrorDetail(response));
  }
  return response.json();
}
