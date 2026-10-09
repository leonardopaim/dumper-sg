import { useEffect, useState } from "react";
import {
  Copy,
  Eye,
  FolderOpen,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import { api, errorMessage } from "../api";
import {
  Alert,
  Button,
  Empty,
  Field,
  Modal,
  PageHeader,
  dateTime,
} from "../components/ui";
import { sizeLabel } from "../components/TableSelector";
import type { BackupEntry, BackupTableInfo } from "../types";

export function Backups({
  backups,
  reload,
  active,
  onRestore,
}: {
  backups: BackupEntry[];
  reload: () => Promise<void>;
  active: boolean;
  onRestore: (path: string) => void;
}) {
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState("recent");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [details, setDetails] = useState<BackupEntry>();
  const [deleting, setDeleting] = useState<BackupEntry>();
  const [tables, setTables] = useState<BackupTableInfo[]>([]);
  const [tableError, setTableError] = useState("");
  const [loadingTables, setLoadingTables] = useState(false);
  useEffect(() => {
    setTables([]);
    setTableError("");
    setLoadingTables(false);
    if (!details?.complete) return;
    const controller = new AbortController();
    setLoadingTables(true);
    void api
      .backupTables(details.path, controller.signal)
      .then((items) => {
        if (!controller.signal.aborted) setTables(items);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setTableError(errorMessage(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingTables(false);
      });
    return () => controller.abort();
  }, [details]);
  const visible = backups
    .filter((item) =>
      `${item.name} ${item.path}`.toLowerCase().includes(search.toLowerCase()),
    )
    .sort((a, b) =>
      sort === "size"
        ? (b.size_bytes || 0) - (a.size_bytes || 0)
        : b.modified_at.localeCompare(a.modified_at),
    );
  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const remove = () =>
    void run(async () => {
      if (!deleting?.id) return;
      await api.deleteBackup(deleting.id);
      setDeleting(undefined);
      setNotice(
        "Backup excluído do computador. O histórico da operação foi preservado.",
      );
      await reload();
    });
  const copy = (path: string) =>
    void run(async () => {
      await navigator.clipboard.writeText(path);
      setNotice("Caminho copiado.");
    });
  return (
    <>
      <PageHeader
        eyebrow="ARQUIVOS LOCAIS"
        title="Meus backups"
        description="Encontre, restaure e gerencie seus backups."
        action={
          <Button
            variant="secondary"
            busy={busy}
            onClick={() => void run(reload)}
          >
            <RefreshCw size={16} />
            Atualizar
          </Button>
        }
      />
      {error && !details && !deleting && <Alert>{error}</Alert>}
      {notice && !details && !deleting && <Alert success>{notice}</Alert>}
      <section className="card history-card backup-library">
        <div className="backup-library-toolbar">
          <Field label="Buscar backup">
            <span className="search-input">
              <Search size={16} />
              <input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Nome ou caminho"
              />
            </span>
          </Field>
          <Field label="Ordenar backups">
            <select value={sort} onChange={(e) => setSort(e.target.value)}>
              <option value="recent">Mais recentes</option>
              <option value="size">Maior tamanho</option>
            </select>
          </Field>
          <div className="backup-library-summary">
            <strong>
              {backups.length} backups ·{" "}
              {sizeLabel(
                backups.reduce((sum, item) => sum + (item.size_bytes || 0), 0),
              )}
            </strong>
            <small>Diretório padrão e destinos no histórico</small>
          </div>
        </div>
        {active && (
          <p className="backup-library-hint">
            A exclusão fica indisponível enquanto uma operação estiver em
            execução.
          </p>
        )}
        {visible.length ? (
          <div className="table-scroll">
            <table>
              <caption className="sr-only">
                Backups armazenados no computador
              </caption>
              <thead>
                <tr>
                  <th scope="col">Backup</th>
                  <th scope="col">Modificado em</th>
                  <th scope="col">Tamanho</th>
                  <th scope="col">Estado</th>
                  <th scope="col">Ações</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((item) => (
                  <tr key={item.path}>
                    <td>
                      <strong className="backup-name" title={item.name}>
                        {item.name}
                      </strong>
                    </td>
                    <td className="nowrap">{dateTime(item.modified_at)}</td>
                    <td className="nowrap">
                      {item.problem ? "≈ " : ""}
                      {sizeLabel(item.size_bytes || 0)}
                    </td>
                    <td>
                      <span
                        className={`badge ${item.complete ? "succeeded" : "failed"}`}
                      >
                        {item.complete ? "Finalizado" : "Incompleto"}
                      </span>
                    </td>
                    <td>
                      <div className="backup-actions">
                        <button
                          className="icon-button"
                          type="button"
                          title="Ver detalhes"
                          aria-label={`Ver detalhes de ${item.name}`}
                          onClick={() => {
                            setError("");
                            setNotice("");
                            setDetails(item);
                          }}
                        >
                          <Eye size={17} />
                        </button>
                        <button
                          className="icon-button"
                          type="button"
                          title="Restaurar"
                          aria-label={`Restaurar ${item.name}`}
                          disabled={!item.complete || active}
                          onClick={() => onRestore(item.path)}
                        >
                          <RotateCcw size={17} />
                        </button>
                        <button
                          className="icon-button"
                          type="button"
                          title="Abrir pasta"
                          aria-label={`Abrir pasta de ${item.name}`}
                          disabled={busy || !item.id}
                          onClick={() =>
                            void run(() => api.openBackup(item.id!))
                          }
                        >
                          <FolderOpen size={17} />
                        </button>
                        <button
                          className="icon-button danger-icon"
                          type="button"
                          title="Excluir backup"
                          aria-label={`Excluir ${item.name}`}
                          disabled={busy || active || !item.id}
                          onClick={() => {
                            setError("");
                            setDeleting(item);
                          }}
                        >
                          <Trash2 size={17} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            title={
              backups.length
                ? "Nenhum backup corresponde à busca"
                : "Nenhum backup encontrado"
            }
          >
            Crie um backup ou confira o diretório padrão nas configurações.
          </Empty>
        )}
      </section>
      {details && (
        <Modal title="Detalhes do backup" onClose={() => setDetails(undefined)}>
          <div className="backup-details">
            {error && <Alert>{error}</Alert>}
            {notice && <Alert success>{notice}</Alert>}
            <strong>{details.name}</strong>
            <code>{details.path}</code>
            <p>
              {sizeLabel(details.size_bytes || 0)} · {details.file_count ?? "—"}{" "}
              arquivos · {dateTime(details.modified_at)}
            </p>
            {details.problem && <Alert>{details.problem}</Alert>}
            {!details.complete && (
              <Alert>
                O backup não tem a marca de conclusão no metadata. Confira os
                logs da operação antes de utilizá-lo.
              </Alert>
            )}
            <div className="backup-detail-actions">
              <Button
                variant="secondary"
                disabled={busy || !details.id}
                onClick={() => void run(() => api.openBackup(details.id!))}
              >
                <FolderOpen size={16} />
                Abrir pasta
              </Button>
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() => copy(details.path)}
              >
                <Copy size={16} />
                Copiar caminho
              </Button>
              <Button
                disabled={!details.complete || active}
                onClick={() => onRestore(details.path)}
              >
                <RotateCcw size={16} />
                Restaurar este backup
              </Button>
            </div>
            {loadingTables && <p role="status">Lendo tabelas do backup…</p>}
            {tableError && <Alert>{tableError}</Alert>}
            {!loadingTables && details.complete && !tableError && (
              <>
                <h3>{tables.length} tabelas / views</h3>
                <div className="backup-tables table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th scope="col">Tabela</th>
                        <th scope="col">Tipo</th>
                        <th scope="col">Tamanho</th>
                      </tr>
                    </thead>
                    <tbody>
                      {tables.map((table) => (
                        <tr key={`${table.database}.${table.name}`}>
                          <td>
                            {table.database}.{table.name}
                          </td>
                          <td>
                            {table.table_type === "VIEW" ? "View" : "Tabela"}
                          </td>
                          <td>{sizeLabel(table.size_bytes)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            )}
          </div>
        </Modal>
      )}
      {deleting && (
        <Modal
          title="Excluir backup?"
          onClose={() => {
            if (!busy) setDeleting(undefined);
          }}
        >
          <div className="backup-details">
            <strong>{deleting.name}</strong>
            <code>{deleting.path}</code>
            <p>
              {sizeLabel(deleting.size_bytes || 0)} ·{" "}
              {deleting.file_count ?? "—"} arquivos
            </p>
            <p>
              A pasta inteira e seus arquivos serão excluídos permanentemente do
              computador. Esta ação não pode ser desfeita.
            </p>
            {error && <Alert>{error}</Alert>}
            <div className="modal-actions">
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() => setDeleting(undefined)}
              >
                Cancelar
              </Button>
              <Button
                variant="danger"
                busy={busy}
                disabled={active}
                onClick={remove}
              >
                <Trash2 size={16} />
                Excluir permanentemente
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </>
  );
}
