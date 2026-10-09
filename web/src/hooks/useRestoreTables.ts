import { useEffect, useRef, useState } from "react";
import { api, errorMessage, tableLimit } from "../api";
import type { BackupTableInfo, TableReference } from "../types";
import type { SelectableTable } from "../components/TableSelector";
export const referenceKey = (reference: TableReference) =>
  JSON.stringify([reference.database, reference.name]);
export const restoreSelectionKey = (profileId: number, backupDir: string) =>
  `dumpersg.restoreTables:${profileId}:${encodeURIComponent(backupDir)}`;
function decode(key: string): TableReference | undefined {
  try {
    const value: unknown = JSON.parse(key);
    if (
      Array.isArray(value) &&
      value.length === 2 &&
      value.every(
        (item) =>
          typeof item === "string" &&
          item.length > 0 &&
          item.length <= 256 &&
          !/[\x00-\x1f\x7f]/.test(item),
      )
    )
      return { database: value[0], name: value[1] };
  } catch {
    /* Ignore invalid stored identifiers. */
  }
}
function read(scope: string): string[] | null {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(scope) || "null");
    if (
      Array.isArray(value) &&
      value.length <= tableLimit &&
      value.every((item) => typeof item === "string" && decode(item))
    )
      return [...new Set(value as string[])];
  } catch {
    /* Storage is optional. */
  }
  return null;
}
export function useRestoreTables(profileId: number, backupDir: string) {
  const scope = restoreSelectionKey(profileId, backupDir);
  const currentDir = useRef(backupDir);
  currentDir.current = backupDir;
  const request = useRef<AbortController | null>(null);
  const [saved, setSaved] = useState(() => ({ scope, value: read(scope) }));
  const [catalog, setCatalog] = useState<{
    directory: string;
    tables: SelectableTable[];
  }>();
  const [pending, setPending] = useState<string>();
  const [failure, setFailure] = useState<{
    directory: string;
    message: string;
  }>();
  const selection = saved.scope === scope ? saved.value : read(scope);
  const tables = catalog?.directory === backupDir ? catalog.tables : null;
  useEffect(() => {
    setSaved((previous) =>
      previous.scope === scope ? previous : { scope, value: read(scope) },
    );
  }, [scope]);
  useEffect(() => {
    setCatalog(undefined);
    setPending(undefined);
    setFailure(undefined);
    return () => {
      request.current?.abort();
      request.current = null;
    };
  }, [backupDir]);
  const select = (names: string[] | null) => {
    const value = names === null ? null : [...new Set(names)];
    if (
      value &&
      (value.length > tableLimit || value.some((key) => !decode(key)))
    ) {
      setFailure({
        directory: backupDir,
        message: "A seleção de tabelas do backup é inválida.",
      });
      return;
    }
    setSaved({ scope, value });
    try {
      if (value === null) localStorage.removeItem(scope);
      else localStorage.setItem(scope, JSON.stringify(value));
    } catch {
      /* Keep the in-memory selection. */
    }
  };
  const load = async () => {
    if (!backupDir.trim()) {
      setFailure({
        directory: backupDir,
        message:
          "Informe o diretório do backup acima para consultar as tabelas.",
      });
      return;
    }
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setPending(backupDir);
    setFailure(undefined);
    try {
      const rows: BackupTableInfo[] = await api.backupTables(
        backupDir,
        controller.signal,
      );
      if (
        !Array.isArray(rows) ||
        rows.length > tableLimit ||
        rows.some(
          (row) =>
            !decode(referenceKey(row)) ||
            !Number.isFinite(row.size_bytes) ||
            row.size_bytes < 0 ||
            !Number.isFinite(row.rows) ||
            row.rows < 0 ||
            typeof row.table_type !== "string",
        )
      )
        throw new Error(
          "O backup retornou um catálogo inválido ou excedeu o limite de tabelas.",
        );
      if (
        !controller.signal.aborted &&
        currentDir.current === backupDir &&
        request.current === controller
      )
        setCatalog({
          directory: backupDir,
          tables: rows
            .map((row) => ({ ...row, key: referenceKey(row) }))
            .sort(
              (a, b) =>
                b.size_bytes - a.size_bytes ||
                a.name.localeCompare(b.name, "pt-BR"),
            ),
        });
    } catch (error) {
      if (
        !controller.signal.aborted &&
        currentDir.current === backupDir &&
        request.current === controller
      )
        setFailure({ directory: backupDir, message: errorMessage(error) });
    } finally {
      if (request.current === controller) {
        request.current = null;
        setPending(undefined);
      }
    }
  };
  return {
    selection,
    references:
      selection === null ? null : selection.map((key) => decode(key)!),
    tables,
    loading: pending === backupDir,
    error: failure?.directory === backupDir ? failure.message : "",
    select,
    load,
    cancel: () => request.current?.abort(),
  };
}
