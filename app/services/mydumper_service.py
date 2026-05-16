from __future__ import annotations

from datetime import datetime
from pathlib import Path
import re

from models.backup_job import BackupJob
from services.docker_service import DockerCommand, DockerService


class MyDumperService:
    def __init__(self, docker_service: DockerService | None = None) -> None:
        self.docker = docker_service or DockerService()

    def build_command(self, job: BackupJob) -> tuple[DockerCommand, Path]:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        safe_db_name = re.sub(r"[^A-Za-z0-9_.-]+", "_", job.database).strip("_")
        backup_name = f"{safe_db_name}_{timestamp}"
        output_dir = job.destination_dir / backup_name

        args = self.docker.build_base_args(job.destination_dir)
        args.extend(
            [
                "mydumper",
                f"--host={job.host}",
                f"--port={job.port}",
                f"--user={job.user}",
                f"--password={job.password}",
                f"--database={job.database}",
                f"--outputdir=/backup/{backup_name}",
                f"--threads={job.threads}",
                "--verbose=3",
            ]
        )
        if job.compress:
            args.append("--compress")
        if job.ssl:
            args.append("--ssl")
        if job.non_locking:
            args.extend(
                [
                    "--sync-thread-lock-mode=NO_LOCK",
                    "--trx-tables",
                    "--skip-ddl-locks",
                    "--no-backup-locks",
                ]
            )
        if job.ignore_regex.strip():
            args.append(f"--regex=^(?!.*({job.ignore_regex})).*")

        return self.docker.command(args), output_dir

