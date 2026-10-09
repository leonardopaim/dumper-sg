import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Backup } from "./Backup";
import { backupDraftKey } from "../hooks/useOperationDraft";
import type { Profile } from "../types";

const profile: Profile = {
  id: 1,
  name: "Origem",
  host: "localhost",
  port: 3306,
  user: "qa",
  database: "origem",
  threads: 2,
  ssl: false,
  has_password: true,
};
const props = {
  settings: {},
  active: false,
  onJob: vi.fn(),
  onProfiles: vi.fn(),
};
beforeEach(() => localStorage.clear());
describe("preferências do backup", () => {
  it("lembra a seleção e preserva os campos quando os perfis são reordenados", async () => {
    const second = {
      ...profile,
      id: 2,
      name: "Outra origem",
      database: "outro",
      ssl: true,
    };
    const user = userEvent.setup();
    const view = render(<Backup {...props} profiles={[profile, second]} />);
    await user.selectOptions(screen.getByLabelText("Perfil de conexão"), "2");
    await user.clear(screen.getByLabelText("Banco de origem"));
    await user.type(screen.getByLabelText("Banco de origem"), "personalizado");
    await user.type(
      screen.getByLabelText(/Diretório de destino/),
      "C:\\backup",
    );
    expect(JSON.parse(localStorage.getItem(backupDraftKey)!)).toMatchObject({
      profile_id: 2,
      database: "personalizado",
      destination_dir: "C:\\backup",
      ssl: true,
    });
    view.unmount();
    render(<Backup {...props} profiles={[second, profile]} />);
    expect(screen.getByLabelText("Perfil de conexão")).toHaveValue("2");
    expect(screen.getByLabelText("Banco de origem")).toHaveValue(
      "personalizado",
    );
    expect(screen.getByLabelText(/Diretório de destino/)).toHaveValue(
      "C:\\backup",
    );
  });
  it("usa os padrões com dados inválidos e não copia campos de credenciais", () => {
    localStorage.setItem(
      backupDraftKey,
      JSON.stringify({
        profile_id: "2",
        database: 42,
        password: "secret",
        user: "other",
      }),
    );
    render(<Backup {...props} profiles={[profile]} />);
    expect(screen.getByLabelText("Perfil de conexão")).toHaveValue("1");
    expect(screen.getByLabelText("Banco de origem")).toHaveValue("origem");
    expect(localStorage.getItem(backupDraftKey)).not.toMatch(
      /password|secret|user/,
    );
  });
});
