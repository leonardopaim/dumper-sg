import { act, renderHook } from "@testing-library/react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { useRestoreTables, referenceKey } from "./useRestoreTables";
import type { BackupTableInfo } from "../types";
const rows: BackupTableInfo[] = [
  {
    database: "a.b",
    name: "c",
    size_bytes: 100,
    rows: 1,
    table_type: "BASE TABLE",
  },
  {
    database: "a",
    name: "b.c",
    size_bytes: 50,
    rows: 1,
    table_type: "BASE TABLE",
  },
];
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());
describe("catálogo local da restauração", () => {
  it("mantém identidade banco/nome sem colisões e persiste seleção por perfil e diretório", async () => {
    vi.spyOn(api, "backupTables").mockResolvedValue(rows);
    let view = renderHook(({ id }) => useRestoreTables(id, "/backup"), {
      initialProps: { id: 1 },
    });
    await act(() => view.result.current.load());
    expect(view.result.current.tables?.map((table) => table.key)).toEqual(
      rows.map(referenceKey),
    );
    act(() => view.result.current.select([referenceKey(rows[1])]));
    expect(view.result.current.references).toEqual([
      { database: "a", name: "b.c" },
    ]);
    view.rerender({ id: 2 });
    expect(view.result.current.selection).toBeNull();
    expect(view.result.current.tables).toHaveLength(2);
    view.unmount();
    view = renderHook(({ id }) => useRestoreTables(id, "/backup"), {
      initialProps: { id: 1 },
    });
    expect(view.result.current.references).toEqual([
      { database: "a", name: "b.c" },
    ]);
  });
  it("aborta ao trocar o backup e ignora sua resposta tardia", async () => {
    let resolveOld!: (rows: BackupTableInfo[]) => void;
    const query = vi
      .spyOn(api, "backupTables")
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve;
          }),
      )
      .mockResolvedValueOnce([{ ...rows[0], name: "nova" }]);
    const view = renderHook(({ dir }) => useRestoreTables(1, dir), {
      initialProps: { dir: "/antigo" },
    });
    act(() => {
      void view.result.current.load();
    });
    const signal = query.mock.calls[0][1]!;
    view.rerender({ dir: "/novo" });
    expect(signal.aborted).toBe(true);
    await act(() => view.result.current.load());
    await act(async () => resolveOld(rows));
    expect(view.result.current.tables?.[0].name).toBe("nova");
    expect(view.result.current.selection).toBeNull();
  });
  it("cancela somente o fetch do catálogo, sem cancelar jobs de operações", async () => {
    let resolveRows!: (rows: BackupTableInfo[]) => void;
    const query = vi.spyOn(api, "backupTables").mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveRows = resolve;
        }),
    );
    const cancel = vi.spyOn(api, "cancel");
    const { result } = renderHook(() => useRestoreTables(1, "/backup"));
    act(() => {
      void result.current.load();
    });
    act(() => result.current.cancel());
    expect(query.mock.calls[0][1]!.aborted).toBe(true);
    await act(async () => resolveRows(rows));
    expect(result.current.tables).toBeNull();
    expect(result.current.loading).toBe(false);
    expect(cancel).not.toHaveBeenCalled();
  });
});
