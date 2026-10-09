import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Restore } from "./Restore";
import { api } from "../api";
import type { BackupTableInfo, Job, Profile } from "../types";
const local: Profile = {
  id: 1,
  name: "Destino local",
  host: "localhost",
  port: 3306,
  user: "root",
  database: "principal",
  ssl: false,
  threads: 8,
  has_password: true,
};
const source: Profile = {
  ...local,
  id: 2,
  name: "Origem remota",
  host: "remote.example",
  database: "origem",
  table_presets: [
    { name: "Essencial", database: "origem", tables: ["cliente", "ausente"] },
    { name: "Inexistente", database: "outro_banco", tables: ["cliente"] },
  ],
};
const rows: BackupTableInfo[] = [
  {
    database: "origem",
    name: "cliente",
    size_bytes: 2000,
    rows: 10,
    table_type: "BASE TABLE",
  },
  {
    database: "outro",
    name: "cliente",
    size_bytes: 1000,
    rows: 10,
    table_type: "BASE TABLE",
  },
  {
    database: "origem",
    name: "logs",
    size_bytes: 500,
    rows: 5,
    table_type: "BASE TABLE",
  },
];
const job: Job = {
  id: "restore1",
  kind: "restore",
  status: "running",
  profile_id: 1,
  profile_name: "Destino local",
  database: "isolado",
  path: "/backup",
  started_at: "2026-10-08T12:00:00Z",
  progress: 0,
  message: "",
};
const props = {
  backups: [],
  reloadBackups: vi.fn().mockResolvedValue(undefined),
  onJob: vi.fn(),
  active: false,
  onProfiles: vi.fn(),
};
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());
async function fill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Diretório do backup/), "/backup");
  await user.type(screen.getByLabelText(/Banco isolado de destino/), "isolado");
  await user.click(screen.getByRole("button", { name: /Selecionar tabelas/ }));
}
describe("seleção dos arquivos para restaurar", () => {
  it("marca todos os arquivos inicialmente e usa o cabeçalho para selecionar o catálogo inteiro", async () => {
    vi.spyOn(api, "backupTables").mockResolvedValue(rows);
    const user = userEvent.setup();
    render(<Restore {...props} profiles={[local, source]} />);
    await fill(user);
    await user.click(
      screen.getByRole("radio", { name: "Seleção personalizada" }),
    );
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    const all = await screen.findByRole("checkbox", {
      name: "Marcar todas as tabelas do catálogo",
    });
    expect(all).toBeChecked();
    await user.click(
      screen.getByRole("checkbox", { name: "Incluir outro / cliente" }),
    );
    expect(all).toBePartiallyChecked();
    await user.type(screen.getByLabelText("Buscar tabelas"), "logs");
    await user.click(all);
    await user.clear(screen.getByLabelText("Buscar tabelas"));
    expect(
      screen.getByRole("checkbox", { name: "Incluir outro / cliente" }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "Incluir origem / cliente" }),
    ).toBeChecked();
  });
  it("consulta arquivos, aplica preset da origem sem mudar destino e confirma nomes qualificados", async () => {
    const catalog = vi.spyOn(api, "backupTables").mockResolvedValue(rows);
    const sql = vi.spyOn(api, "tables");
    const restore = vi.spyOn(api, "restore").mockResolvedValue(job);
    const user = userEvent.setup();
    render(<Restore {...props} profiles={[local, source]} />);
    await fill(user);
    expect(screen.getByLabelText(/Usar seleção salva/)).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    expect(catalog).toHaveBeenCalledWith("/backup", expect.any(AbortSignal));
    expect(sql).not.toHaveBeenCalled();
    await user.selectOptions(
      screen.getByLabelText(/Usar seleção salva/),
      "2:0",
    );
    expect(screen.getByLabelText("Perfil local")).toHaveValue("1");
    expect(
      screen.getByText(/Seleção “Essencial” aplicada: 1 de 2 tabelas/),
    ).toHaveTextContent("Ausentes no backup: ausente");
    await user.click(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    );
    let modal = screen.getByRole("dialog", { name: "Confirmar restauração" });
    expect(within(modal).getByText("1 selecionadas")).toBeInTheDocument();
    expect(within(modal).getByText("origem / cliente")).toBeInTheDocument();
    await user.click(within(modal).getByRole("button", { name: "Cancelar" }));
    expect(restore).not.toHaveBeenCalled();
    await user.click(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    );
    modal = screen.getByRole("dialog", { name: "Confirmar restauração" });
    await user.click(
      within(modal).getByRole("button", { name: "Confirmar restauração" }),
    );
    expect(restore).toHaveBeenCalledWith(
      expect.objectContaining({
        profile_id: 1,
        target_database: "isolado",
        tables: [{ database: "origem", name: "cliente" }],
      }),
    );
  });
  it("bloqueia seleção vazia ou preset sem correspondência e retorna para todas", async () => {
    vi.spyOn(api, "backupTables").mockResolvedValue(rows);
    const restore = vi.spyOn(api, "restore").mockResolvedValue(job);
    const user = userEvent.setup();
    render(<Restore {...props} profiles={[local, source]} />);
    await fill(user);
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    await user.selectOptions(
      screen.getByLabelText(/Usar seleção salva/),
      "2:1",
    );
    expect(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    ).toBeDisabled();
    expect(screen.getByText(/aplicada: 0 de 1 tabelas/)).toBeInTheDocument();
    await user.click(
      screen.getByRole("radio", { name: "Todas as tabelas do backup" }),
    );
    expect(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    ).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Desmarcar todas" }));
    expect(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    ).toBeDisabled();
    await user.click(
      screen.getByRole("checkbox", { name: "Incluir outro / cliente" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Iniciar restauração" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Confirmar restauração" }),
    );
    expect(restore).toHaveBeenCalledWith(
      expect.objectContaining({
        tables: [{ database: "outro", name: "cliente" }],
      }),
    );
  });
});
