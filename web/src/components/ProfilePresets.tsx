import { useRef, useState } from "react";
import { api, errorMessage } from "../api";
import {
  useTableSelection,
  tableSelectionKey,
} from "../hooks/useTableSelection";
import { TableSelector } from "./TableSelector";
import { Alert, Button, Empty, Field, Modal } from "./ui";
import type { Job, Profile, TablePreset } from "../types";
export function ProfilePresets({
  profile,
  active,
  onJob,
  reload,
  onClose,
}: {
  profile: Profile;
  active: boolean;
  onJob: (job: Job) => void;
  reload: () => Promise<void>;
  onClose: () => void;
}) {
  const [database, setDatabase] = useState(
    profile.database || profile.table_presets?.[0]?.database || "",
  );
  const [name, setName] = useState("");
  const [presets, setPresets] = useState(profile.table_presets || []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const selection = useTableSelection(
    { profile_id: profile.id, database, ssl: profile.ssl },
    onJob,
  );
  const persist = async (items: TablePreset[]) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await api.updateProfile(profile.id, { table_presets: items });
      setPresets(items);
      await reload();
      setNotice("Seleções salvas neste perfil.");
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  };
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!database.trim() || !name.trim() || !selection.selection?.length) {
      setError(
        "Informe o banco, o nome da seleção e marque ao menos uma tabela.",
      );
      return;
    }
    const entry: TablePreset = {
      name: name.trim(),
      database,
      tables: selection.selection,
    };
    const others = presets.filter(
      (preset) =>
        preset.name.toLocaleLowerCase("pt-BR") !==
        entry.name.toLocaleLowerCase("pt-BR"),
    );
    if (others.length >= 20) {
      setError(
        "Este perfil já possui 20 seleções. Remova uma seleção antes de salvar outra.",
      );
      return;
    }
    await persist([...others, entry]);
  };
  return (
    <Modal
      title={`Seleções de tabelas · ${profile.name}`}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <p>
        Seleções nomeadas ficam salvas no perfil local e podem ser reutilizadas
        em backups e restaurações.
      </p>
      <div className="preset-list">
        {presets.length ? (
          presets.map((preset) => (
            <div className="preset-row" key={preset.name}>
              <div>
                <strong>{preset.name}</strong>
                <small>
                  {preset.database} · {preset.tables.length} tabelas
                </small>
              </div>
              <Button
                variant="secondary"
                disabled={busy || selection.loading}
                onClick={() => {
                  selection.applyTo(profile.id, preset.database, preset.tables);
                  setDatabase(preset.database);
                  setName(preset.name);
                  setError("");
                }}
              >
                Editar seleção
              </Button>
              <Button
                variant="danger"
                disabled={busy}
                aria-label={`Remover seleção ${preset.name}`}
                onClick={() =>
                  void persist(
                    presets.filter((item) => item.name !== preset.name),
                  )
                }
              >
                Remover
              </Button>
            </div>
          ))
        ) : (
          <Empty title="Nenhuma seleção salva neste perfil" />
        )}
      </div>
      <form onSubmit={save}>
        <div className="form-grid">
          <Field
            label="Banco de origem da seleção"
            hint="O banco padrão do perfil é opcional. Informe aqui o banco que deseja consultar."
          >
            <input
              ref={input}
              required
              value={database}
              onChange={(event) => {
                setDatabase(event.target.value);
                setNotice("");
              }}
            />
          </Field>
          <Field
            label="Nome da seleção"
            hint="Salvar substitui a seleção com o mesmo nome."
          >
            <input
              required
              maxLength={64}
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Ex.: Financeiro essencial"
            />
          </Field>
        </div>
        <TableSelector
          controller={selection}
          scope={tableSelectionKey(profile.id, database)}
          active={active}
          canQuery={!!database.trim()}
          missingReason="Informe o banco de origem da seleção acima para consultar."
          onInvalidQuery={() => input.current?.focus()}
          initialExpanded
        />
        {selection.selection === null && (
          <p className="table-hint">
            Para salvar uma seleção nomeada, marque todas do catálogo ou escolha
            tabelas específicas.
          </p>
        )}
        {error && <Alert>{error}</Alert>}
        {notice && <Alert success>{notice}</Alert>}
        <div className="modal-footer">
          <Button
            variant="secondary"
            type="button"
            disabled={busy}
            onClick={onClose}
          >
            Fechar
          </Button>
          <Button
            type="submit"
            busy={busy}
            disabled={selection.loading || !selection.selection?.length}
          >
            Salvar seleção no perfil
          </Button>
        </div>
      </form>
    </Modal>
  );
}
