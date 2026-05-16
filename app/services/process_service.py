from __future__ import annotations

from PySide6.QtCore import QObject, QProcess, QTimer, Signal


class ProcessService(QObject):
    started = Signal()
    output_received = Signal(str)
    error_received = Signal(str)
    finished = Signal(int, str)

    def __init__(self, parent: QObject | None = None) -> None:
        super().__init__(parent)
        self.process = QProcess(self)
        self.process.setProcessChannelMode(QProcess.ProcessChannelMode.SeparateChannels)
        self.process.started.connect(self.started.emit)
        self.process.readyReadStandardOutput.connect(self._read_stdout)
        self.process.readyReadStandardError.connect(self._read_stderr)
        self.process.finished.connect(self._finished)
        self.process.errorOccurred.connect(self._error_occurred)

    def start(self, program: str, arguments: list[str]) -> None:
        if self.is_running:
            raise RuntimeError("Ja existe um processo em execucao.")
        self.process.start(program, arguments)

    def cancel(self) -> None:
        if not self.is_running:
            return
        self.process.terminate()
        QTimer.singleShot(3000, self._kill_if_running)

    @property
    def is_running(self) -> bool:
        return self.process.state() != QProcess.ProcessState.NotRunning

    def _read_stdout(self) -> None:
        data = bytes(self.process.readAllStandardOutput()).decode("utf-8", errors="replace")
        if data:
            self.output_received.emit(data)

    def _read_stderr(self) -> None:
        data = bytes(self.process.readAllStandardError()).decode("utf-8", errors="replace")
        if data:
            self.error_received.emit(data)

    def _finished(self, exit_code: int, exit_status: QProcess.ExitStatus) -> None:
        status = "normal" if exit_status == QProcess.ExitStatus.NormalExit else "crash"
        self.finished.emit(exit_code, status)

    def _error_occurred(self, error: QProcess.ProcessError) -> None:
        self.error_received.emit(f"Erro do processo: {error.name}")
        if error == QProcess.ProcessError.FailedToStart:
            self.finished.emit(-1, "failed_to_start")

    def _kill_if_running(self) -> None:
        if self.is_running:
            self.process.kill()
