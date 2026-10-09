import { useEffect, useRef, useState } from "react";
import { RotateCw } from "lucide-react";
import { api, APIError, errorMessage } from "../api";
import { boundedRequest, waitForRestart } from "../application";
import type { ApplicationStatus } from "../types";
import { Alert, Button, Modal } from "./ui";

export function ApplicationRestart({
  blocked = false,
  onRestarted = () => location.reload(),
}: {
  blocked?: boolean;
  onRestarted?: () => void;
}) {
  const [application, setApplication] = useState<ApplicationStatus>();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    const load = new AbortController();
    void boundedRequest(api.application, load.signal)
      .then(setApplication)
      .catch(() => undefined);
    return () => {
      load.abort();
      controller.current?.abort();
    };
  }, []);
  const restart = async () => {
    const operation = new AbortController();
    controller.current = operation;
    setBusy(true);
    setError("");
    try {
      const current = await boundedRequest(api.application, operation.signal);
      if (!current.restart_available)
        throw new Error(
          "Inicie o DumperSG por scripts/start.ps1 ou pela inicialização automática para habilitar o reinício.",
        );
      try {
        await boundedRequest(api.restartApplication, operation.signal);
      } catch (e) {
        if (e instanceof APIError) throw e;
        // An accepted restart can lose its response as the old process exits.
      }
      await waitForRestart(current.instance_id, operation.signal);
      onRestarted();
    } catch (e) {
      if (!operation.signal.aborted) setError(errorMessage(e));
    } finally {
      if (!operation.signal.aborted) setBusy(false);
    }
  };
  return (
    <>
      <Button
        type="button"
        variant="secondary"
        className="application-restart-toggle"
        aria-label="Reiniciar aplicação"
        disabled={blocked || busy}
        title={
          blocked
            ? "Conclua ou cancele a operação em andamento antes de reiniciar."
            : "Reiniciar aplicação"
        }
        onClick={() => {
          setError("");
          setOpen(true);
        }}
      >
        <RotateCw size={15} />
        <span className="topbar-nav-label">Reiniciar</span>
      </Button>
      {open && (
        <Modal
          title="Reiniciar aplicação"
          onClose={() => {
            if (!busy) setOpen(false);
          }}
        >
          <p>
            {busy
              ? "Reiniciando o DumperSG. A página será recarregada quando a aplicação estiver pronta."
              : "Deseja reiniciar o DumperSG? A aplicação ficará indisponível por alguns segundos. Seus perfis, configurações e histórico serão preservados."}
          </p>
          {application?.restart_available === false && (
            <Alert>
              Inicie pelos scripts ou pela inicialização automática para
              habilitar o reinício.
            </Alert>
          )}
          {blocked && (
            <Alert>
              Conclua ou cancele a operação em andamento antes de reiniciar.
            </Alert>
          )}
          {error && <Alert>{error}</Alert>}
          <p className="muted">
            Se o backend não responder, use no PowerShell:
          </p>
          <code className="path-block">.\scripts\restart.ps1 -Force</code>
          <div className="modal-footer">
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => setOpen(false)}
            >
              Voltar
            </Button>
            <Button
              busy={busy}
              disabled={blocked || application?.restart_available === false}
              onClick={() => void restart()}
            >
              Confirmar reinício
            </Button>
          </div>
        </Modal>
      )}
    </>
  );
}
