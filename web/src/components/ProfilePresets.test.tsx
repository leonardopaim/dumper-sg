import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Profiles } from "../pages/Profiles";
import { api } from "../api";
import type { Profile } from "../types";
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
const props = {
  reload: vi.fn().mockResolvedValue(undefined),
  onJob: vi.fn(),
  active: false,
};
beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());
describe("seleções nomeadas do perfil", () => {
  it("salva e remove presets via PATCH apenas de table_presets, sem copiar credenciais", async () => {
    const tables = vi.spyOn(api, "tables").mockResolvedValue([
      { name: "clientes", size_bytes: 10, rows: 1, table_type: "BASE TABLE" },
      { name: "logs", size_bytes: 5, rows: 1, table_type: "BASE TABLE" },
    ]);
    const patch = vi.spyOn(api, "updateProfile").mockResolvedValue(profile);
    const user = userEvent.setup();
    render(<Profiles {...props} profiles={[profile]} />);
    await user.click(
      screen.getByRole("button", { name: "Seleções de tabelas" }),
    );
    const modal = screen.getByRole("dialog", {
      name: "Seleções de tabelas · Origem",
    });
    await user.type(
      within(modal).getByLabelText(/Banco de origem da seleção/),
      "origem",
    );
    await user.type(
      within(modal).getByLabelText(/Nome da seleção/),
      "Essencial",
    );
    await user.click(
      within(modal).getByRole("button", { name: "Consultar tabelas" }),
    );
    expect(tables).toHaveBeenCalledWith(
      { profile_id: 1, database: "origem", ssl: false },
      expect.any(Object),
    );
    await user.click(
      within(modal).getByRole("checkbox", { name: "Incluir logs" }),
    );
    await user.click(
      within(modal).getByRole("button", { name: "Salvar seleção no perfil" }),
    );
    expect(patch).toHaveBeenLastCalledWith(1, {
      table_presets: [
        { name: "Essencial", database: "origem", tables: ["clientes"] },
      ],
    });
    expect(screen.getByText("Essencial")).toBeInTheDocument();
    await user.click(
      within(modal).getByRole("button", { name: "Remover seleção Essencial" }),
    );
    expect(patch).toHaveBeenLastCalledWith(1, { table_presets: [] });
    for (const [, body] of patch.mock.calls)
      expect(Object.keys(body)).toEqual(["table_presets"]);
  });
  it("reabre seleções recebidas do backend sem consultar automaticamente", async () => {
    const tables = vi.spyOn(api, "tables");
    render(
      <Profiles
        {...props}
        profiles={[
          {
            ...profile,
            table_presets: [
              { name: "Persistida", database: "origem", tables: ["clientes"] },
            ],
          },
        ]}
      />,
    );
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Seleções de tabelas" }));
    expect(screen.getByText("Persistida")).toBeInTheDocument();
    expect(screen.getByLabelText(/Banco de origem da seleção/)).toHaveValue(
      "origem",
    );
    expect(tables).not.toHaveBeenCalled();
  });
});
