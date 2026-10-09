import { useOperationRequest } from "../components/OperationFeedback";
import { useEffect, useRef, useState } from "react";
import { Archive, Database, RefreshCw, Search } from "lucide-react";
import { api, errorMessage } from "../api";
import { Alert, Button, Empty, Field, PageHeader } from "../components/ui";
import type {
  BackupSource,
  DatabaseCatalog,
  DatabaseInfo,
  Job,
  Profile,
} from "../types";

const searchable = (value: string) =>
  value
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLocaleLowerCase("pt-BR")
    .replace(/\s+/g, " ")
    .trim();

type SearchMode = "auto" | "exact" | "contains";
function matchesDatabase(row: DatabaseInfo, query: string, mode: SearchMode) {
  if (!query) return true;
  if (mode === "auto" && /^sommusgestor_\d+$/.test(query))
    return searchable(row.name) === query;
  if (mode === "auto" && /^\d+$/.test(query))
    return (
      String(row.group_id) === query ||
      searchable(row.name) === `sommusgestor_${query}`
    );
  const values = [
    row.name,
    row.group_name || "",
    String(row.group_id || ""),
    ...(row.companies || []).flatMap((company) => [
      company.legal_name,
      company.trade_name,
    ]),
  ].map(searchable);
  return mode === "exact"
    ? values.some((value) => value === query)
    : query
        .split(" ")
        .every((term) => values.some((value) => value.includes(term)));
}

