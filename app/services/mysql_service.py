from __future__ import annotations

from models.connection_profile import ConnectionProfile
from services.docker_service import DockerCommand, DockerService


class MySqlService:
    def __init__(self, docker_service: DockerService | None = None, image: str = "mysql:8.4") -> None:
        self.docker = docker_service or DockerService()
        self.image = image

    def build_test_connection_command(self, profile: ConnectionProfile) -> DockerCommand:
        args = self._base_args()
        args.extend(
            [
                "mysql",
                f"--host={profile.host}",
                f"--port={profile.porta}",
                f"--user={profile.usuario}",
                f"--password={profile.senha}",
                "--connect-timeout=8",
                "--execute=SELECT 1;",
            ]
        )
        if profile.ssl:
            args.append("--ssl")
        return self.docker.command(args)

    def build_create_database_command(self, profile: ConnectionProfile, database_name: str) -> DockerCommand:
        escaped_db = database_name.replace("`", "``")
        args = self._base_args()
        args.extend(
            [
                "mysql",
                f"--host={profile.host}",
                f"--port={profile.porta}",
                f"--user={profile.usuario}",
                f"--password={profile.senha}",
                f"--execute=CREATE DATABASE IF NOT EXISTS `{escaped_db}` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
            ]
        )
        if profile.ssl:
            args.append("--ssl")
        return self.docker.command(args)

    def _base_args(self) -> list[str]:
        args = ["run", "--rm"]
        if self.docker.use_host_network:
            args.extend(["--network", "host"])
        args.append(self.image)
        return args
