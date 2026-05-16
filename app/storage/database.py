from __future__ import annotations

import sqlite3
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator

from utils.paths import database_path, ensure_app_directories


class Database:
    def __init__(self, path: Path | None = None) -> None:
        ensure_app_directories()
        self.path = path or database_path()

    @contextmanager
    def connect(self) -> Iterator[sqlite3.Connection]:
        conn = sqlite3.connect(self.path)
        conn.row_factory = sqlite3.Row
        try:
            yield conn
            conn.commit()
        finally:
            conn.close()

    def initialize(self) -> None:
        with self.connect() as conn:
            conn.execute("PRAGMA journal_mode=WAL")
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS profiles (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    nome TEXT NOT NULL UNIQUE,
                    host TEXT NOT NULL,
                    porta INTEGER NOT NULL,
                    usuario TEXT NOT NULL,
                    senha TEXT NOT NULL,
                    database_name TEXT NOT NULL DEFAULT '',
                    ssl INTEGER NOT NULL DEFAULT 0,
                    threads_default INTEGER NOT NULL DEFAULT 8,
                    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS app_settings (
                    key TEXT PRIMARY KEY,
                    value TEXT NOT NULL
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS backup_history (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    profile_id INTEGER,
                    profile_name TEXT NOT NULL,
                    database_name TEXT NOT NULL,
                    destination_dir TEXT NOT NULL,
                    status TEXT NOT NULL,
                    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    finished_at TEXT,
                    message TEXT,
                    FOREIGN KEY(profile_id) REFERENCES profiles(id)
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS restore_history (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    profile_id INTEGER,
                    profile_name TEXT NOT NULL,
                    source_dir TEXT NOT NULL,
                    database_name TEXT NOT NULL,
                    status TEXT NOT NULL,
                    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    finished_at TEXT,
                    message TEXT,
                    FOREIGN KEY(profile_id) REFERENCES profiles(id)
                )
                """
            )

