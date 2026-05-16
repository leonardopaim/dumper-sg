from __future__ import annotations

import re
from pathlib import Path


LOCALHOST_ALIASES = {"localhost", "127.0.0.1", "::1", "host.docker.internal"}


def validate_required(value: str, field_name: str) -> str | None:
    if not value.strip():
        return f"{field_name} e obrigatorio."
    return None


def validate_port(port: int) -> str | None:
    if port < 1 or port > 65535:
        return "Porta deve estar entre 1 e 65535."
    return None


def validate_threads(threads: int) -> str | None:
    if threads < 1 or threads > 256:
        return "Threads deve estar entre 1 e 256."
    return None


def validate_regex(pattern: str) -> str | None:
    if not pattern.strip():
        return None
    try:
        re.compile(pattern)
    except re.error as exc:
        return f"Regex invalido: {exc}"
    return None


def validate_existing_directory(path: str, field_name: str) -> str | None:
    if not path.strip():
        return f"{field_name} e obrigatorio."
    if not Path(path).exists() or not Path(path).is_dir():
        return f"{field_name} deve ser uma pasta existente."
    return None


def validate_local_mysql_host(host: str) -> str | None:
    normalized = host.strip().lower()
    if normalized in LOCALHOST_ALIASES:
        return None
    return (
        "Restore bloqueado: restauracoes so podem ser executadas em MySQL local "
        "(localhost, 127.0.0.1, ::1 ou host.docker.internal)."
    )

