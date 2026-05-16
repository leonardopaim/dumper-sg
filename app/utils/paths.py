from __future__ import annotations

import sys
from pathlib import Path


def app_root() -> Path:
    if getattr(sys, "frozen", False):
        return Path(sys.executable).resolve().parent
    return Path(__file__).resolve().parents[1]


def logs_dir() -> Path:
    return app_root() / "logs"


def backups_dir() -> Path:
    return app_root() / "backups"


def data_dir() -> Path:
    return app_root() / "data"


def database_path() -> Path:
    return data_dir() / "dumper_sg.sqlite3"


def ensure_app_directories() -> None:
    for path in (logs_dir(), backups_dir(), data_dir()):
        path.mkdir(parents=True, exist_ok=True)

