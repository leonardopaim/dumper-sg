import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Backup } from "./Backup";
import { api } from "../api";
import { tableSelectionKey } from "../hooks/useTableSelection";
import type { Job, Profile, TableInfo } from "../types";
const profile: Profile = {
  id: 1,
  name: "Origem",
  host: "localhost",
  port: 3306,
  user: "root",
  database: "origem",
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
const rows: TableInfo[] = [
  { name: "view_resumo", size_bytes: 0, rows: 0, table_type: "VIEW" },
  { name: "pequena", size_bytes: 1024, rows: 5, table_type: "BASE TABLE" },
  {
    name: "usuarios",
    size_bytes: 1048576,
    rows: 100,
    table_type: "BASE TABLE",
  },
];
const props = {
  settings: {},
  active: false,
  onJob: vi.fn(),
  onProfiles: vi.fn(),
};
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());
describe("backup com seleção de tabelas", () => {
  it("carrega todas marcadas ao escolher personalizada antes da consulta e oferece seleção global", async () => {
    vi.spyOn(api, "tables").mockResolvedValue(rows);
    const backup = vi.spyOn(api, "backup").mockResolvedValue(job);
    const user = userEvent.setup();
    render(<Backup {...props} profiles={[profile]} />);
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    await user.click(
      screen.getByRole("radio", { name: "Seleção personalizada" }),
    );
    expect(
      screen.getByRole("radio", { name: "Seleção personalizada" }),
    ).toBeChecked();
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    const all = await screen.findByRole("checkbox", {
      name: "Marcar todas as tabelas do catálogo",
    });
    await waitFor(() =>
      expect(
        JSON.parse(localStorage.getItem(tableSelectionKey(1, "origem"))!),
      ).toEqual(["usuarios", "pequena", "view_resumo"]),
    );
    expect(all).toBeChecked();
    for (const name of ["usuarios", "pequena", "view_resumo"])
      expect(
        screen.getByRole("checkbox", { name: `Incluir ${name}` }),
      ).toBeChecked();
    await user.click(screen.getByRole("checkbox", { name: "Incluir pequena" }));
    expect(all).toBePartiallyChecked();
    await user.type(screen.getByLabelText("Buscar tabelas"), "usuarios");
    await user.click(all);
    expect(all).toBeChecked();
    expect(
      JSON.parse(localStorage.getItem(tableSelectionKey(1, "origem"))!),
    ).toEqual(["usuarios", "pequena", "view_resumo"]);
    await user.click(all);
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeDisabled();
    await user.click(all);
    await user.click(screen.getByRole("button", { name: "Iniciar backup" }));
    expect(backup).toHaveBeenCalledWith(
      expect.objectContaining({
        tables: ["usuarios", "pequena", "view_resumo"],
      }),
    );
  });

  it("preserva a seleção salva quando consulta ou atualiza tabelas", async () => {
    localStorage.setItem(tableSelectionKey(1, "origem"), '["pequena"]');
    vi.spyOn(api, "tables").mockResolvedValue(rows);
    const user = userEvent.setup();
    render(<Backup {...props} profiles={[profile]} />);
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    expect(
      await screen.findByRole("checkbox", { name: "Incluir pequena" }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "Incluir usuarios" }),
    ).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "Atualizar tabelas" }));
    expect(
      screen.getByRole("checkbox", { name: "Incluir usuarios" }),
    ).not.toBeChecked();
    expect(localStorage.getItem(tableSelectionKey(1, "origem"))).toBe(
      '["pequena"]',
    );
  });

  it("preserva um conjunto aplicado enquanto a primeira consulta personalizada está em andamento", async () => {
    let finish!: (tables: TableInfo[]) => void;
    vi.spyOn(api, "tables").mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const user = userEvent.setup();
    render(
      <Backup
        {...props}
        profiles={[
          {
            ...profile,
            table_presets: [
              { name: "Essencial", database: "origem", tables: ["usuarios"] },
            ],
          },
        ]}
      />,
    );
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    await user.click(
      screen.getByRole("radio", { name: "Seleção personalizada" }),
    );
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    await user.selectOptions(
      screen.getByLabelText(/Usar seleção salva/),
      "1:0",
    );
    finish(rows);
    expect(
      await screen.findByRole("checkbox", { name: "Incluir usuarios" }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "Incluir pequena" }),
    ).not.toBeChecked();
    expect(localStorage.getItem(tableSelectionKey(1, "origem"))).toBe(
      '["usuarios"]',
    );
  });
  it("mantém todas como padrão sem consultar o banco", async () => {
    const tables = vi.spyOn(api, "tables");
    const backup = vi.spyOn(api, "backup").mockResolvedValue(job);
    render(<Backup {...props} profiles={[profile]} />);
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Iniciar backup" }));
    expect(tables).not.toHaveBeenCalled();
    expect(backup).toHaveBeenCalledWith(
      expect.objectContaining({ tables: null }),
    );
  });
  it("consulta tamanho decrescente, filtra seleção explícita e bloqueia nenhuma tabela", async () => {
    const tables = vi.spyOn(api, "tables").mockResolvedValue(rows);
    const backup = vi.spyOn(api, "backup").mockResolvedValue(job);
    const user = userEvent.setup();
    const view = render(<Backup {...props} profiles={[profile]} />);
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    expect(tables).toHaveBeenCalledWith(
      { profile_id: 1, database: "origem", ssl: false },
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    const catalog = screen.getByLabelText("Catálogo de tabelas por tamanho");
    expect(within(catalog).getAllByRole("row")[1]).toHaveTextContent(
      "usuarios",
    );
    expect(within(catalog).getByText("View")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Buscar tabelas"), "usuarios");
    await user.click(
      screen.getByRole("button", { name: "Selecionar somente visíveis" }),
    );
    expect(localStorage.getItem(tableSelectionKey(1, "origem"))).toBe(
      '["usuarios"]',
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "1 selecionadas · 1 MiB estimados",
    );
    await user.type(
      screen.getByLabelText(/Excluir tabelas por expressão/),
      "logs",
    );
    await user.click(screen.getByRole("button", { name: "Iniciar backup" }));
    expect(backup).toHaveBeenCalledWith(
      expect.objectContaining({ tables: ["usuarios"], ignore_regex: "logs" }),
    );
    await user.click(screen.getByRole("button", { name: "Desmarcar todas" }));
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeDisabled();
    expect(backup).toHaveBeenCalledTimes(1);
    view.unmount();
    render(<Backup {...props} profiles={[profile]} />);
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeDisabled();
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    await user.click(screen.getByRole("radio", { name: /Todas as tabelas/ }));
    expect(
      screen.getByRole("button", { name: "Iniciar backup" }),
    ).toBeEnabled();
    expect(localStorage.getItem(tableSelectionKey(1, "origem"))).toBeNull();
  });
  it("altera o checkbox sem descartar nomes fora da busca", async () => {
    vi.spyOn(api, "tables").mockResolvedValue(rows);
    const user = userEvent.setup();
    render(<Backup {...props} profiles={[profile]} />);
    await user.click(
      screen.getByRole("button", { name: /Selecionar tabelas/ }),
    );
    await user.click(screen.getByRole("button", { name: "Consultar tabelas" }));
    await user.click(screen.getByRole("checkbox", { name: "Incluir pequena" }));
    expect(
      JSON.parse(localStorage.getItem(tableSelectionKey(1, "origem"))!),
    ).toEqual(["usuarios", "view_resumo"]);
    await user.type(screen.getByLabelText("Buscar tabelas"), "usuarios");
    await user.click(
      screen.getByRole("button", { name: "Desmarcar visíveis" }),
    );
    expect(
      JSON.parse(localStorage.getItem(tableSelectionKey(1, "origem"))!),
    ).toEqual(["view_resumo"]);
  });
});