export function Databases({
  profiles,
  active,
  onJob,
  onBackup,
  onProfiles,
}: {
  profiles: Profile[];
  active: boolean;
  onJob: (job: Job) => void;
  onBackup: (source: BackupSource) => void;
  onProfiles: () => void;
}) {
  const runOperation = useOperationRequest();
  const [profileId, setProfileId] = useState(profiles[0]?.id || 0);
  const [filter, setFilter] = useState("");
  const [searchMode, setSearchMode] = useState<SearchMode>("auto");
  const [catalog, setCatalog] = useState<{
    profileId: number;
    result: DatabaseCatalog;
  }>();
  const [pending, setPending] = useState<number>();
  const [failure, setFailure] = useState<{
    profileId: number;
    message: string;
  }>();
  const request = useRef<AbortController | null>(null);
  const currentProfile = useRef(profileId);
  currentProfile.current = profileId;
  const profile = profiles.find((item) => item.id === profileId);
  const result = catalog?.profileId === profileId ? catalog.result : undefined;
  const busy = pending === profileId;
  const rows = result?.databases
    .filter((row) => matchesDatabase(row, searchable(filter), searchMode))
    .sort(
      (a, b) =>
        (a.group_name || a.name).localeCompare(
          b.group_name || b.name,
          "pt-BR",
        ) || a.name.localeCompare(b.name),
    );

  useEffect(() => {
    if (!profiles.some((item) => item.id === profileId))
      setProfileId(profiles[0]?.id || 0);
  }, [profiles, profileId]);

  useEffect(() => {
    setFilter("");
    setCatalog(undefined);
    setPending(undefined);
    setFailure(undefined);
    return () => {
      request.current?.abort();
      request.current = null;
    };
  }, [profileId]);

  const load = async () => {
    if (!profile || active || busy) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setPending(profileId);
    setFailure(undefined);
    setCatalog(undefined);
    try {
      const result = await runOperation("database_list", () =>
        api.databases(profileId, {
          signal: controller.signal,
          onJob: (job) => {
            if (
              !controller.signal.aborted &&
              currentProfile.current === profileId
            )
              onJob(job);
          },
        }),
      );
      if (!controller.signal.aborted && currentProfile.current === profileId)
        setCatalog({ profileId, result });
    } catch (error) {
      if (!controller.signal.aborted && currentProfile.current === profileId)
        setFailure({ profileId, message: errorMessage(error) });
    } finally {
      if (request.current === controller) {
        request.current = null;
        setPending(undefined);
      }
    }
  };

  return (
    <>
      <PageHeader
        eyebrow="CATÁLOGO DE ORIGENS"
        title="Bancos disponíveis"
        description="Encontre o banco e prepare seu backup."
      />
      {profiles.length === 0 ? (
        <Empty title="Nenhum perfil de conexão">
          <Button onClick={onProfiles}>Configurar um perfil</Button>
        </Empty>
      ) : (
        <section className="card database-library">
          <div className="database-toolbar">
            <Field label="Perfil de conexão">
              <select
                value={profileId}
                onChange={(event) => setProfileId(Number(event.target.value))}
              >
                {profiles.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name} · {item.host}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Buscar grupo, empresa ou database">
              <span className="search-input">
                <Search size={16} />
                <input
                  type="search"
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                  placeholder="Grupo, razão social, nome fantasia, ID ou database"
                />
              </span>
            </Field>
            <details className="search-options">
              <summary>Filtro de pesquisa</summary>
              <Field label="Tipo de pesquisa">
                <select
                  value={searchMode}
                  onChange={(event) =>
                    setSearchMode(event.target.value as SearchMode)
                  }
                >
                  <option value="auto">Automática</option>
                  <option value="exact">Igual (exata)</option>
                  <option value="contains">Contém (parcial)</option>
                </select>
              </Field>
            </details>
            <Button
              variant="secondary"
              busy={busy}
              disabled={active || !profile}
              onClick={() => void load()}
            >
              <RefreshCw size={16} />
              {result ? "Atualizar bancos" : "Carregar bancos"}
            </Button>
          </div>
          {failure?.profileId === profileId && (
            <div className="database-feedback">
              <Alert>{failure.message}</Alert>
            </div>
          )}
          {result?.warning && (
            <p className="database-feedback muted" role="status">
              {result.warning}
            </p>
          )}
          {active && !busy && (
            <p className="database-feedback muted">
              Aguarde o término da operação para consultar ou preparar outro
              backup.
            </p>
          )}
          {busy ? (
            <Empty title="Consultando bancos, grupos e empresas…" />
          ) : result ? (
            <>
              <div className="table-scroll">
                <table aria-label="Bancos disponíveis para backup">
                  <thead>
                    <tr>
                      <th>Grupo</th>
                      <th>Empresas</th>
                      <th>Database</th>
                      <th>Ação</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows?.map((row) => (
                      <tr key={row.name}>
                        <td>
                          <strong>
                            {row.group_name || "Sem grupo identificado"}
                          </strong>
                          {row.group_id && <small>Grupo {row.group_id}</small>}
                        </td>
                        <td>
                          {row.companies?.length ? (
                            row.companies.map((company) => (
                              <div
                                className="database-company"
                                key={company.company_id}
                              >
                                <strong>{company.legal_name}</strong>
                                {company.trade_name &&
                                  company.trade_name !== company.legal_name && (
                                    <small>{company.trade_name}</small>
                                  )}
                              </div>
                            ))
                          ) : (
                            <span className="muted">
                              Sem empresa identificada
                            </span>
                          )}
                        </td>
                        <td>
                          <code>{row.name}</code>
                        </td>
                        <td>
                          <Button
                            variant="secondary"
                            disabled={active || !profile}
                            aria-label={`Preparar backup de ${row.group_name || row.name}`}
                            onClick={() =>
                              onBackup({
                                profile_id: profileId,
                                database: row.name,
                              })
                            }
                          >
                            <Archive size={14} />
                            Preparar backup
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {rows?.length === 0 && (
                <Empty
                  title={
                    result.databases.length
                      ? "Nenhum banco corresponde à busca"
                      : "Nenhum banco disponível para este perfil"
                  }
                />
              )}
              <div className="table-footer">
                <small>
                  {rows?.length} de {result.databases.length} bancos
                </small>
              </div>
            </>
          ) : (
            !busy && (
              <Empty title="Carregue os bancos do perfil selecionado">
                <Database size={20} /> A lista mostra os databases disponíveis
                para esta conexão.
              </Empty>
            )
          )}
        </section>
      )}
    </>
  );
}
