import { useState } from "react";
import { ArrowUpRight, RefreshCw, Search } from "lucide-react";
import {
  Alert,
  Badge,
  Button,
  Empty,
  Field,
  PageHeader,
  dateTime,
  kinds,
} from "../components/ui";
import { errorMessage } from "../api";
import type { Job } from "../types";
export function History({
  history,
  reload,
  onSelect,
  selectedId,
}: {
  history: Job[];
  reload: () => Promise<void>;
  onSelect: (job: Job) => void;
  selectedId?: string;
}) {
  const [filter, setFilter] = useState("");
  const [status, setStatus] = useState("");
  const [kind, setKind] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const filtered = history.filter(
    (job) =>
      (!status || job.status === status) &&
      (!kind || job.kind === kind) &&
      `${job.profile_name} ${job.database} ${job.path}`
        .toLowerCase()
        .includes(filter.toLowerCase()),
  );
  const refresh = async () => {
    setBusy(true);
    setError("");
    try {
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <PageHeader
        eyebrow="RASTREABILIDADE"
        title="Histórico de operações"
        description="Consulte os últimos 100 resultados, inclusive depois de reiniciar o aplicativo."
        action={
          <Button
            variant="secondary"
            busy={busy}
            onClick={() => void refresh()}
          >
            <RefreshCw size={16} />
            Atualizar
          </Button>
        }
      />
      {error && <Alert>{error}</Alert>}
      <section className="card history-card">
        <div className="filter-row">
          <Field label="Buscar operação">
            <span className="search-input">
              <Search size={16} />
              <input
                type="search"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="Perfil, banco ou caminho"
              />
            </span>
          </Field>
          <Field label="Tipo">
            <select value={kind} onChange={(e) => setKind(e.target.value)}>
              <option value="">Todas as operações</option>
              {Object.entries(kinds).map(([key, value]) => (
                <option key={key} value={key}>
                  {value}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Status">
            <select value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="">Todos os status</option>
              <option value="succeeded">Concluído</option>
              <option value="failed">Falhou</option>
              <option value="cancelled">Cancelado</option>
              <option value="running">Em execução</option>
              <option value="cancel_requested">Cancelando</option>
            </select>
          </Field>
        </div>
        {filtered.length ? (
          <div className="table-scroll">
            <table>
              <caption className="sr-only">
                Operações salvas no histórico
              </caption>
              <thead>
                <tr>
                  <th scope="col">Operação</th>
                  <th scope="col">Perfil / banco</th>
                  <th scope="col">Início</th>
                  <th scope="col">Status</th>
                  <th scope="col">
                    <span className="sr-only">Detalhes</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((job) => (
                  <tr
                    key={job.id}
                    className={selectedId === job.id ? "selected-row" : ""}
                  >
                    <td>
                      <strong>{kinds[job.kind]}</strong>
                    </td>
                    <td>
                      <strong>{job.profile_name}</strong>
                      <small>{job.database || "Conexão"}</small>
                    </td>
                    <td className="nowrap">{dateTime(job.started_at)}</td>
                    <td>
                      <Badge
                        status={job.status}
                        partial={job.partial_result}
                        warnings={!!job.warning_count}
                      />
                    </td>
                    <td>
                      <button
                        className="icon-button"
                        aria-label={`Ver detalhes de ${kinds[job.kind]} em ${job.profile_name}, ${dateTime(job.started_at)}`}
                        onClick={() => onSelect(job)}
                      >
                        <ArrowUpRight size={19} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            title={
              history.length
                ? "Nenhuma operação corresponde aos filtros"
                : "Seu histórico está vazio"
            }
          >
            {history.length
              ? "Ajuste os filtros para ver outros resultados."
              : "As operações realizadas ficam registradas automaticamente."}
          </Empty>
        )}
        <div className="table-footer">
          <span>
            {filtered.length} de {history.length} operações
          </span>
          <small>
            Selecione uma linha para consultar os eventos disponíveis.
          </small>
        </div>
      </section>
    </>
  );
}
