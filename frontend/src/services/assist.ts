import { apiBaseUrl, apiFetch } from "./api";
import { getToken } from "./auth";

// The assist lab's API (api/assist_handler.go): extract text from an uploaded file and draft a
// registration from the documents, using a local model. Nothing is stored and nothing is written
// to the database -- it is a manual-testing surface for the drafting pipeline.

export interface AssistInfo {
  enabled: boolean;
  model: string;
  /** Whether an OCR model reads images and scanned PDFs. */
  ocr?: boolean;
  /** The reference labels a draft fills (an ePol task), registered by the organization. */
  referenceLabels?: string[];
}

export type AssistKind = "document" | "evidence";

export interface AssistDocument {
  name: string;
  sha256: string;
  text: string;
  contentType?: string;
  /** A document says what is asked; an evidence is material to examine. Empty is a document. */
  kind?: AssistKind;
  /** The text was read from an image by the OCR model. */
  ocr?: boolean;
}

/** A drafted occurrence, as the server writes occurrences.CreateInput (Go's field names). */
export interface DraftedOccurrence {
  Types?: string[];
  Attempted?: boolean | null;
  OccurredFrom?: string;
  Status?: string;
  Description?: string;
  Agency?: string;
  AgencyNumber?: string;
  InquiryNumber?: string;
  OffenderCount?: number | null;
  VictimCount?: number | null;
  Place?: { Lon?: number; Lat?: number; Precision?: string; Name?: string; Address?: string; Neighbourhood?: string; PlaceType?: string };
}

/** A drafted request (requests.RegisterInput). */
export interface DraftedRequest {
  authority?: string;
  documentReference?: string;
  documentDate?: string;
  receivedOn?: string;
  dueOn?: string;
  citedReference?: string;
  citedInquiryNumber?: string;
  notes?: string;
  questions?: string[] | null;
  references?: { label: string; number: string }[] | null;
  Occurrence?: DraftedOccurrence | null;
  CaseType?: string;
  Cases?: { task?: string; description?: string }[] | null;
}

export interface AssistUsage {
  promptTokens: number;
  completionTokens: number;
  reasoningTokens?: number;
  durationMs: number;
}

export interface AssistDraft {
  request: DraftedRequest;
  unsure?: string[];
  reasoning?: string;
  usage?: AssistUsage;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function getAssistInfo(): Promise<AssistInfo> {
  return apiFetch(`${apiBaseUrl}/assist`, { headers: authHeaders() }).then(async (r) => {
    if (!r.ok) throw new Error("The request could not be completed");
    return (await r.json()) as AssistInfo;
  });
}

export async function extractDocument(file: File): Promise<AssistDocument> {
  const form = new FormData();
  form.append("file", file);
  const response = await apiFetch(`${apiBaseUrl}/assist/extract`, { method: "POST", headers: authHeaders(), body: form });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? `Could not read ${file.name}`);
  }
  return (await response.json()) as AssistDocument;
}

export async function draftDocuments(documents: AssistDocument[], instructions: string, signal?: AbortSignal): Promise<AssistDraft> {
  const response = await apiFetch(`${apiBaseUrl}/assist/draft`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ documents, instructions }),
    signal,
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? "The request could not be completed");
  }
  return (await response.json()) as AssistDraft;
}
