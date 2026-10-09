import { useOperationRequest } from "../components/OperationFeedback";
import { useEffect, useRef, useState } from "react";
import { api, errorMessage, tableLimit } from "../api";
import type { Job, TableInfo, TableQuery } from "../types";

export const tableSelectionKey = (profileId: number, database: string) =>
  `dumpersg.tableSelection:${profileId}:${encodeURIComponent(database)}`;
function storedSelection(key: string): string[] | null {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(key) || "null");
    if (
      !Array.isArray(value) ||
      value.length > tableLimit ||
      value.some(
        (name) =>
          typeof name !== "string" ||
          !name ||
          name.length > 256 ||
          /[\x00-\x1f\x7f]/.test(name),
      )
    )
      return null;
    return [...new Set(value as string[])];
  } catch {
    return null;
  }
}
export function useTableSelection(
  query: TableQuery,
  onJob: (job: Job) => void,
) {
  const runOperation = useOperationRequest();
  const scope = tableSelectionKey(query.profile_id, query.database);
  const context = `${scope}:${query.ssl}`;
  const currentContext = useRef(context);
  currentContext.current = context;
  const request = useRef<AbortController | null>(null);
  const [saved, setSaved] = useState(() => ({
    scope,
    value: storedSelection(scope),
  }));
  const [catalog, setCatalog] = useState<{
    context: string;
    tables: TableInfo[];
  }>();
  const [pending, setPending] = useState<string>();
  const [failure, setFailure] = useState<{
    context: string;
    message: string;
  }>();
  const selection =
    saved.scope === scope ? saved.value : storedSelection(scope);
  const tables = catalog?.context === context ? catalog.tables : null;
  const loading = pending === context;
  useEffect(() => {
    setPending(undefined);
    setCatalog(undefined);
    setFailure(undefined);
    setSaved((previous) =>
      previous.scope === scope
        ? previous
        : { scope, value: storedSelection(scope) },
    );
    return () => {
      request.current?.abort();
      request.current = null;
    };
  }, [context, scope]);
  const selectAt = (targetScope: string, names: string[] | null) => {
    const value = names === null ? null : [...new Set(names)];
    if (
      value &&
      (value.length > tableLimit ||
        value.some(
          (name) => !name || name.length > 256 || /[\x00-\x1f\x7f]/.test(name),
        ))
    ) {
      setFailure({
        context,
        message:
          "A seleção contém nomes inválidos ou excede o limite de tabelas. Ajuste a seleção ou use o modo automático.",
      });
      return;
    }
    setSaved({ scope: targetScope, value });
    try {
      if (value === null) localStorage.removeItem(targetScope);
      else localStorage.setItem(targetScope, JSON.stringify(value));
    } catch {
      /* A seleção em memória permanece disponível. */
    }
  };
  const load = async () => {
    if (!query.profile_id || !query.database.trim()) {
      setFailure({
        context,
        message:
          "Selecione um perfil e informe o banco para consultar as tabelas.",
      });
      return;
    }
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setPending(context);
    setFailure(undefined);
    try {
      const result = await runOperation("table_list", () =>
        api.tables(query, {
          signal: controller.signal,
          onJob: (job) => {
            if (
              !controller.signal.aborted &&
              currentContext.current === context
            )
              onJob(job);
          },
        }),
      );
      if (
        !controller.signal.aborted &&
        currentContext.current === context &&
        request.current === controller
      ) {
        setCatalog({
          context,
          tables: [...result].sort(
            (a, b) =>
              b.size_bytes - a.size_bytes ||
              a.name.localeCompare(b.name, "pt-BR"),
          ),
        });
      }
    } catch (error) {
      if (
        !controller.signal.aborted &&
        currentContext.current === context &&
        request.current === controller
      )
        setFailure({ context, message: errorMessage(error) });
    } finally {
      if (request.current === controller) {
        setPending(undefined);
        request.current = null;
      }
    }
  };
  return {
    selection,
    tables,
    loading,
    error: failure?.context === context ? failure.message : "",
    select: (names: string[] | null) => selectAt(scope, names),
    applyTo: (profileId: number, database: string, names: string[]) =>
      selectAt(tableSelectionKey(profileId, database), names),
    load,
    cancel: () => request.current?.abort(),
  };
}
export type TableSelectionController = ReturnType<typeof useTableSelection>;
