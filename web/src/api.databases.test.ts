import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import type { DatabaseCatalog, Job } from "./types";

const job: Job = {
  id: "databases",
  kind: "database_list",
  status: "running",
  profile_id: 2,
  profile_name: "Origem",
  database: "",
  path: "",
  started_at: "2026-10-09T12:00:00Z",
  progress: 0,
  message: "Consultando",
};
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("catálogo de bancos pela API", () => {
  it("aguarda a conclusão antes de obter os bancos e nomes dos grupos", async () => {
    vi.useFakeTimers();
    const start = vi.spyOn(api, "startDatabaseList").mockResolvedValue(job);
    vi.spyOn(api, "job").mockResolvedValue({ ...job, status: "succeeded" });
    const result = vi.spyOn(api, "databaseResult").mockResolvedValue({
      databases: [
        { name: "sommusgestor_12", group_id: 12, group_name: "São José" },
      ],
    });
    const promise = api.databases(2);
    expect(result).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(500);
    expect((await promise).databases[0].group_name).toBe("São José");
    expect(start).toHaveBeenCalledWith(2);
    expect(result).toHaveBeenCalledWith(job.id, expect.any(AbortSignal));
  });

  it("rejeita nomes duplicados e dados malformados", async () => {
    vi.spyOn(api, "startDatabaseList").mockResolvedValue({
      ...job,
      status: "succeeded",
    });
    const result = vi.spyOn(api, "databaseResult");
    for (const databases of [
      [{ name: "a" }, { name: "a" }],
      [{ name: "a\n" }],
      [{ name: "a", group_id: -1 }],
    ]) {
      result.mockResolvedValue({ databases });
      await expect(api.databases(2)).rejects.toThrow(
        "catálogo de bancos inválido",
      );
    }
  });

  it("rejeita empresas com dados inválidos, IDs repetidos ou associadas ao grupo errado", async () => {
    vi.spyOn(api, "startDatabaseList").mockResolvedValue({
      ...job,
      status: "succeeded",
    });
    const company = {
      company_id: 1,
      group_id: 12,
      legal_name: "Empresa Ltda",
      trade_name: "Loja",
    };
    const result = vi.spyOn(api, "databaseResult");
    result.mockResolvedValue({
      databases: [
        { name: "sommusgestor_12", group_id: 12, companies: [company] },
      ],
    });
    expect(
      (await api.databases(2)).databases[0].companies?.[0].trade_name,
    ).toBe("Loja");
    for (const companies of [
      [{ ...company, group_id: 10 }],
      [{ ...company, legal_name: "" }],
      [company, company],
      {},
      [null],
    ]) {
      result.mockResolvedValue({
        databases: [{ name: "sommusgestor_12", group_id: 12, companies }],
      } as unknown as DatabaseCatalog);
      await expect(api.databases(2)).rejects.toThrow(
        "catálogo de bancos inválido",
      );
    }
  });
});
