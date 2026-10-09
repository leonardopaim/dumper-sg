import { useEffect, useId, useRef } from "react";
import { AlertCircle, CheckCircle2, LoaderCircle, X } from "lucide-react";
import type { ReactNode, ButtonHTMLAttributes } from "react";
import type { JobKind, JobStatus } from "../types";
export const kinds: Record<JobKind, string> = {
  backup: "Backup",
  restore: "Restauração",
  connection_test: "Teste de conexão",
  create_database: "Criar banco",
  table_list: "Consulta de tabelas",
  database_list: "Consulta de bancos",
};
export const statuses: Record<JobStatus, string> = {
  running: "Em execução",
  cancel_requested: "Cancelando",
  succeeded: "Concluído",
  failed: "Falhou",
  cancelled: "Cancelado",
};
export const isActive = (status: JobStatus) =>
  status === "running" || status === "cancel_requested";
export const dateTime = (value?: string) =>
  value ? new Date(value).toLocaleString("pt-BR") : "—";
export function Button({
  children,
  busy = false,
  variant = "primary",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  busy?: boolean;
  variant?: "primary" | "secondary" | "danger" | "ghost";
}) {
  return (
    <button
      {...props}
      disabled={props.disabled || busy}
      className={`button ${variant} ${props.className || ""}`}
    >
      {busy && <LoaderCircle className="spin" size={16} />}
      {children}
    </button>
  );
}
export function Alert({
  children,
  success = false,
  warning = false,
}: {
  children: ReactNode;
  success?: boolean;
  warning?: boolean;
}) {
  return (
    <div
      className={`alert ${success ? "success" : warning ? "warning" : "error"}`}
      role={success ? "status" : "alert"}
    >
      {success ? <CheckCircle2 size={18} /> : <AlertCircle size={18} />}
      <span>{children}</span>
    </div>
  );
}
export function Badge({
  status,
  partial = false,
  warnings = false,
}: {
  status: JobStatus;
  partial?: boolean;
  warnings?: boolean;
}) {
  const qualified = status === "succeeded" && (partial || warnings);
  return (
    <span className={`badge ${qualified ? "partial" : status}`}>
      {isActive(status) && <span className="status-dot" />}
      {qualified ? (partial ? "Parcial" : "Com alertas") : statuses[status]}
    </span>
  );
}
export function Empty({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <strong>{title}</strong>
      {children && <p>{children}</p>}
    </div>
  );
}
export function PageHeader({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="page-header">
      <div>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action}
    </div>
  );
}
export function Field({
  label,
  hint,
  children,
  className = "",
}: {
  label: string;
  hint?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <label className={`field ${className}`}>
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
export function Toggle({
  label,
  hint,
  checked,
  onChange,
  disabled = false,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (value: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <label className="toggle-field">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        disabled={disabled}
      />
      <span>
        <strong>{label}</strong>
        {hint && <small>{hint}</small>}
      </span>
    </label>
  );
}
export function Modal({
  title,
  children,
  onClose,
  closeLabel = "Fechar janela",
  className = "",
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  closeLabel?: string;
  className?: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    ref.current?.showModal();
    return () => {
      ref.current?.close();
      previouslyFocused?.focus();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className={`modal ${className}`}
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
    >
      <div className="modal-heading">
        <h2 id={titleId}>{title}</h2>
        <button
          className="icon-button"
          type="button"
          aria-label={closeLabel}
          onClick={onClose}
        >
          <X size={20} />
        </button>
      </div>
      {children}
    </dialog>
  );
}
