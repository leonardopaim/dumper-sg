import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api, APIError } from "../api";
import { ApplicationRestart } from "./ApplicationRestart";

afterEach(() => vi.restoreAllMocks());
const old = { instance_id: "old", restart_available: true };
describe("reinício da aplicação", () => {
  it("confirma, aguarda outra instância e só então recarrega", async () => {
    const application = vi.spyOn(api, "application").mockResolvedValue(old);
    const restart = vi
      .spyOn(api, "restartApplication")
      .mockImplementation(async () => {
        application.mockResolvedValue({ ...old, instance_id: "new" });
        return { status: "restarting", instance_id: "old" };
      });
    const reload = vi.fn();
    const user = userEvent.setup();
    render(<ApplicationRestart onRestarted={reload} />);
    await user.click(
      screen.getByRole("button", { name: "Reiniciar aplicação" }),
    );
    expect(restart).not.toHaveBeenCalled();
    await user.click(
      screen.getByRole("button", { name: "Confirmar reinício" }),
    );
    await waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
    expect(restart).toHaveBeenCalledTimes(1);
  });
  it("bloqueia durante operações e informa quando falta supervisor", async () => {
    vi.spyOn(api, "application").mockResolvedValue({
      ...old,
      restart_available: false,
    });
    const restart = vi.spyOn(api, "restartApplication");
    const user = userEvent.setup();
    const view = render(<ApplicationRestart blocked />);
    expect(
      screen.getByRole("button", { name: "Reiniciar aplicação" }),
    ).toBeDisabled();
    view.rerender(<ApplicationRestart />);
    await user.click(
      screen.getByRole("button", { name: "Reiniciar aplicação" }),
    );
    await screen.findByText(/Inicie pelos scripts/);
    expect(
      screen.getByRole("button", { name: "Confirmar reinício" }),
    ).toBeDisabled();
    expect(restart).not.toHaveBeenCalled();
  });
  it("exibe recusa do backend sem reiniciar nem recarregar", async () => {
    vi.spyOn(api, "application").mockResolvedValue(old);
    vi.spyOn(api, "restartApplication").mockRejectedValue(
      new APIError("Operação em andamento.", 409),
    );
    const reload = vi.fn();
    const user = userEvent.setup();
    render(<ApplicationRestart onRestarted={reload} />);
    await user.click(
      screen.getByRole("button", { name: "Reiniciar aplicação" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Confirmar reinício" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Operação em andamento.",
    );
    expect(reload).not.toHaveBeenCalled();
  });
});
