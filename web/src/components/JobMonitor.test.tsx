import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { JobMonitor, logsPreferenceKey } from "./JobMonitor";
import type { JobsController } from "../hooks/useJobs";
import type { Job, JobKind } from "../types";
const running: Job = {
  id: "job1",
  kind: "backup",
  status: "running",
  profile_id: 1,
  profile_name: "Local",
  database: "origem",
  path: "/backups/operacao",
  started_at: "2026-10-08T12:00:00Z",
  progress: 37,
  message: "Processando tabelas",
};
function controller(job: Job = running, error = ""): JobsController {
  return {
    jobs: [job],
    selected: job,
    events: [
      {
        sequence: 1,
        time: job.started_at,
        level: "info",
        message: "Evento recebido em segundo plano",
      },
    ],
    error,
    eventsLoading: false,
    cancelBusy: false,
    select: vi.fn(),
    accept: vi.fn(),
    cancel: vi.fn().mockResolvedValue(undefined),
    refresh: vi.fn().mockResolvedValue([job]),
  };
}
beforeEach(() => localStorage.clear());
describe("acompanhamento compacto", () => {
  it.each<JobKind>(["backup", "restore"])(
    "exibe progresso e cancelamento de %s sem abrir logs",
    async (kind) => {
      const control = controller({ ...running, kind });
      render(<JobMonitor controller={control} />);
      const monitor = screen.getByRole("region", {
        name: "Acompanhamento da operação",
      });
      expect(within(monitor).getByRole("progressbar")).toHaveAttribute(
        "aria-valuenow",
        "37",
      );
      expect(within(monitor).getByText("37% estimado")).toBeInTheDocument();
      expect(
        within(monitor).getByText("Processando tabelas"),
      ).toBeInTheDocument();
      expect(
        within(monitor).getByText("/backups/operacao"),
      ).toBeInTheDocument();
      expect(within(monitor).getByText(/Duração/)).toBeInTheDocument();
      expect(
        screen.queryByLabelText("Logs da operação"),
      ).not.toBeInTheDocument();
      await userEvent
        .setup()
        .click(within(monitor).getByRole("button", { name: "Cancelar" }));
      expect(control.cancel).toHaveBeenCalledWith(
        expect.objectContaining({ kind }),
      );
    },
  );
  it("persiste apenas a preferência booleana e restaura Mostrar/Ocultar logs após recarregar", async () => {
    const user = userEvent.setup();
    const control = controller();
    let view = render(<JobMonitor controller={control} />);
    await user.click(screen.getByRole("button", { name: "Mostrar logs" }));
    expect(localStorage.getItem(logsPreferenceKey)).toBe("true");
    expect(localStorage.length).toBe(1);
    view.unmount();
    view = render(<JobMonitor controller={control} />);
    expect(screen.getByLabelText("Logs da operação")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ocultar logs" }),
    ).toHaveAttribute("aria-expanded", "true");
    await user.click(screen.getByRole("button", { name: "Ocultar logs" }));
    expect(localStorage.getItem(logsPreferenceKey)).toBe("false");
    view.unmount();
    render(<JobMonitor controller={control} />);
    expect(
      screen.getByRole("button", { name: "Mostrar logs" }),
    ).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByLabelText("Logs da operação")).not.toBeInTheDocument();
  });
  it("mantém erros acessíveis com os logs ocultos", () => {
    render(
      <JobMonitor
        controller={controller(
          {
            ...running,
            status: "failed",
            message: "Falha no processo",
            cleanup_required: true,
          },
          "Não foi possível verificar o término.",
        )}
      />,
    );
    expect(screen.getByText("Falha no processo")).toBeInTheDocument();
    expect(
      screen.getByText("Não foi possível verificar o término."),
    ).toBeInTheDocument();
    expect(screen.getByText(/daemon Docker no WSL/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Verificar término" }),
    ).toBeEnabled();
    expect(screen.queryByLabelText("Logs da operação")).not.toBeInTheDocument();
  });
  it("mostra progresso indeterminado antes da primeira estimativa", () => {
    render(<JobMonitor controller={controller({ ...running, progress: 0 })} />);
    expect(screen.getByRole("progressbar")).not.toHaveAttribute(
      "aria-valuenow",
    );
    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuetext",
      "Em andamento, sem estimativa",
    );
    expect(
      screen.getByText("Em andamento · sem estimativa"),
    ).toBeInTheDocument();
  });
});
