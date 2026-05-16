from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass(slots=True)
class BackupJob:
    profile_id: int | None
    profile_name: str
    host: str
    port: int
    user: str
    password: str
    database: str
    destination_dir: Path
    threads: int
    compress: bool
    ssl: bool
    non_locking: bool = True
    ignore_regex: str = ""


@dataclass(slots=True)
class RestoreJob:
    profile_id: int | None
    profile_name: str
    host: str
    port: int
    user: str
    password: str
    backup_dir: Path
    target_database: str
    threads: int
    overwrite_tables: bool

