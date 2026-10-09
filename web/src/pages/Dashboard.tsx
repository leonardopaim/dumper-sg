import { Archive, ArrowRight, Database, RotateCcw } from "lucide-react";
import {
  Alert,
  Badge,
  Button,
  Empty,
  PageHeader,
  dateTime,
  kinds,
} from "../components/ui";
import type { Diagnostics, Job, Profile } from "../types";
export function Dashboard({
  profiles,
  history,
  diagnostics,
  onNavigate,
  onSelect,
}: {
  profiles: Profile[];
  history: Job[];
  diagnostics?: Diagnostics;
  onNavigate: (page: string) => void;
  onSelect: (job: Job) => void;
}) {
  return (
    <>
      <PageHeader
        eyebrow=""
        title="Visão geral"
        description="Crie um backup ou restaure seus dados."
      />
      {diagnostics && !diagnostics.available && (
        <Alert>
          Docker indisponível.{" "}
          <Button variant="ghost" onClick={() => onNavigate("settings")}>
            Verificar ambiente
          </Button>
        </Alert>
      )}
      <div className="dashboard-actions">
        {profiles.length ? (
          <>
            <Button onClick={() => onNavigate("backup")}>
              <Archive size={17} /> Criar backup
            </Button>
            <Button variant="secondary" onClick={() => onNavigate("restore")}>
              <RotateCcw size={17} /> Restaurar
            </Button>
          </>
        ) : (
          <Button onClick={() => onNavigate("profiles")}>
            Adicionar primeiro perfil
          </Button>
        )}
      </div>
      <section className="card recent-card">
        <div className="section-header">
          <div>
            <h2>Atividade recente</h2>
          </div>
          <Button variant="ghost" onClick={() => onNavigate("history")}>
            Ver histórico
            <ArrowRight size={16} />
          </Button>
        </div>
        {history.length ? (
          <div className="activity-list">
            {history.slice(0, 3).map((job) => (
              <button
                className="activity-row"
                key={job.id}
                onClick={() => onSelect(job)}
              >
                <span className="activity-icon">
                  {job.kind === "restore" ? (
                    <RotateCcw size={18} />
                  ) : job.kind === "backup" ? (
                    <Archive size={18} />
                  ) : (
                    <Database size={18} />
                  )}
                </span>
                <span className="activity-info">
                  <strong>
                    {kinds[job.kind]} · {job.profile_name}
                  </strong>
                  <small>
                    {job.database || "Conexão"} · {dateTime(job.started_at)}
                  </small>
                </span>
                <Badge
                  status={job.status}
                  partial={job.partial_result}
                  warnings={!!job.warning_count}
                />
                <ArrowRight size={16} />
              </button>
            ))}
          </div>
        ) : (
          <Empty title="Nenhuma operação por enquanto">
            Crie um perfil e inicie seu primeiro backup. Os resultados
            aparecerão aqui.
          </Empty>
        )}
      </section>
    </>
  );
}
