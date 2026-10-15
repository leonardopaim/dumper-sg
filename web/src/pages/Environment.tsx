import { useCallback, useEffect, useState } from "react";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { api, errorMessage } from "../api";
import { Alert, Button, PageHeader, dateTime } from "../components/ui";
import type { EnvironmentReport } from "../types";

export function Environment({ onBack }: { onBack: () => void }) {
  const [report, setReport] = useState<EnvironmentReport>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const load = useCallback(async (signal?: AbortSignal) => {
    setBusy(true);
    setError("");
    try {
      const next = await api.environment(signal);
      if (!signal?.aborted) setReport(next);
    } catch (e) {
      if (!signal?.aborted) setError(errorMessage(e));
    } finally {
      if (!signal?.aborted) setBusy(false);
    }
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);
  const docker = report?.docker;
  return (
    <>
      <PageHeader
        eyebrow="DIAGNÓSTICO"
        title="Ambiente da aplicação"
        description="Confira o ambiente selecionado para executar as próximas operações."
        action={
          <div className="inline-actions">
            <Button variant="secondary" onClick={onBack}>
              <ArrowLeft size={16} />
              Configurações
            </Button>
            <Button variant="secondary" busy={busy} onClick={() => void load()}>
              <RefreshCw size={16} />
              Atualizar ambiente
            </Button>
          </div>
        }
      />
      {error && (
        <Alert>
          {error}
          {report && " Os dados abaixo são da última consulta concluída."}
        </Alert>
      )}
      {busy && (
        <p role="status">
          Verificando Docker, imagens e acesso aos diretórios…
        </p>
      )}
      {report && docker && (
        <div className="environment-layout" aria-busy={busy}>
          <p className="muted">Consultado em {dateTime(report.checked_at)}</p>
          <section className="card form-card">
            <div className="card-heading">
              <h2>Execução e Docker</h2>
              <span
                className={`badge ${docker.available ? "succeeded" : "failed"}`}
              >
                {docker.available ? "Disponível" : "Indisponível"}
              </span>
            </div>
            <p>{docker.message}</p>
            <dl className="environment-details">
              <div>
                <dt>Servidor da aplicação</dt>
                <dd>{report.api_address}</dd>
              </div>
              <div>
                <dt>Sistema do core</dt>
                <dd>
                  {report.platform} / {report.architecture}
                </dd>
              </div>
              <div>
                <dt>Transporte Docker</dt>
                <dd>
                  {docker.discovery_pending
                    ? "Detecção automática pendente"
                    : docker.runtime === "wsl"
                      ? `WSL · ${docker.distribution || "Não identificada"}`
                      : "CLI nativa"}
                </dd>
              </div>
              <div>
                <dt>Engine e versão</dt>
                <dd>
                  {docker.engine || "Não identificado"} ·{" "}
                  {docker.version || "Não verificada"}
                </dd>
              </div>
              <div>
                <dt>Daemon selecionado</dt>
                <dd>{docker.daemon_name || "Não identificado"}</dd>
              </div>
              <div>
                <dt>Containers</dt>
                <dd>{docker.container_os || "Não verificado"}</dd>
              </div>
              <div className="full">
                <dt>Rede usada pelos clientes</dt>
                <dd>{docker.network}</dd>
              </div>
              <div className="full">
                <dt>Montagem do diretório de backups</dt>
                <dd>
                  {docker.backup_mount_accessible
                    ? docker.backup_mount
                    : "Não validada"}
                </dd>
              </div>
            </dl>
            <details className="advanced-options">
              <summary>Detalhes do ambiente</summary>
              <dl className="environment-details">
                <div>
                  <dt>Runtime Go</dt>
                  <dd>{report.go_version}</dd>
                </div>
                <div>
                  <dt>Sistema do daemon</dt>
                  <dd>{docker.operating_system || "Não identificado"}</dd>
                </div>
                <div className="full">
                  <dt>Identificação do daemon</dt>
                  <dd>{docker.daemon_id || "Não identificada"}</dd>
                </div>
                <div className="full">
                  <dt>Banco de dados da aplicação</dt>
                  <dd>{report.database_path}</dd>
                </div>
              </dl>
            </details>
          </section>
          <section className="card form-card">
            <h2>Imagens utilizadas</h2>
            <div className="environment-images">
              {docker.images.map((image) => (
                <div key={image.name}>
                  <strong>{image.name}</strong>
                  <span
                    className={`badge ${image.present ? "succeeded" : "partial"}`}
                  >
                    {image.present
                      ? "Disponível"
                      : docker.available
                        ? "Verificar no primeiro uso"
                        : "Não verificada"}
                  </span>
                  <p>{image.message}</p>
                  {image.id && (
                    <details>
                      <summary>Identificação da imagem</summary>
                      <code>{image.id}</code>
                    </details>
                  )}
                </div>
              ))}
            </div>
          </section>
          <section className="card form-card">
            <h2>Armazenamento e permissões</h2>
            <div className="environment-directories">
              {report.directories.map((directory) => (
                <div key={directory.name}>
                  <strong>{directory.name}</strong>
                  <code>{directory.path}</code>
                  <span>
                    Leitura:{" "}
                    {directory.readable ? "Disponível" : "Indisponível"} ·
                    Escrita:{" "}
                    {directory.writable ? "Disponível" : "Indisponível"}
                  </span>
                  {directory.message && (
                    <Alert warning>{directory.message}</Alert>
                  )}
                </div>
              ))}
            </div>
          </section>
        </div>
      )}
    </>
  );
}
