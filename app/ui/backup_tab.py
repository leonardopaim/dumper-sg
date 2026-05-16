from __future__ import annotations

from pathlib import Path
import re
from time import monotonic

from PySide6.QtCore import QTimer, Signal
from PySide6.QtWidgets import (
    QCheckBox,
    QFileDialog,
    QGridLayout,
    QGroupBox,
    QHBoxLayout,
    QLabel,
    QLineEdit,
    QMessageBox,
    QProgressBar,
    QPushButton,
    QSplitter,
    QTableWidget,
    QTableWidgetItem,
    QVBoxLayout,
    QWidget,
)

from models.backup_job import BackupJob
from models.connection_profile import ConnectionProfile
from services.mydumper_service import MyDumperService
from services.mysql_service import MySqlService
from services.process_service import ProcessService
from storage.settings_repository import SettingsRepository
from ui.controls import NumberInput
from ui.logs_widget import LogsWidget
from utils.logger import get_logger
from utils.validators import validate_existing_directory, validate_regex, validate_required, validate_threads


class BackupTab(QWidget):
    profiles_changed = Signal()
    status_changed = Signal(str)

    def __init__(self, repository: SettingsRepository, parent: QWidget | None = None) -> None:
        super().__init__(parent)
        self.repository = repository
        self.mydumper = MyDumperService()
        self.mysql = MySqlService()
        self.process = ProcessService(self)
        self.test_process = ProcessService(self)
        self.current_history_id: int | None = None
        self.current_output_dir: Path | None = None
        self.loaded_profile_id: int | None = None
        self.started_at = 0.0
        self.current_progress = 0
        self.logger = get_logger("backup")

        self.elapsed_timer = QTimer(self)
        self.elapsed_timer.setInterval(1000)
        self.elapsed_timer.timeout.connect(self._tick_elapsed)

        self._build_ui()
        self._connect_process()
        self.refresh()

    def _build_ui(self) -> None:
        self.profile_name = QLineEdit()
        self.host = QLineEdit("127.0.0.1")
        self.port = NumberInput(1, 65535, 3306)
        self.user = QLineEdit("root")
        self.password = QLineEdit()
        self.password.setEchoMode(QLineEdit.EchoMode.Password)
        self.database = QLineEdit()
        self.destination = QLineEdit(self.repository.default_backup_dir())
        self.threads = NumberInput(1, 256, 8)
        self.compress = QCheckBox("Compressao")
        self.ssl = QCheckBox("SSL")
        self.non_locking = QCheckBox("Nao bloquear banco")
        self.non_locking.setChecked(True)
        self.non_locking.setToolTip(
            "Usa NO_LOCK, trx-tables e desabilita locks DDL/backup quando possivel. "
            "Recomendado para bases InnoDB em uso."
        )
        self.ignore_regex = QLineEdit()
        self.ignore_regex.setPlaceholderText("Ex.: ^tmp_|\\.log$")

        self.port.setFixedWidth(150)
        self.threads.setFixedWidth(130)

        form = QGridLayout()
        form.setContentsMargins(0, 4, 0, 0)
        form.setHorizontalSpacing(10)
        form.setVerticalSpacing(6)
        form.addWidget(QLabel("Perfil"), 0, 0)
        form.addWidget(QLabel("Host"), 0, 2)
        form.addWidget(QLabel("Porta"), 0, 4)
        form.addWidget(QLabel("Threads"), 0, 5)
        form.addWidget(self.profile_name, 1, 0, 1, 2)
        form.addWidget(self.host, 1, 2, 1, 2)
        form.addWidget(self.port, 1, 4)
        form.addWidget(self.threads, 1, 5)
        form.addWidget(QLabel("Usuario"), 2, 0)
        form.addWidget(QLabel("Senha"), 2, 2)
        form.addWidget(QLabel("Database"), 2, 4)
        form.addWidget(self.user, 3, 0, 1, 2)
        form.addWidget(self.password, 3, 2, 1, 2)
        form.addWidget(self.database, 3, 4, 1, 2)
        form.addWidget(QLabel("Destino"), 4, 0)
        form.addWidget(QLabel("Ignorar tabelas"), 4, 3)
        form.addWidget(self.destination, 5, 0, 1, 3)
        form.addWidget(self.ignore_regex, 5, 3, 1, 3)
        form.addWidget(self.compress, 6, 0)
        form.addWidget(self.ssl, 6, 1)
        form.addWidget(self.non_locking, 6, 2, 1, 2)
        form.setColumnStretch(0, 2)
        form.setColumnStretch(1, 2)
        form.setColumnStretch(2, 2)
        form.setColumnStretch(3, 2)
        form.setColumnStretch(4, 1)
        form.setColumnStretch(5, 1)

        self.select_folder_button = QPushButton("Selecionar pasta")
        self.test_button = QPushButton("Testar conexao")
        self.run_button = QPushButton("Executar backup")
        self.cancel_button = QPushButton("Cancelar backup")
        self.cancel_button.setEnabled(False)
        self.open_folder_button = QPushButton("Abrir pasta")

        buttons = QHBoxLayout()
        buttons.setSpacing(8)
        for button in (
            self.select_folder_button,
            self.test_button,
            self.open_folder_button,
            self.run_button,
            self.cancel_button,
        ):
            buttons.addWidget(button)
        buttons.addStretch()

        self.progress = QProgressBar()
        self.progress.setRange(0, 100)
        self.elapsed = QLabel("00:00:00")
        self.status = QLabel("Aguardando")

        status_grid = QGridLayout()
        status_grid.addWidget(QLabel("Status"), 0, 0)
        status_grid.addWidget(self.status, 0, 1)
        status_grid.addWidget(QLabel("Tempo decorrido"), 1, 0)
        status_grid.addWidget(self.elapsed, 1, 1)
        status_grid.addWidget(self.progress, 2, 0, 1, 2)

        box = QGroupBox("Backup MyDumper")
        box_layout = QVBoxLayout(box)
        box_layout.addLayout(form)
        box_layout.addLayout(buttons)
        box_layout.addLayout(status_grid)

        self.logs = LogsWidget()
        self.history = QTableWidget(0, 6)
        self.history.setHorizontalHeaderLabels(["ID", "Perfil", "Database", "Destino", "Status", "Inicio"])
        self.history.horizontalHeader().setStretchLastSection(True)
        self.history.setMinimumHeight(220)

        left_panel = QWidget()
        left_layout = QVBoxLayout(left_panel)
        left_layout.setContentsMargins(0, 0, 0, 0)
        left_layout.addWidget(box)
        left_layout.addWidget(QLabel("Historico de backups"))
        left_layout.addWidget(self.history, 1)

        logs_panel = QWidget()
        logs_layout = QVBoxLayout(logs_panel)
        logs_layout.setContentsMargins(0, 0, 0, 0)
        logs_layout.addWidget(QLabel("Logs em tempo real"))
        logs_layout.addWidget(self.logs, 1)

        splitter = QSplitter()
        splitter.addWidget(left_panel)
        splitter.addWidget(logs_panel)
        splitter.setSizes([760, 430])
        splitter.setChildrenCollapsible(False)

        layout = QVBoxLayout(self)
        layout.addWidget(splitter)

        self.select_folder_button.clicked.connect(self._select_folder)
        self.test_button.clicked.connect(self._test_connection)
        self.run_button.clicked.connect(self._run_backup)
        self.cancel_button.clicked.connect(self._cancel_backup)
        self.open_folder_button.clicked.connect(self._open_folder)

    def _connect_process(self) -> None:
        self.process.started.connect(self._backup_started)
        self.process.output_received.connect(self._handle_output)
        self.process.error_received.connect(lambda text: self._handle_output(text, error=True))
        self.process.finished.connect(self._backup_finished)

        self.test_process.output_received.connect(lambda text: self.logs.append(text.strip()))
        self.test_process.error_received.connect(lambda text: self.logs.append(text.strip(), "ERROR"))
        self.test_process.finished.connect(self._test_finished)

    def refresh(self) -> None:
        self._load_history()

    def load_profile(self, profile: ConnectionProfile) -> None:
        self.loaded_profile_id = profile.id
        self.profile_name.setText(profile.nome)
        self.host.setText(profile.host)
        self.port.setValue(profile.porta)
        self.user.setText(profile.usuario)
        self.password.setText(profile.senha)
        self.database.setText(profile.database)
        self.ssl.setChecked(profile.ssl)
        self.threads.setValue(profile.threads_default)

    def _profile_from_fields(self) -> ConnectionProfile:
        return ConnectionProfile(
            id=self.loaded_profile_id,
            nome=self.profile_name.text().strip() or "Manual",
            host=self.host.text().strip(),
            porta=self.port.value(),
            usuario=self.user.text().strip(),
            senha=self.password.text(),
            database=self.database.text().strip(),
            ssl=self.ssl.isChecked(),
            threads_default=self.threads.value(),
        )

    def _select_folder(self) -> None:
        folder = QFileDialog.getExistingDirectory(self, "Selecionar pasta destino", self.destination.text())
        if folder:
            self.destination.setText(folder)

    def _validate_backup(self) -> list[str]:
        errors = [
            validate_required(self.host.text(), "Host"),
            validate_required(self.user.text(), "Usuario"),
            validate_required(self.database.text(), "Database"),
            validate_existing_directory(self.destination.text(), "Pasta destino"),
            validate_threads(self.threads.value()),
            validate_regex(self.ignore_regex.text()),
        ]
        return [err for err in errors if err]

    def _test_connection(self) -> None:
        if self.test_process.is_running:
            return
        profile = self._profile_from_fields()
        command = self.mysql.build_test_connection_command(profile)
        self.logs.append(f"Executando teste: {command.display()}")
        self.test_button.setEnabled(False)
        self.test_process.start(command.program, command.args)

    def _test_finished(self, exit_code: int, _: str) -> None:
        self.test_button.setEnabled(True)
        if exit_code == 0:
            self.logs.append("Conexao MySQL validada.", "OK")
        else:
            self.logs.append("Falha ao conectar no MySQL.", "ERROR")

    def _run_backup(self) -> None:
        errors = self._validate_backup()
        if errors:
            QMessageBox.warning(self, "Validacao", "\n".join(errors))
            return

        job = BackupJob(
            profile_id=self.loaded_profile_id,
            profile_name=self.profile_name.text().strip() or "Manual",
            host=self.host.text().strip(),
            port=self.port.value(),
            user=self.user.text().strip(),
            password=self.password.text(),
            database=self.database.text().strip(),
            destination_dir=Path(self.destination.text()),
            threads=self.threads.value(),
            compress=self.compress.isChecked(),
            ssl=self.ssl.isChecked(),
            non_locking=self.non_locking.isChecked(),
            ignore_regex=self.ignore_regex.text().strip(),
        )
        command, output_dir = self.mydumper.build_command(job)
        self.current_output_dir = output_dir
        self.current_history_id = self.repository.add_backup_history(
            job.profile_id,
            job.profile_name,
            job.database,
            str(output_dir),
            "RUNNING",
            "Backup iniciado",
        )
        self.logs.append(f"Comando: {command.display()}")
        if job.non_locking:
            self.logs.append(
                "Modo sem bloqueio ativo: o backup nao solicita FTWRL/LOCK TABLE. "
                "Use preferencialmente com tabelas InnoDB; DDL durante o backup pode afetar consistencia.",
                "WARN",
            )
        self.logger.info("Backup iniciado: %s", command.display())
        self.process.start(command.program, command.args)

    def _backup_started(self) -> None:
        self.started_at = monotonic()
        self.current_progress = 0
        self.elapsed_timer.start()
        self.progress.setRange(0, 100)
        self._set_progress(2)
        self.status.setText("Executando backup")
        self.status_changed.emit("Backup em execucao")
        self.run_button.setEnabled(False)
        self.cancel_button.setEnabled(True)

    def _handle_output(self, text: str, error: bool = False) -> None:
        for line in text.splitlines():
            if not line.strip():
                continue
            level = "ERROR" if error or self._looks_like_error(line) else "INFO"
            self.logs.append(line, level)
            self.logger.info(line)
            progress = self._parse_progress(line)
            if progress is not None:
                self._set_progress(progress)
            else:
                self._advance_progress_from_activity()

    def _backup_finished(self, exit_code: int, status: str) -> None:
        self.elapsed_timer.stop()
        self.progress.setRange(0, 100)
        self._set_progress(100 if exit_code == 0 else self.current_progress)
        self.run_button.setEnabled(True)
        self.cancel_button.setEnabled(False)
        final_status = "SUCCESS" if exit_code == 0 else "FAILED"
        message = f"Processo finalizado com exit_code={exit_code}, status={status}"
        if self.current_history_id is not None:
            self.repository.finish_backup_history(self.current_history_id, final_status, message)
        self.status.setText("Concluido" if exit_code == 0 else "Falhou")
        self.status_changed.emit(self.status.text())
        self.logs.append(message, "OK" if exit_code == 0 else "ERROR")
        self.logger.info(message)
        self._load_history()

    def _cancel_backup(self) -> None:
        self.logs.append("Cancelando backup...", "WARN")
        self.process.cancel()

    def _open_folder(self) -> None:
        from PySide6.QtGui import QDesktopServices
        from PySide6.QtCore import QUrl

        path = self.current_output_dir if self.current_output_dir and self.current_output_dir.exists() else Path(self.destination.text())
        QDesktopServices.openUrl(QUrl.fromLocalFile(str(path)))

    def _tick_elapsed(self) -> None:
        seconds = int(monotonic() - self.started_at)
        h, rem = divmod(seconds, 3600)
        m, s = divmod(rem, 60)
        self.elapsed.setText(f"{h:02d}:{m:02d}:{s:02d}")
        if self.process.is_running:
            self._advance_progress_from_time(seconds)

    def _load_history(self) -> None:
        rows = self.repository.list_backup_history()
        self.history.setRowCount(len(rows))
        for row_index, row in enumerate(rows):
            values = [row["id"], row["profile_name"], row["database_name"], row["destination_dir"], row["status"], row["started_at"]]
            for col, value in enumerate(values):
                self.history.setItem(row_index, col, QTableWidgetItem(str(value or "")))

    def _parse_progress(self, line: str) -> int | None:
        match = re.search(r"(\d{1,3})\s*%", line)
        if not match:
            return None
        return max(0, min(100, int(match.group(1))))

    def _set_progress(self, value: int) -> None:
        value = max(self.current_progress, min(100, value))
        self.current_progress = value
        self.progress.setValue(value)

    def _advance_progress_from_activity(self) -> None:
        if self.current_progress < 85:
            self._set_progress(self.current_progress + 1)

    def _advance_progress_from_time(self, seconds: int) -> None:
        if seconds > 0 and seconds % 4 == 0 and self.current_progress < 90:
            self._set_progress(self.current_progress + 1)

    def _looks_like_error(self, line: str) -> bool:
        lowered = line.lower()
        return any(token in lowered for token in ("error", "failed", "denied", "refused", "timeout", "unknown"))
