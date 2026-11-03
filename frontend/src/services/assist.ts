import { apiBaseUrl } from "./api";
import { getToken } from "./auth";

// The assist lab's API (api/assist_handler.go): extract text from an uploaded file and draft a
// registration from the documents, using a local model. Nothing is stored and nothing is written
// to the database -- it is a manual-testing surface for the drafting pipeline.

export interface AssistInfo {
  enabled: boolean;
  model: string;
}

export interface AssistDocument {
  name: string;
  sha256: string;
  text: string;
}

export interface AssistUsage {
  promptTokens: number;
  completionTokens: number;
  reasoningTokens?: number;
  durationMs: number;
}

export interface AssistDraft {
  request: unknown;
  unsure?: string[];
  reasoning?: string;
  usage?: AssistUsage;
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function getAssistInfo(): Promise<AssistInfo> {
  return fetch(`${apiBaseUrl}/assist`, { headers: authHeaders() }).then(async (r) => {
    if (!r.ok) throw new Error("The request could not be completed");
    return (await r.json()) as AssistInfo;
  });
}

export async function extractDocument(file: File): Promise<AssistDocument> {
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${apiBaseUrl}/assist/extract`, { method: "POST", headers: authHeaders(), body: form });
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(body.error ?? `Could not read ${file.name}`);
  }
  return (await response.json()) as AssistDocument;
}

export async function draftDocuments(documents: AssistDocument[], instructions: string, signal?: AbortSignal): Promise<AssistDraft> {
  const response = await fetch(`${apiBaseUrl}/assist/draft`, {
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
