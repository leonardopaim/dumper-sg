import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Profiles } from "./Profiles";
import { api } from "../api";
import type { Profile } from "../types";
const profile: Profile = {
  id: 1,
  name: "Local",
  host: "127.0.0.1",
  port: 3306,
  user: "root",
  database: "origem",
  ssl: false,
  threads: 8,
  has_password: true,
};
describe("credenciais de perfil", () => {
  it("edita mantendo a senha sem enviar password e sem exibir senha anterior", async () => {
    const save = vi.spyOn(api, "updateProfile").mockResolvedValue(profile);
    render(
      <Profiles
        profiles={[profile]}
        reload={vi.fn().mockResolvedValue(undefined)}
        onJob={vi.fn()}
        active={false}
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Editar Local" }));
    expect(screen.queryByLabelText("Nova senha")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Salvar perfil" }));
    expect(save).toHaveBeenCalledWith(
      1,
      expect.not.objectContaining({ password: expect.anything() }),
    );
    save.mockRestore();
  });
  it("limpa a senha apenas quando esta ação é selecionada", async () => {
    const save = vi.spyOn(api, "updateProfile").mockResolvedValue(profile);
    render(
      <Profiles
        profiles={[profile]}
        reload={vi.fn().mockResolvedValue(undefined)}
        onJob={vi.fn()}
        active={false}
      />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Editar Local" }));
    await user.selectOptions(
      screen.getByRole("combobox", { name: /Senha/ }),
      "clear",
    );
    await user.click(screen.getByRole("button", { name: "Salvar perfil" }));
    expect(save).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ password: "" }),
    );
    save.mockRestore();
  });
});
