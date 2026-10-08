import {
  Archive,
  ArrowRight,
  Database,
  CheckCircle2,
  RotateCcw,
  Activity,
} from "lucide-react";
import {
  Badge,
  Button,
  Empty,
  PageHeader,
  dateTime,
  isActive,
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
  const completed = history.filter((job) => job.status === "succeeded").length;
  const running = history.find((job) => isActive(job.status));
  return (
    <>
      <PageHeader
        eyebrow="CONSOLE LOCAL"
        title="Seus dados, protegidos."
        description="Gerencie backups e restaurações MySQL em um único lugar."
      />
      <div className="stats-grid">
        <article className="card stat-card">
          <span className="stat-icon blue">
            <Database size={21} />
          </span>
          <div>
            <small>PERFIS DE CONEXÃO</small>
            <strong>{profiles.length}</strong>
            <span>Bancos configurados</span>
          </div>
        </article>
        <article className="card stat-card">
          <span className="stat-icon green">
            <CheckCircle2 size={21} />
          </span>
          <div>
            <small>OPERAÇÕES CONCLUÍDAS</small>
            <strong>{completed}</strong>
            <span>Nas últimas {history.length} operações</span>
          </div>
        </article>
        <article className="card stat-card">
          <span className="stat-icon purple">
            <Activity size={21} />
          </span>
          <div>
            <small>ESTADO DO CORE</small>
            <strong className="text-stat">
              {running
                ? "Em execução"
                : diagnostics?.available
                  ? "Pronto"
                  : diagnostics
                    ? "Docker ausente"
                    : "Verificando"}
            </strong>
            <span>
              {running
                ? kinds[running.kind]
                : diagnostics?.available
                  ? "Disponível para operar"
                  : "Confira o diagnóstico"}
            </span>
          </div>
        </article>
      </div>
      <section className="hero-card">
        <div>
          <span className="eyebrow">PROTEJA O QUE IMPORTA</span>
          <h2>Seu próximo backup começa aqui.</h2>
          <p>
            Defina a origem, escolha o destino e acompanhe os eventos sem sair
            do console.
          </p>
          <Button
            onClick={() => onNavigate(profiles.length ? "backup" : "profiles")}
          >
            {profiles.length ? "Criar um backup" : "Adicionar primeiro perfil"}
            <ArrowRight size={17} />
          </Button>
        </div>
      </section>
      <div className="quick-grid">
        <button
          className="card quick-card"
          onClick={() => onNavigate("backup")}
        >
          <span className="icon-tile">
            <Archive size={23} />
          </span>
          <span>
            <strong>Novo backup</strong>
            <small>Exporte com compressão e múltiplas threads.</small>
          </span>
          <ArrowRight size={20} />
        </button>
        <button
          className="card quick-card"
          onClick={() => onNavigate("restore")}
        >
          <span className="icon-tile amber">
            <RotateCcw size={23} />
          </span>
          <span>
            <strong>Restaurar dados</strong>
            <small>Recupere um backup em um banco isolado.</small>
          </span>
          <ArrowRight size={20} />
        </button>
      </div>
      <section className="card recent-card">
        <div className="section-header">
          <div>
            <h2>Atividade recente</h2>
            <p>Resultados persistidos pelo core local.</p>
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
                <Badge status={job.status} />
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
