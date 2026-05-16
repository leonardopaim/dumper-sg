from __future__ import annotations

from pathlib import Path
import re
from time import monotonic

from PySide6.QtCore import QTimer, Signal
from PySide6.QtWidgets import (
    QCheckBox,
    QComboBox,
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

from models.backup_job import RestoreJob
from models.connection_profile import ConnectionProfile
from services.myloader_service import MyLoaderService
from services.mysql_service import MySqlService
from services.process_service import ProcessService
from storage.settings_repository import SettingsRepository
from ui.controls import NumberInput
from ui.logs_widget import LogsWidget
from utils.logger import get_logger
from utils.validators import validate_existing_directory, validate_local_mysql_host, validate_required, validate_threads


class RestoreTab(QWidget):
    status_changed = Signal(str)

    def __init__(self, repository: SettingsRepository, parent: QWidget | None = None) -> None:
        super().__init__(parent)
        self.repository = repository
        self.myloader = MyLoaderService()
        self.mysql = MySqlService()
        self.process = ProcessService(self)
        self.create_db_process = ProcessService(self)
        self.current_history_id: int | None = None
        self.profiles: list[ConnectionProfile] = []
        self.started_at = 0.0
        self.current_progress = 0
        self.logger = get_logger("restore")

        self.elapsed_timer = QTimer(self)
        self.elapsed_timer.setInterval(1000)
        self.elapsed_timer.timeout.connect(self._tick_elapsed)

        self._build_ui()
        self._connect_process()
        self.refresh()

    def _build_ui(self) -> None:
        self.profile_combo = QComboBox()
        self.backup_dir = QLineEdit()
        self.target_database = QLineEdit()
        self.threads = NumberInput(1, 256, 8)
        self.overwrite = QCheckBox("Overwrite tables")

        self.threads.setFixedWidth(130)

        form = QGridLayout()
        form.setContentsMargins(0, 4, 0, 0)
        form.setHorizontalSpacing(10)
        form.setVerticalSpacing(6)
        form.addWidget(QLabel("Perfil"), 0, 0)
        form.addWidget(QLabel("Threads"), 0, 4)
        form.addWidget(self.profile_combo, 1, 0, 1, 4)
        form.addWidget(self.threads, 1, 4)
        form.addWidget(QLabel("Backup"), 2, 0)
        form.addWidget(self.backup_dir, 3, 0, 1, 5)
        form.addWidget(QLabel("Database destino"), 4, 0)
        form.addWidget(self.target_database, 5, 0, 1, 4)
        form.addWidget(self.overwrite, 5, 4)
        form.setColumnStretch(0, 2)
        form.setColumnStretch(1, 2)
        form.setColumnStretch(2, 2)
        form.setColumnStretch(3, 2)
        form.setColumnStretch(4, 1)

        self.select_backup_button = QPushButton("Selecionar backup")
        self.create_database_button = QPushButton("Criar database")
        self.run_button = QPushButton("Executar restore")
        self.cancel_button = QPushButton("Cancelar restore")
        self.cancel_button.setEnabled(False)

        buttons = QHBoxLayout()
        buttons.setSpacing(8)
        for button in (
            self.select_backup_button,
            self.create_database_button,
            self.run_button,
            self.cancel_button,
        ):
            buttons.addWidget(button)
        buttons.addStretch()

        self.progress = QProgressBar()
        self.progress.setRange(0, 100)
        self.elapsed = QLabel("00:00:00")
        self.eta = QLabel("Aguardando amostras de log")
        self.status = QLabel("Aguardando")

        status_grid = QGridLayout()
        status_grid.addWidget(QLabel("Status"), 0, 0)
        status_grid.addWidget(self.status, 0, 1)
        status_grid.addWidget(QLabel("Tempo decorrido"), 1, 0)
        status_grid.addWidget(self.elapsed, 1, 1)
        status_grid.addWidget(QLabel("Tempo estimado"), 2, 0)
        status_grid.addWidget(self.eta, 2, 1)
        status_grid.addWidget(self.progress, 3, 0, 1, 2)

        box = QGroupBox("Restore MyLoader")
        box_layout = QVBoxLayout(box)
        box_layout.addLayout(form)
        box_layout.addLayout(buttons)
        box_layout.addLayout(status_grid)

        self.logs = LogsWidget()
        self.history = QTableWidget(0, 6)
        self.history.setHorizontalHeaderLabels(["ID", "Perfil", "Origem", "Database", "Status", "Inicio"])
        self.history.horizontalHeader().setStretchLastSection(True)
        self.history.setMinimumHeight(220)

        left_panel = QWidget()
        left_layout = QVBoxLayout(left_panel)
        left_layout.setContentsMargins(0, 0, 0, 0)
        left_layout.addWidget(box)
        left_layout.addWidget(QLabel("Historico de restores"))
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

        self.profile_combo.currentIndexChanged.connect(self._profile_changed)
        self.select_backup_button.clicked.connect(self._select_backup)
        self.create_database_button.clicked.connect(self._create_database)
        self.run_button.clicked.connect(self._run_restore)
        self.cancel_button.clicked.connect(self._cancel_restore)

    def _connect_process(self) -> None:
        self.process.started.connect(self._restore_started)
        self.process.output_received.connect(self._handle_output)
        self.process.error_received.connect(lambda text: self._handle_output(text, error=True))
        self.process.finished.connect(self._restore_finished)

        self.create_db_process.output_received.connect(lambda text: self.logs.append(text.strip()))
        self.create_db_process.error_received.connect(lambda text: self.logs.append(text.strip(), "ERROR"))
        self.create_db_process.finished.connect(self._create_database_finished)

    def refresh(self) -> None:
        current_id = self.current_profile().id if self.current_profile() else None
        self.profiles = self.repository.list_profiles()
        self.profile_combo.blockSignals(True)
        self.profile_combo.clear()
        for profile in self.profiles:
            self.profile_combo.addItem(profile.label, profile.id)
        self.profile_combo.blockSignals(False)
        if current_id is not None:
            index = self.profile_combo.findData(current_id)
            if index >= 0:
                self.profile_combo.setCurrentIndex(index)
        self._profile_changed()
        self._load_history()

    def current_profile(self) -> ConnectionProfile | None:
        index = self.profile_combo.currentIndex()
        if index < 0 or index >= len(self.profiles):
            return None
        return self.profiles[index]

    def _profile_changed(self) -> None:
        profile = self.current_profile()
        if not profile:
            return
        self.threads.setValue(profile.threads_default)
        if profile.database and not self.target_database.text().strip():
            self.target_database.setText(f"{profile.database}_restore")

    def _select_backup(self) -> None:
        folder = QFileDialog.getExistingDirectory(self, "Selecionar pasta de backup", self.repository.default_backup_dir())
        if folder:
            self.backup_dir.setText(folder)

    def _validate_restore(self) -> list[str]:
        profile = self.current_profile()
        target_database = self.target_database.text().strip()
        errors = [
            "Selecione um perfil." if profile is None else None,
            validate_local_mysql_host(profile.host) if profile else None,
            validate_existing_directory(self.backup_dir.text(), "Pasta do backup"),
            validate_required(target_database, "Database destino"),
            validate_threads(self.threads.value()),
        ]
        if profile and profile.database and target_database.lower() == profile.database.lower():
            errors.append(
                "Restore bloqueado: o database destino e igual ao database padrao do perfil. "
                "Use um database isolado para nao impactar usuarios em producao."
            )
        return [err for err in errors if err]

    def _create_database(self) -> None:
        profile = self.current_profile()
        if profile is None:
            QMessageBox.warning(self, "Validacao", "Selecione um perfil.")
            return
        if validate_required(self.target_database.text(), "Database destino"):
            QMessageBox.warning(self, "Validacao", "Database destino e obrigatorio.")
            return
        command = self.mysql.build_create_database_command(profile, self.target_database.text().strip())
        self.logs.append(f"Comando: {command.display()}")
        self.create_database_button.setEnabled(False)
        self.create_db_process.start(command.program, command.args)

    def _create_database_finished(self, exit_code: int, _: str) -> None:
        self.create_database_button.setEnabled(True)
        if exit_code == 0:
            self.logs.append("Database criado ou ja existente.", "OK")
        else:
            self.logs.append("Falha ao criar database.", "ERROR")

    def _run_restore(self) -> None:
        errors = self._validate_restore()
        if errors:
            QMessageBox.warning(self, "Validacao", "\n".join(errors))
            return

        profile = self.current_profile()
        assert profile is not None
        job = RestoreJob(
            profile_id=profile.id,
            profile_name=profile.nome,
            host=profile.host,
            port=profile.porta,
            user=profile.usuario,
            password=profile.senha,
            backup_dir=Path(self.backup_dir.text()),
            target_database=self.target_database.text().strip(),
            threads=self.threads.value(),
            overwrite_tables=self.overwrite.isChecked(),
        )
        command = self.myloader.build_command(job)
        self.current_history_id = self.repository.add_restore_history(
            job.profile_id,
            job.profile_name,
            str(job.backup_dir),
            job.target_database,
            "RUNNING",
            "Restore iniciado",
        )
        self.logs.append(f"Comando: {command.display()}")
        self.logger.info("Restore iniciado: %s", command.display())
        self.process.start(command.program, command.args)

    def _restore_started(self) -> None:
        self.started_at = monotonic()
        self.current_progress = 0
        self.elapsed_timer.start()
        self.progress.setRange(0, 100)
        self._set_progress(2)
        self.status.setText("Executando restore")
        self.status_changed.emit("Restore em execucao")
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
                self._update_eta(progress)
            else:
                self._advance_progress_from_activity()
                self._update_eta(self.current_progress)

    def _restore_finished(self, exit_code: int, status: str) -> None:
        self.elapsed_timer.stop()
        self.progress.setRange(0, 100)
        self._set_progress(100 if exit_code == 0 else self.current_progress)
        self.run_button.setEnabled(True)
        self.cancel_button.setEnabled(False)
        final_status = "SUCCESS" if exit_code == 0 else "FAILED"
        message = f"Processo finalizado com exit_code={exit_code}, status={status}"
        if self.current_history_id is not None:
            self.repository.finish_restore_history(self.current_history_id, final_status, message)
        self.status.setText("Concluido" if exit_code == 0 else "Falhou")
        self.status_changed.emit(self.status.text())
        self.logs.append(message, "OK" if exit_code == 0 else "ERROR")
        self.logger.info(message)
        self._load_history()

    def _cancel_restore(self) -> None:
        self.logs.append("Cancelando restore...", "WARN")
        self.process.cancel()

    def _tick_elapsed(self) -> None:
        seconds = int(monotonic() - self.started_at)
        h, rem = divmod(seconds, 3600)
        m, s = divmod(rem, 60)
        self.elapsed.setText(f"{h:02d}:{m:02d}:{s:02d}")
        if self.process.is_running:
            self._advance_progress_from_time(seconds)
            self._update_eta(self.current_progress)

    def _update_eta(self, progress: int) -> None:
        if progress <= 0:
            return
        elapsed = monotonic() - self.started_at
        total = elapsed / (progress / 100)
        remaining = max(0, int(total - elapsed))
        h, rem = divmod(remaining, 3600)
        m, s = divmod(rem, 60)
        self.eta.setText(f"{h:02d}:{m:02d}:{s:02d}")

    def _load_history(self) -> None:
        rows = self.repository.list_restore_history()
        self.history.setRowCount(len(rows))
        for row_index, row in enumerate(rows):
            values = [row["id"], row["profile_name"], row["source_dir"], row["database_name"], row["status"], row["started_at"]]
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

