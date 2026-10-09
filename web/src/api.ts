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
  TableInfo,
  TableQuery,
  BackupTableInfo,
  ApplicationStatus,
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
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") headers["X-DumperSG-Token"] = await token();
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(`${base}${path}`, {
    method,
    headers,
    signal,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 403) tokenPromise = undefined;
  return read<T>(response);
}
export const api = {
  application: (signal?: AbortSignal) =>
    request<ApplicationStatus>("/application", "GET", undefined, signal),
  restartApplication: (signal?: AbortSignal) =>
    request<{ status: string; instance_id: string }>(
      "/application/restart",
      "POST",
      {},
      signal,
    ),
  startTableList: (data: TableQuery) => request<Job>("/tables", "POST", data),
  tableResult: (id: string, signal?: AbortSignal) =>
    request<TableInfo[]>(
      `/jobs/${encodeURIComponent(id)}/tables`,
      "GET",
      undefined,
      signal,
    ),
  tables: queryTables,
  backupTables: (backup_dir: string, signal?: AbortSignal) =>
    request<BackupTableInfo[]>(
      "/backups/tables",
      "POST",
      { backup_dir },
      signal,
    ),
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
  job: (id: string, signal?: AbortSignal) =>
    request<Job>(`/jobs/${encodeURIComponent(id)}`, "GET", undefined, signal),
  events: (id: string, after = 0) =>
    request<JobEvent[]>(
      `/jobs/${encodeURIComponent(id)}/events?after=${after}`,
    ),
  cancel: (id: string) =>
    request<Job>(`/jobs/${encodeURIComponent(id)}/cancel`, "POST"),
  backups: () => request<BackupEntry[]>("/backups"),
  deleteBackup: (id: string) =>
    request<void>(`/backups/${encodeURIComponent(id)}`, "DELETE"),
  openBackup: (id: string) =>
    request<void>(`/backups/${encodeURIComponent(id)}/open`, "POST"),
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

export const tableLimit = 10000;
export const tableQueryTimeoutMs = 120000;
interface TableQueryOptions {
  signal?: AbortSignal;
  onJob?: (job: Job) => void;
}
function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  return new Promise((resolve, reject) => {
    const abort = () =>
      reject(new DOMException("Consulta cancelada.", "AbortError"));
    if (signal.aborted) abort();
    else signal.addEventListener("abort", abort, { once: true });
    promise
      .then(resolve, reject)
      .finally(() => signal.removeEventListener("abort", abort));
  });
}
async function queryTables(
  data: TableQuery,
  options: TableQueryOptions = {},
): Promise<TableInfo[]> {
  const controller = new AbortController();
  const externalAbort = () => controller.abort();
  options.signal?.addEventListener("abort", externalAbort, { once: true });
  if (options.signal?.aborted) controller.abort();
  let timedOut = false;
  let created: Job | undefined;
  let complete = false;
  let cancelled = false;
  const cancelOwnJob = () => {
    if (
      !cancelled &&
      created &&
      (created.status === "running" || created.status === "cancel_requested")
    ) {
      cancelled = true;
      void api.cancel(created.id).catch(() => undefined);
    }
  };
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, tableQueryTimeoutMs);
  try {
    controller.signal.throwIfAborted();
    // Keep the creation response observable so a late response can cancel this job only.
    const creation = api.startTableList(data).then((job) => {
      created = job;
      if (controller.signal.aborted) cancelOwnJob();
      return job;
    });
    created = await abortable(creation, controller.signal);
    options.onJob?.(created);
    while (
      created.status === "running" ||
      created.status === "cancel_requested"
    ) {
      await abortable(
        new Promise<void>((resolve) => setTimeout(resolve, 500)),
        controller.signal,
      );
      created = await api.job(created.id, controller.signal);
    }
    if (created.status !== "succeeded")
      throw new Error(
        created.message || "Não foi possível consultar as tabelas.",
      );
    complete = true;
    options.onJob?.(created);
    const tables = await api.tableResult(created.id, controller.signal);
    if (!Array.isArray(tables) || tables.length > tableLimit)
      throw new Error(
        `O catálogo excede o limite de ${tableLimit.toLocaleString("pt-BR")} tabelas. Use todas as tabelas ou consulte um banco menor.`,
      );
    if (
      tables.some(
        (table) =>
          typeof table.name !== "string" ||
          !table.name ||
          table.name.length > 256 ||
          !Number.isFinite(table.size_bytes) ||
          table.size_bytes < 0 ||
          !Number.isFinite(table.rows) ||
          table.rows < 0 ||
          typeof table.table_type !== "string",
      )
    )
      throw new Error("A consulta retornou um catálogo de tabelas inválido.");
    return tables.sort(
      (a, b) =>
        b.size_bytes - a.size_bytes || a.name.localeCompare(b.name, "pt-BR"),
    );
  } catch (error) {
    if (timedOut)
      throw new Error(
        "A consulta de tabelas excedeu 2 minutos. Verifique a conexão e tente novamente.",
      );
    throw error;
  } finally {
    clearTimeout(timer);
    options.signal?.removeEventListener("abort", externalAbort);
    if (!complete) cancelOwnJob();
  }
}
