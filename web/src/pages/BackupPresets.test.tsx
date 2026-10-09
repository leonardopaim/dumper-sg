import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Backup } from "./Backup";
import { api } from "../api";
import { backupDraftKey } from "../hooks/useOperationDraft";
import { tableSelectionKey } from "../hooks/useTableSelection";
import type { Job, Profile } from "../types";
const profile: Profile = {
  id: 1,
  name: "Origem",
  host: "localhost",
  port: 3306,
  user: "root",
  database: "",
  ssl: false,
  threads: 8,
  has_password: true,
};
const job: Job = {
  id: "backup1",
  kind: "backup",
  status: "running",
  profile_id: 1,
  profile_name: "Origem",
  database: "origem",
  path: "",
  started_at: "2026-10-08T12:00:00Z",
  progress: 0,
  message: "",
};
const props = {
  settings: {},
  active: false,
  onJob: vi.fn(),
  onProfiles: vi.fn(),
};
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());
describe("consulta e seleções salvas no backup", () => {
  it("explica o banco vazio, permite consultar para focar/validar e ativa após preencher", async () => {
    const tables = vi.spyOn(api, "tables").mockResolvedValue([]);
    const user = userEvent.setup();
    render(<Backup {...props} profiles={[profile]} />);
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    expect(
      screen.getByText(
        "Informe o banco de origem acima para consultar as tabelas.",
      ),
    ).toBeInTheDocument();
    const button = screen.getByRole("button", { name: "Consultar tabelas" });
    expect(button).toBeEnabled();
    await user.click(button);
    expect(screen.getByLabelText(/Banco de origem/)).toHaveFocus();
    expect(tables).not.toHaveBeenCalled();
    await user.type(screen.getByLabelText(/Banco de origem/), "origem");
    await user.click(button);
    expect(tables).toHaveBeenCalledWith(
      { profile_id: 1, database: "origem", ssl: false },
      expect.any(Object),
    );
  });
  it("recupera draft vazio do banco padrão atualizado sem impedir edição posterior", async () => {
    localStorage.setItem(
      backupDraftKey,
      JSON.stringify({ profile_id: 1, database: "" }),
    );
    const user = userEvent.setup();
    const view = render(<Backup {...props} profiles={[profile]} />);
    expect(screen.getByLabelText(/Banco de origem/)).toHaveValue("");
    view.rerender(
      <Backup {...props} profiles={[{ ...profile, database: "atualizado" }]} />,
    );
    expect(screen.getByLabelText(/Banco de origem/)).toHaveValue("atualizado");
    await user.clear(screen.getByLabelText(/Banco de origem/));
    await user.type(screen.getByLabelText(/Banco de origem/), "personalizado");
    expect(screen.getByLabelText(/Banco de origem/)).toHaveValue(
      "personalizado",
    );
  });
  it("explica exclusividade quando outra operação impede a consulta", async () => {
    render(
      <Backup
        {...props}
        active
        profiles={[{ ...profile, database: "origem" }]}
      />,
    );
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: /Selecionar tabelas/ }));
    expect(
      screen.getByRole("button", { name: "Consultar tabelas" }),
    ).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent(
      "Aguarde a operação em andamento",
    );
  });
  it("aplica uma seleção de outro perfil na origem correta e a preserva após recarregar", async () => {
    const source: Profile = {
      ...profile,
      id: 2,
      name: "Remoto",
      database: "padrao",
      ssl: true,
      table_presets: [
        {
          name: "Essencial",
          database: "financeiro",
          tables: ["nome,especial", "nome.com.ponto"],
        },
      ],
    };
    const backup = vi.spyOn(api, "backup").mockResolvedValue(job);
    const user = userEvent.setup();
    let view = render(<Backup {...props} profiles={[profile, source]} />);
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    await user.selectOptions(
      screen.getByLabelText(/Usar seleção salva/),
      "2:0",
    );
    expect(screen.getByLabelText("Perfil de conexão")).toHaveValue("2");
    expect(screen.getByLabelText(/Banco de origem/)).toHaveValue("financeiro");
    expect(localStorage.getItem(tableSelectionKey(2, "financeiro"))).toBe(
      '["nome,especial","nome.com.ponto"]',
    );
    expect(localStorage.getItem(tableSelectionKey(1, ""))).toBeNull();
    view.unmount();
    view = render(<Backup {...props} profiles={[profile, source]} />);
    await user.click(screen.getByRole("button", { name: "Iniciar backup" }));
    expect(backup).toHaveBeenCalledWith(
      expect.objectContaining({
        profile_id: 2,
        database: "financeiro",
        ssl: true,
        tables: ["nome,especial", "nome.com.ponto"],
      }),
    );
  });
});
