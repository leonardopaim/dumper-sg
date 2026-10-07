import type {
  Profile,
  ProfileInput,
  Job,
  JobEvent,
  BackupEntry,
  Diagnostics,
  Settings,
  BackupInput,
  RestoreInput,
} from "./types";
const base = "/api/v1";
let tokenPromise: Promise<string> | undefined;
export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "APIError";
  }
}
async function read<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new APIError(
      data?.error || `Falha na API local (HTTP ${response.status}).`,
      response.status,
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
async function token(): Promise<string> {
  if (!tokenPromise)
    tokenPromise = fetch(`${base}/session`)
      .then(read<{ token: string }>)
      .then((data) => data.token)
      .catch((error) => {
        tokenPromise = undefined;
        throw error;
      });
  return tokenPromise;
}
export async function request<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") headers["X-DumperSG-Token"] = await token();
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(`${base}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 403) tokenPromise = undefined;
  return read<T>(response);
}
export const api = {
  profiles: () => request<Profile[]>("/profiles"),
  createProfile: (data: ProfileInput) =>
    request<Profile>("/profiles", "POST", data),
  updateProfile: (id: number, data: Partial<ProfileInput>) =>
    request<Profile>(`/profiles/${id}`, "PATCH", data),
  deleteProfile: (id: number) => request<void>(`/profiles/${id}`, "DELETE"),
  testConnection: (id: number) => request<Job>(`/profiles/${id}/test`, "POST"),
  backup: (data: BackupInput) => request<Job>("/backups", "POST", data),
  restore: (data: RestoreInput) => request<Job>("/restores", "POST", data),
  createDatabase: (profile_id: number, database: string) =>
    request<Job>("/databases", "POST", { profile_id, database }),
  jobs: () => request<Job[]>("/jobs"),
  history: (limit = 100) => request<Job[]>(`/history?limit=${limit}`),
  job: (id: string) => request<Job>(`/jobs/${encodeURIComponent(id)}`),
  events: (id: string, after = 0) =>
    request<JobEvent[]>(
      `/jobs/${encodeURIComponent(id)}/events?after=${after}`,
    ),
  cancel: (id: string) =>
    request<Job>(`/jobs/${encodeURIComponent(id)}/cancel`, "POST"),
  backups: () => request<BackupEntry[]>("/backups"),
  settings: () => request<Settings>("/settings"),
  saveSettings: (data: Settings) =>
    request<Settings>("/settings", "PATCH", data),
  diagnostics: () => request<Diagnostics>("/diagnostics"),
  health: () => request<{ status: string; version: string }>("/health"),
  importLegacy: (path: string) =>
    request<{ profiles: number; jobs: number }>("/import/legacy", "POST", {
      path,
    }),
};
export function errorMessage(error: unknown): string {
  return error instanceof TypeError
    ? "Não foi possível acessar a API local. Verifique se o DumperSG está em execução."
    : error instanceof Error
      ? error.message
      : "Não foi possível concluir a operação.";
}
