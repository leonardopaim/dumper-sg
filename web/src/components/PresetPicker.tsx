import { Field } from "./ui";
import type { Profile, TablePreset } from "../types";
export function PresetPicker({
  profiles,
  onApply,
  disabled = false,
  hint,
}: {
  profiles: Profile[];
  onApply: (profile: Profile, preset: TablePreset) => void;
  disabled?: boolean;
  hint?: string;
}) {
  const sources = profiles.filter((profile) => profile.table_presets?.length);
  return (
    <Field
      label="Usar seleção salva de um perfil"
      hint={
        hint ||
        (sources.length
          ? "Escolha uma seleção nomeada para aplicar os nomes de origem."
          : "Crie seleções em Perfis de banco → Seleções de tabelas.")
      }
    >
      <select
        disabled={disabled || !sources.length}
        value=""
        onChange={(event) => {
          const [id, index] = event.target.value.split(":").map(Number);
          const source = profiles.find((profile) => profile.id === id);
          const preset = source?.table_presets?.[index];
          if (source && preset) onApply(source, preset);
        }}
      >
        <option value="">
          {sources.length
            ? "Aplicar uma seleção salva…"
            : "Nenhuma seleção salva nos perfis"}
        </option>
        {sources.map((profile) => (
          <optgroup label={profile.name} key={profile.id}>
            {profile.table_presets!.map((preset, index) => (
              <option
                key={`${preset.name}:${index}`}
                value={`${profile.id}:${index}`}
              >
                {preset.name} · {preset.database} · {preset.tables.length}{" "}
                tabelas
              </option>
            ))}
          </optgroup>
        ))}
      </select>
    </Field>
  );
}
