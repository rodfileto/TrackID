export const apiBaseUrl = import.meta.env.VITE_API_URL ?? "/api";

export interface ErrorResponse {
  error?: string;
}

export async function requestApi<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
    ...options,
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ErrorResponse;
    throw new Error(body.error ?? "The request could not be completed");
  }

  return (await response.json()) as T;
}
