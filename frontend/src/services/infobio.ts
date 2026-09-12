import { getToken } from "./auth";

const apiBaseUrl = import.meta.env.VITE_API_URL ?? "/api";

export type SearchMode = "nome" | "nif" | "rin";

export type InfoBioEntry = Record<string, unknown>;

export type InfoBioSearchResponse = {
  search_type: string;
  resultado?: InfoBioEntry;
  resultados?: InfoBioEntry[];
  total_resultados?: number;
  [key: string]: unknown;
};

export type InfoBioAuthError = Error & {
  code?: string;
  expired?: boolean;
  status?: number;
};

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken();
  const response = await fetch(`${apiBaseUrl}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  });

  const body = (await response.json().catch(() => ({}))) as Record<string, unknown>;
  if (!response.ok) {
    const error = new Error(
      typeof body.detail === "string"
        ? body.detail
        : typeof body.error === "string"
          ? body.error
          : "A solicitação não foi concluída",
    ) as InfoBioAuthError;
    error.code = typeof body.code === "string" ? body.code : undefined;
    error.expired = typeof body.expired === "boolean" ? body.expired : undefined;
    error.status = response.status;
    throw error;
  }
  return body as T;
}

export async function getInfoBioSessionStatus(): Promise<{ active: boolean }> {
  return request<{ active: boolean }>("/infobio/auth");
}

export async function createInfoBioSession(password: string): Promise<{ active: boolean }> {
  return request<{ active: boolean }>("/infobio/auth", {
    method: "POST",
    body: JSON.stringify({ password }),
  });
}

export async function invalidateInfoBioSession(): Promise<{ active: boolean }> {
  return request<{ active: boolean }>("/infobio/auth", { method: "DELETE" });
}

export async function searchInfoBio(params: {
  mode: SearchMode;
  value: string;
  nomeMae?: string;
  nomePai?: string;
  dataNascimento?: string;
}): Promise<InfoBioSearchResponse> {
  const payload =
    params.mode === "nome"
      ? {
          nome: params.value,
          nome_mae: params.nomeMae || undefined,
          nome_pai: params.nomePai || undefined,
          data_nascimento: params.dataNascimento || undefined,
        }
      : { [params.mode]: params.value };
  return request<InfoBioSearchResponse>("/infobio/search", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function getInfoBioDetails(nif: string): Promise<InfoBioEntry> {
  return request<InfoBioEntry>("/infobio/details", {
    method: "POST",
    body: JSON.stringify({ nif }),
  });
}

export function isInfoBioAuthError(error: unknown): error is InfoBioAuthError {
  return (
    typeof error === "object" &&
    error !== null &&
    (error as InfoBioAuthError).code === "infobio_auth_required"
  );
}

export function entryValue(entry: InfoBioEntry, ...keys: string[]): string {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value === "string" && value.trim()) return value;
    if (typeof value === "number") return String(value);
  }
  return "-";
}
