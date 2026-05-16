from __future__ import annotations

from typing import Any

from models.connection_profile import ConnectionProfile
from storage.database import Database
from utils.paths import backups_dir


class SettingsRepository:
    def __init__(self, db: Database) -> None:
        self.db = db

    def list_profiles(self) -> list[ConnectionProfile]:
        with self.db.connect() as conn:
            rows = conn.execute(
                """
                SELECT id, nome, host, porta, usuario, senha, database_name, ssl, threads_default
                FROM profiles
                ORDER BY nome
                """
            ).fetchall()
        return [
            ConnectionProfile(
                id=row["id"],
                nome=row["nome"],
                host=row["host"],
                porta=row["porta"],
                usuario=row["usuario"],
                senha=row["senha"],
                database=row["database_name"],
                ssl=bool(row["ssl"]),
                threads_default=row["threads_default"],
            )
            for row in rows
        ]

    def get_profile(self, profile_id: int) -> ConnectionProfile | None:
        with self.db.connect() as conn:
            row = conn.execute(
                """
                SELECT id, nome, host, porta, usuario, senha, database_name, ssl, threads_default
                FROM profiles
                WHERE id = ?
                """,
                (profile_id,),
            ).fetchone()
        if row is None:
            return None
        return ConnectionProfile(
            id=row["id"],
            nome=row["nome"],
            host=row["host"],
            porta=row["porta"],
            usuario=row["usuario"],
            senha=row["senha"],
            database=row["database_name"],
            ssl=bool(row["ssl"]),
            threads_default=row["threads_default"],
        )

    def save_profile(self, profile: ConnectionProfile) -> int:
        with self.db.connect() as conn:
            if profile.id is None:
                cursor = conn.execute(
                    """
                    INSERT INTO profiles
                    (nome, host, porta, usuario, senha, database_name, ssl, threads_default)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        profile.nome,
                        profile.host,
                        profile.porta,
                        profile.usuario,
                        profile.senha,
                        profile.database,
                        int(profile.ssl),
                        profile.threads_default,
                    ),
                )
                return int(cursor.lastrowid)

            conn.execute(
                """
                UPDATE profiles
                SET nome = ?, host = ?, porta = ?, usuario = ?, senha = ?,
                    database_name = ?, ssl = ?, threads_default = ?,
                    updated_at = CURRENT_TIMESTAMP
                WHERE id = ?
                """,
                (
                    profile.nome,
                    profile.host,
                    profile.porta,
                    profile.usuario,
                    profile.senha,
                    profile.database,
                    int(profile.ssl),
                    profile.threads_default,
                    profile.id,
                ),
            )
            return profile.id

    def delete_profile(self, profile_id: int) -> None:
        with self.db.connect() as conn:
            conn.execute("DELETE FROM profiles WHERE id = ?", (profile_id,))

    def get_setting(self, key: str, default: str = "") -> str:
        with self.db.connect() as conn:
            row = conn.execute("SELECT value FROM app_settings WHERE key = ?", (key,)).fetchone()
        if row is None:
            return default
        return str(row["value"])

    def set_setting(self, key: str, value: Any) -> None:
        with self.db.connect() as conn:
            conn.execute(
                """
                INSERT INTO app_settings (key, value)
                VALUES (?, ?)
                ON CONFLICT(key) DO UPDATE SET value = excluded.value
                """,
                (key, str(value)),
            )

    def default_backup_dir(self) -> str:
        return self.get_setting("default_backup_dir", str(backups_dir()))

    def add_backup_history(
        self,
        profile_id: int | None,
        profile_name: str,
        database_name: str,
        destination_dir: str,
        status: str,
        message: str = "",
    ) -> int:
        with self.db.connect() as conn:
            cursor = conn.execute(
                """
                INSERT INTO backup_history
                (profile_id, profile_name, database_name, destination_dir, status, message)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                (profile_id, profile_name, database_name, destination_dir, status, message),
            )
            return int(cursor.lastrowid)

    def finish_backup_history(self, history_id: int, status: str, message: str = "") -> None:
        with self.db.connect() as conn:
            conn.execute(
                """
                UPDATE backup_history
                SET status = ?, finished_at = CURRENT_TIMESTAMP, message = ?
                WHERE id = ?
                """,
                (status, message, history_id),
            )

    def list_backup_history(self, limit: int = 50) -> list[dict[str, Any]]:
        with self.db.connect() as conn:
            rows = conn.execute(
                """
                SELECT id, profile_name, database_name, destination_dir, status, started_at, finished_at, message
                FROM backup_history
                ORDER BY id DESC
                LIMIT ?
                """,
                (limit,),
            ).fetchall()
        return [dict(row) for row in rows]

    def add_restore_history(
        self,
        profile_id: int | None,
        profile_name: str,
        source_dir: str,
        database_name: str,
        status: str,
        message: str = "",
    ) -> int:
        with self.db.connect() as conn:
            cursor = conn.execute(
                """
                INSERT INTO restore_history
                (profile_id, profile_name, source_dir, database_name, status, message)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                (profile_id, profile_name, source_dir, database_name, status, message),
            )
            return int(cursor.lastrowid)

    def finish_restore_history(self, history_id: int, status: str, message: str = "") -> None:
        with self.db.connect() as conn:
            conn.execute(
                """
                UPDATE restore_history
                SET status = ?, finished_at = CURRENT_TIMESTAMP, message = ?
                WHERE id = ?
                """,
                (status, message, history_id),
            )

    def list_restore_history(self, limit: int = 50) -> list[dict[str, Any]]:
        with self.db.connect() as conn:
            rows = conn.execute(
                """
                SELECT id, profile_name, source_dir, database_name, status, started_at, finished_at, message
                FROM restore_history
                ORDER BY id DESC
                LIMIT ?
                """,
                (limit,),
            ).fetchall()
        return [dict(row) for row in rows]

