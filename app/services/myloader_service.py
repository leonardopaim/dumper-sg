from __future__ import annotations

from pathlib import Path

from models.backup_job import RestoreJob
from services.docker_service import DockerCommand, DockerService


class MyLoaderService:
    def __init__(self, docker_service: DockerService | None = None) -> None:
        self.docker = docker_service or DockerService()

    def build_command(self, job: RestoreJob) -> DockerCommand:
        backup_dir = job.backup_dir.resolve()
        host_mount = backup_dir.parent
        container_dir = f"/backup/{backup_dir.name}"

        args = self.docker.build_base_args(host_mount)
        args.extend(
            [
                "myloader",
                f"--host={job.host}",
                f"--port={job.port}",
                f"--user={job.user}",
                f"--password={job.password}",
                f"--database={job.target_database}",
                f"--directory={container_dir}",
                f"--threads={job.threads}",
                "--verbose=3",
            ]
        )
        if job.overwrite_tables:
            args.append("--overwrite-tables")

        return self.docker.command(args)

