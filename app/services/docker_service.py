from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from PySide6.QtCore import QObject, Signal

from services.process_service import ProcessService


MYDUMPER_IMAGE = "mydumper/mydumper:latest"


@dataclass(slots=True)
class DockerCommand:
    program: str
    args: list[str]

    def display(self) -> str:
        quoted = [self.program]
        for arg in self.args:
            if arg.startswith("--password="):
                arg = "--password=***"
            if " " in arg or "\\" in arg:
                quoted.append(f'"{arg}"')
            else:
                quoted.append(arg)
        return " ".join(quoted)


class DockerValidationService(QObject):
    output = Signal(str)
    finished = Signal(bool, str)

    def __init__(self, parent: QObject | None = None) -> None:
        super().__init__(parent)
        self.process = ProcessService(self)
        self.process.output_received.connect(self.output.emit)
        self.process.error_received.connect(self.output.emit)
        self.process.finished.connect(self._finished)

    def validate(self) -> None:
        self.process.start("docker", ["version", "--format", "{{.Server.Version}}"])

    def _finished(self, exit_code: int, _: str) -> None:
        if exit_code == 0:
            self.finished.emit(True, "Docker disponivel.")
        else:
            self.finished.emit(False, "Docker nao respondeu. Verifique Docker Desktop e WSL2.")


class DockerService:
    def __init__(self, image: str = MYDUMPER_IMAGE, use_host_network: bool = True) -> None:
        self.image = image
        self.use_host_network = use_host_network

    def build_base_args(self, host_path: Path, container_path: str = "/backup") -> list[str]:
        args = ["run", "--rm"]
        if self.use_host_network:
            args.extend(["--network", "host"])
        args.extend(["-v", f"{host_path.resolve()}:{container_path}", self.image])
        return args

    def command(self, docker_args: list[str]) -> DockerCommand:
        return DockerCommand("docker", docker_args)

