from __future__ import annotations

from PySide6.QtCore import Qt
from PySide6.QtWidgets import (
    QFrame,
    QHBoxLayout,
    QLabel,
    QMainWindow,
    QPushButton,
    QStackedWidget,
    QStatusBar,
    QVBoxLayout,
    QWidget,
)

from services.docker_service import DockerValidationService
from storage.database import Database
from storage.settings_repository import SettingsRepository
from ui.backup_tab import BackupTab
from ui.restore_tab import RestoreTab
from ui.settings_tab import SettingsTab


class MainWindow(QMainWindow):
    def __init__(self, db: Database) -> None:
        super().__init__()
        self.setWindowTitle("DumperSG - MySQL Backup & Restore")
        self.repository = SettingsRepository(db)
        self.docker_validation = DockerValidationService(self)

        self.backup_tab = BackupTab(self.repository)
        self.restore_tab = RestoreTab(self.repository)
        self.settings_tab = SettingsTab(self.repository)

        self._build_ui()
        self._connect()
        self._validate_docker()

    def _build_ui(self) -> None:
        self.sidebar = QFrame()
        self.sidebar.setObjectName("sidebar")
        self.sidebar.setFixedWidth(220)
        self.sidebar.setStyleSheet(
            """
            QFrame#sidebar { background: #0b0f14; border-right: 1px solid #232d38; }
            QPushButton {
                text-align: left;
                padding: 12px 14px;
                border: 0;
                border-radius: 4px;
                background: transparent;
                color: #c9d2df;
            }
            QPushButton:hover { background: #17202b; }
            QPushButton:checked { background: #1f3554; color: #ffffff; }
            QLabel#brand { color: #ffffff; font-size: 18pt; font-weight: 700; }
            QLabel#subtitle { color: #8391a5; }
            """
        )

        brand = QLabel("DumperSG")
        brand.setObjectName("brand")
        subtitle = QLabel("MyDumper DevOps")
        subtitle.setObjectName("subtitle")

        self.backup_button = QPushButton("Backup")
        self.restore_button = QPushButton("Restore")
        self.settings_button = QPushButton("Configuracoes")
        for button in (self.backup_button, self.restore_button, self.settings_button):
            button.setCheckable(True)

        side_layout = QVBoxLayout(self.sidebar)
        side_layout.setContentsMargins(16, 22, 16, 16)
        side_layout.addWidget(brand)
        side_layout.addWidget(subtitle)
        side_layout.addSpacing(24)
        side_layout.addWidget(self.backup_button)
        side_layout.addWidget(self.restore_button)
        side_layout.addWidget(self.settings_button)
        side_layout.addStretch()

        self.stack = QStackedWidget()
        self.stack.addWidget(self.backup_tab)
        self.stack.addWidget(self.restore_tab)
        self.stack.addWidget(self.settings_tab)

        central = QWidget()
        layout = QHBoxLayout(central)
        layout.setContentsMargins(0, 0, 0, 0)
        layout.addWidget(self.sidebar)
        layout.addWidget(self.stack)
        self.setCentralWidget(central)

        self.status = QStatusBar()
        self.setStatusBar(self.status)
        self.status.showMessage("Inicializando")

        self._select_tab(0)

    def _connect(self) -> None:
        self.backup_button.clicked.connect(lambda: self._select_tab(0))
        self.restore_button.clicked.connect(lambda: self._select_tab(1))
        self.settings_button.clicked.connect(lambda: self._select_tab(2))

        self.settings_tab.profiles_updated.connect(self.restore_tab.refresh)
        self.settings_tab.profiles_updated.connect(self.backup_tab.refresh)
        self.settings_tab.profile_selected.connect(self.backup_tab.load_profile)
        self.settings_tab.profile_selected.connect(lambda _: self._select_tab(0))

        self.backup_tab.status_changed.connect(self.status.showMessage)
        self.restore_tab.status_changed.connect(self.status.showMessage)
        self.settings_tab.status_changed.connect(self.status.showMessage)

        self.docker_validation.output.connect(lambda text: self.status.showMessage(text.strip()))
        self.docker_validation.finished.connect(self._docker_validation_finished)

    def _select_tab(self, index: int) -> None:
        self.stack.setCurrentIndex(index)
        for i, button in enumerate((self.backup_button, self.restore_button, self.settings_button)):
            button.setChecked(i == index)
        titles = ("Backup", "Restore", "Configuracoes")
        self.status.showMessage(titles[index])

    def _validate_docker(self) -> None:
        self.status.showMessage("Validando Docker Desktop")
        self.docker_validation.validate()

    def _docker_validation_finished(self, ok: bool, message: str) -> None:
        self.status.showMessage(message)
        target_log = self.backup_tab.logs
        target_log.append(message, "OK" if ok else "ERROR")

    def keyPressEvent(self, event) -> None:
        if event.key() == Qt.Key.Key_Escape:
            self.status.showMessage("Operacao mantida. Use o botao Cancelar para interromper processos.")
            return
        super().keyPressEvent(event)

