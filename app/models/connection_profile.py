from __future__ import annotations

from dataclasses import dataclass


@dataclass(slots=True)
class ConnectionProfile:
    id: int | None
    nome: str
    host: str
    porta: int
    usuario: str
    senha: str
    database: str = ""
    ssl: bool = False
    threads_default: int = 8

    @property
    def label(self) -> str:
        return f"{self.nome} ({self.host}:{self.porta})"

