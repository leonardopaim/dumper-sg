import { act, renderHook } from "@testing-library/react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { useTableSelection, tableSelectionKey } from "./useTableSelection";
import type { TableInfo, TableQuery } from "../types";
const query: TableQuery = { profile_id: 1, database: "origem", ssl: false };
const rows: TableInfo[] = [
  { name: "maior", size_bytes: 2048, rows: 10, table_type: "BASE TABLE" },
];
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());
describe("seleção por perfil e banco", () => {
  it("restaura seleção validada, não mistura bancos e mantém modo automático sem consulta", () => {
    const tables = vi.spyOn(api, "tables");
    const key = tableSelectionKey(1, "origem");
    localStorage.setItem(
      key,
      JSON.stringify(["nome,especial", "nome.com.ponto", "nome,especial"]),
    );
    const view = renderHook(({ data }) => useTableSelection(data, vi.fn()), {
      initialProps: { data: query },
    });
    expect(view.result.current.selection).toEqual([
      "nome,especial",
      "nome.com.ponto",
    ]);
    view.rerender({ data: { ...query, database: "outro" } });
    expect(view.result.current.selection).toBeNull();
    act(() => view.result.current.select([]));
    expect(view.result.current.selection).toEqual([]);
    view.rerender({ data: query });
    expect(view.result.current.selection).toEqual([
      "nome,especial",
      "nome.com.ponto",
    ]);
    act(() => view.result.current.select(null));
    expect(localStorage.getItem(key)).toBeNull();
    expect(tables).not.toHaveBeenCalled();
  });
  it("descarta dados inválidos da seleção salva", () => {
    localStorage.setItem(
      tableSelectionKey(1, "origem"),
      JSON.stringify(["maior", 9]),
    );
    const { result } = renderHook(() => useTableSelection(query, vi.fn()));
    expect(result.current.selection).toBeNull();
  });
  it("invalida o catálogo ao alterar SSL e mantém nomes da mesma origem", async () => {
    vi.spyOn(api, "tables").mockResolvedValue(rows);
    const view = renderHook(({ data }) => useTableSelection(data, vi.fn()), {
      initialProps: { data: query },
    });
    await act(() => view.result.current.load());
    act(() => view.result.current.select(["maior"]));
    expect(view.result.current.tables).toEqual(rows);
    view.rerender({ data: { ...query, ssl: true } });
    expect(view.result.current.tables).toBeNull();
    expect(view.result.current.selection).toEqual(["maior"]);
  });
  it("aborta a consulta anterior e ignora resposta tardia de outro banco", async () => {
    let resolveOld!: (rows: TableInfo[]) => void;
    const tables = vi
      .spyOn(api, "tables")
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve;
          }),
      )
      .mockResolvedValueOnce([{ ...rows[0], name: "nova" }]);
    const onJob = vi.fn();
    const view = renderHook(({ data }) => useTableSelection(data, onJob), {
      initialProps: { data: query },
    });
    act(() => {
      void view.result.current.load();
    });
    const oldSignal = tables.mock.calls[0][1]!.signal!;
    view.rerender({ data: { ...query, database: "novo" } });
    expect(oldSignal.aborted).toBe(true);
    await act(() => view.result.current.load());
    await act(async () => {
      resolveOld(rows);
    });
    expect(view.result.current.tables?.map((table) => table.name)).toEqual([
      "nova",
    ]);
    expect(view.result.current.selection).toBeNull();
  });
  it("volta ao banco anterior sem deixar uma consulta abortada presa em loading", async () => {
    let resolveQuery!: (tables: TableInfo[]) => void;
    vi.spyOn(api, "tables").mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveQuery = resolve;
        }),
    );
    const view = renderHook(({ data }) => useTableSelection(data, vi.fn()), {
      initialProps: { data: query },
    });
    act(() => {
      void view.result.current.load();
    });
    expect(view.result.current.loading).toBe(true);
    view.rerender({ data: { ...query, database: "outro" } });
    view.rerender({ data: query });
    expect(view.result.current.loading).toBe(false);
    expect(view.result.current.tables).toBeNull();
    await act(async () => {
      resolveQuery(rows);
    });
    expect(view.result.current.tables).toBeNull();
  });
});
