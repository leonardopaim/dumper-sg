import { afterEach, describe, expect, it, vi } from "vitest";
import { api, tableLimit, tableQueryTimeoutMs } from "./api";
import type { Job, TableInfo } from "./types";
const running: Job = {
  id: "table-job",
  kind: "table_list",
  status: "running",
  profile_id: 1,
  profile_name: "Origem",
  database: "origem",
  path: "",
  started_at: "2026-10-08T12:00:00Z",
  progress: 0,
  message: "Consultando",
};
const query = { profile_id: 1, database: "origem", ssl: true };
const rows: TableInfo[] = [
  { name: "pequena", size_bytes: 1, rows: 1, table_type: "BASE TABLE" },
  { name: "nome,com.ponto", size_bytes: 50, rows: 2, table_type: "BASE TABLE" },
];
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});
describe("consulta de tabelas pela API", () => {
  it("consulta o resultado somente após sucesso e retorna ordem decrescente sem usar eventos", async () => {
    vi.useFakeTimers();
    const start = vi.spyOn(api, "startTableList").mockResolvedValue(running);
    vi.spyOn(api, "job")
      .mockResolvedValueOnce(running)
      .mockResolvedValueOnce({ ...running, status: "succeeded" });
    const result = vi.spyOn(api, "tableResult").mockResolvedValue(rows);
    const events = vi.spyOn(api, "events");
    const promise = api.tables(query);
    await vi.advanceTimersByTimeAsync(500);
    expect(result).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(500);
    expect((await promise).map((table) => table.name)).toEqual([
      "nome,com.ponto",
      "pequena",
    ]);
    expect(start).toHaveBeenCalledWith(query);
    expect(result).toHaveBeenCalledWith("table-job", expect.any(AbortSignal));
    expect(events).not.toHaveBeenCalled();
  });
  it("aborta imediatamente e cancela somente o próprio job quando a criação responde tarde", async () => {
    let finishCreation!: (job: Job) => void;
    vi.spyOn(api, "startTableList").mockImplementation(
      () =>
        new Promise((resolve) => {
          finishCreation = resolve;
        }),
    );
    const cancel = vi
      .spyOn(api, "cancel")
      .mockResolvedValue({ ...running, status: "cancelled" });
    const poll = vi.spyOn(api, "job");
    const controller = new AbortController();
    const promise = api
      .tables(query, { signal: controller.signal })
      .catch((error) => error);
    controller.abort();
    expect((await promise).name).toBe("AbortError");
    finishCreation(running);
    await Promise.resolve();
    expect(cancel).toHaveBeenCalledExactlyOnceWith("table-job");
    expect(poll).not.toHaveBeenCalled();
  });
  it("mostra a falha real do job e não busca o catálogo nem cancela um job finalizado", async () => {
    vi.spyOn(api, "startTableList").mockResolvedValue({
      ...running,
      status: "failed",
      message: "Conexão recusada pelo servidor.",
    });
    const result = vi.spyOn(api, "tableResult");
    const cancel = vi.spyOn(api, "cancel");
    await expect(api.tables(query)).rejects.toThrow(
      "Conexão recusada pelo servidor.",
    );
    expect(result).not.toHaveBeenCalled();
    expect(cancel).not.toHaveBeenCalled();
  });
  it("limita o tempo da consulta e cancela seu job ainda em execução", async () => {
    vi.useFakeTimers();
    vi.spyOn(api, "startTableList").mockResolvedValue(running);
    vi.spyOn(api, "job").mockResolvedValue(running);
    const cancel = vi
      .spyOn(api, "cancel")
      .mockResolvedValue({ ...running, status: "cancelled" });
    const promise = api.tables(query).catch((error) => error);
    await vi.advanceTimersByTimeAsync(tableQueryTimeoutMs);
    expect((await promise).message).toMatch(/excedeu 2 minutos/);
    expect(cancel).toHaveBeenCalledExactlyOnceWith("table-job");
  });
  it("rejeita um catálogo acima do limite sem truncar silenciosamente", async () => {
    vi.spyOn(api, "startTableList").mockResolvedValue({
      ...running,
      status: "succeeded",
    });
    vi.spyOn(api, "tableResult").mockResolvedValue(
      Array.from({ length: tableLimit + 1 }, () => rows[0]),
    );
    await expect(api.tables(query)).rejects.toThrow("excede o limite");
  });
});
