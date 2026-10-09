import { useEffect, useId, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronUp, ListFilter, RefreshCw } from "lucide-react";
import { Alert, Button, Empty, Field } from "./ui";
import type { ReactNode } from "react";
import type { TableInfo } from "../types";
export interface SelectableTable extends TableInfo {
  key?: string;
  database?: string;
}
export interface TableSelectorController {
  selection: string[] | null;
  tables: SelectableTable[] | null;
  loading: boolean;
  error: string;
  select: (names: string[] | null) => void;
  load: () => Promise<void>;
  cancel: () => void;
}
const identity = (table: SelectableTable) => table.key ?? table.name;
const displayName = (table: SelectableTable) =>
  table.database ? `${table.database} / ${table.name}` : table.name;
export function sizeLabel(bytes: number) {
  if (!bytes) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const power = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  );
  return `${(bytes / 1024 ** power).toLocaleString("pt-BR", { maximumFractionDigits: power ? 1 : 0 })} ${units[power]}`;
}
export function TableSelector({
  controller,
  active,
  scope,
  canQuery,
  ignoreRegex = "",
  mode = "backup",
  missingReason,
  onInvalidQuery,
  extra,
  initialExpanded = false,
}: {
  controller: TableSelectorController;
  active: boolean;
  scope: string;
  canQuery: boolean;
  ignoreRegex?: string;
  mode?: "backup" | "restore";
  missingReason?: string;
  onInvalidQuery?: () => void;
  extra?: ReactNode;
  initialExpanded?: boolean;
}) {
  const [expanded, setExpanded] = useState(initialExpanded);
  const panelId = useId();
  const [search, setSearch] = useState("");
  const [customScope, setCustomScope] = useState<string>();
  const allCheckbox = useRef<HTMLInputElement>(null);
  const { selection, tables, loading, error, select, load, cancel } =
    controller;
  useEffect(() => setSearch(""), [scope]);
  const visible =
    tables?.filter((table) =>
      displayName(table)
        .toLocaleLowerCase("pt-BR")
        .includes(search.toLocaleLowerCase("pt-BR")),
    ) || [];
  const selectedNames = useMemo(
    () => (selection === null ? null : new Set(selection)),
    [selection],
  );
  const catalogNames = useMemo(() => new Set(tables?.map(identity)), [tables]);
  const visibleNames = new Set(visible.map(identity));
  const chosen =
    tables?.filter(
      (table) => selectedNames === null || selectedNames.has(identity(table)),
    ) || [];
  const total = chosen.reduce((sum, table) => sum + table.size_bytes, 0);
  const customMode = selection !== null || customScope === scope;
  const allChecked = !!tables?.length && chosen.length === tables.length;
  useEffect(() => {
    if (allCheckbox.current)
      allCheckbox.current.indeterminate = chosen.length > 0 && !allChecked;
  }, [chosen.length, allChecked, expanded]);
  useEffect(() => {
    if (customScope === undefined) return;
    if (customScope !== scope || selection !== null) {
      setCustomScope(undefined);
    } else if (tables !== null) {
      setCustomScope(undefined);
      select(tables.map(identity));
    }
  }, [customScope, scope, selection, tables, select]);
  const missing =
    tables && selection
      ? selection.filter((name) => !catalogNames.has(name))
      : [];
  const toggle = (name: string) => {
    const names = selection ?? tables?.map(identity) ?? [];
    select(
      names.includes(name)
        ? names.filter((item) => item !== name)
        : [...names, name],
    );
  };
  return (
    <section
      className="table-selector"
      aria-label={`Seleção de tabelas para ${mode === "restore" ? "restauração" : "backup"}`}
    >
      <button
        className="table-selector-heading"
        type="button"
        aria-expanded={expanded}
        aria-controls={panelId}
        onClick={() => setExpanded(!expanded)}
      >
        <ListFilter size={17} />
        <strong>
          Selecionar tabelas <small>Opcional</small>
        </strong>
        <span>
          {selection === null
            ? "Todas as tabelas"
            : `${selection.length} selecionadas`}
        </span>
        {expanded ? <ChevronUp size={17} /> : <ChevronDown size={17} />}
      </button>
      {expanded && (
        <div id={panelId} className="table-selector-body">
          <div className="table-mode-row">
            <label>
              <input
                type="radio"
                name={`table-mode-${panelId}`}
                checked={!customMode}
                onChange={() => {
                  setCustomScope(undefined);
                  select(null);
                }}
              />{" "}
              {mode === "restore"
                ? "Todas as tabelas do backup"
                : "Todas as tabelas (automático)"}
            </label>
            <label>
              <input
                type="radio"
                name={`table-mode-${panelId}`}
                checked={customMode}
                onChange={() => {
                  if (tables !== null) select(tables.map(identity));
                  else {
                    setCustomScope(scope);
                    select(null);
                  }
                }}
              />{" "}
              Seleção personalizada
            </label>
            <Button
              type="button"
              variant="secondary"
              busy={loading}
              disabled={active}
              onClick={() => {
                if (!canQuery) onInvalidQuery?.();
                void load();
              }}
            >
              <RefreshCw size={14} />
              {tables ? "Atualizar tabelas" : "Consultar tabelas"}
            </Button>
            {loading && (
              <Button type="button" variant="danger" onClick={cancel}>
                Cancelar consulta
              </Button>
            )}
          </div>
          {active && (
            <p className="table-hint" role="status">
              Aguarde a operação em andamento para consultar as tabelas.
            </p>
          )}
          {!canQuery && missingReason && (
            <p className="table-hint">{missingReason}</p>
          )}
          <p className="table-hint">
            {mode === "restore"
              ? "Consulte os arquivos do backup para escolher as tabelas. O catálogo não acessa o banco de destino."
              : "O modo automático inclui todas as tabelas, inclusive novas, sem consultar o catálogo. A seleção personalizada guarda apenas os nomes escolhidos."}
          </p>
          {extra}
          {error && <Alert>{error}</Alert>}
          {tables && (
            <>
              <div className="table-actions">
                <Field label="Buscar tabelas">
                  <input
                    type="search"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder="Filtrar pelo nome"
                  />
                </Field>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => select(tables.map(identity))}
                >
                  Marcar todas do catálogo
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => select([])}
                >
                  Desmarcar todas
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={!visible.length}
                  onClick={() => select(visible.map(identity))}
                >
                  Selecionar somente visíveis
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={!visible.length}
                  onClick={() =>
                    select(
                      (selection ?? tables.map(identity)).filter(
                        (name) => !visibleNames.has(name),
                      ),
                    )
                  }
                >
                  Desmarcar visíveis
                </Button>
              </div>
              <div
                className="table-catalog"
                tabIndex={0}
                aria-label="Catálogo de tabelas por tamanho"
              >
                <table>
                  <caption className="sr-only">
                    {mode === "restore"
                      ? "Tabelas em ordem decrescente do tamanho dos arquivos"
                      : "Tabelas em ordem decrescente de tamanho estimado"}
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col">
                        <label className="table-name">
                          <input
                            ref={allCheckbox}
                            type="checkbox"
                            aria-label="Marcar todas as tabelas do catálogo"
                            checked={allChecked}
                            disabled={!tables.length}
                            onChange={(event) =>
                              select(
                                event.target.checked
                                  ? tables.map(identity)
                                  : [],
                              )
                            }
                          />
                          <span>Tabela</span>
                        </label>
                      </th>
                      <th scope="col">
                        {mode === "restore"
                          ? "Tamanho nos arquivos"
                          : "Tamanho estimado"}
                      </th>
                      <th scope="col">Linhas estimadas</th>
                      <th scope="col">Tipo</th>
                    </tr>
                  </thead>
                  <tbody>
                    {visible.map((table) => (
                      <tr key={identity(table)}>
                        <td>
                          <label className="table-name">
                            <input
                              type="checkbox"
                              aria-label={`Incluir ${displayName(table)}`}
                              checked={
                                selection === null ||
                                selectedNames!.has(identity(table))
                              }
                              onChange={() => toggle(identity(table))}
                            />
                            <span>{displayName(table)}</span>
                          </label>
                        </td>
                        <td>{sizeLabel(table.size_bytes)}</td>
                        <td>{table.rows.toLocaleString("pt-BR")}</td>
                        <td>
                          <span
                            className={`table-type ${table.table_type.toUpperCase() === "VIEW" ? "view" : ""}`}
                          >
                            {table.table_type.toUpperCase() === "VIEW"
                              ? "View"
                              : "Tabela"}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!visible.length && (
                  <Empty
                    title={
                      tables.length
                        ? "Nenhuma tabela corresponde à busca"
                        : mode === "restore"
                          ? "Este backup não retornou tabelas"
                          : "Este banco não retornou tabelas"
                    }
                  />
                )}
              </div>
              <div className="table-selection-summary" role="status">
                <strong>
                  {selection === null ? tables.length : selection.length}{" "}
                  selecionadas · {sizeLabel(total)}{" "}
                  {mode === "restore" ? "nos arquivos" : "estimados"}
                </strong>
                <span>
                  {visible.length} de {tables.length} visíveis ·{" "}
                  {mode === "restore"
                    ? "tamanho dos arquivos do backup"
                    : "dados + índices (DATA_LENGTH + INDEX_LENGTH)"}
                </span>
              </div>
              {missing.length > 0 && (
                <p className="table-hint">
                  {missing.length} nomes da seleção salva não foram encontrados
                  neste catálogo. Confira a seleção antes de{" "}
                  {mode === "restore" ? "restaurar" : "criar o backup"}.
                </p>
              )}
            </>
          )}
          {!tables && !loading && (
            <p className="table-hint">
              {mode === "restore"
                ? "Consulte as tabelas para escolher por origem e nome e ver o tamanho dos arquivos."
                : "Consulte as tabelas para escolher por nome e ver os tamanhos estimados."}
              {selection !== null &&
                ` Há ${selection.length} nomes na seleção salva.`}
            </p>
          )}
          {selection?.length === 0 && (
            <Alert>
              Nenhuma tabela selecionada. Marque ao menos uma tabela ou use o
              modo automático.
            </Alert>
          )}
          <p className="table-hint">
            {mode === "restore"
              ? "Tabelas relacionadas (FK) não são incluídas automaticamente. A seleção identifica cada banco e nome de origem."
              : ignoreRegex
                ? "Tabelas relacionadas (FK) não são incluídas automaticamente. O filtro de exclusão informado também será aplicado: tabelas selecionadas que coincidirem com a expressão serão ignoradas."
                : "Tabelas relacionadas (FK) não são incluídas automaticamente. O filtro de exclusão por expressão pode reduzir a seleção. O tamanho é estimado antes desse filtro e da compressão."}
          </p>
        </div>
      )}
    </section>
  );
}
