from __future__ import annotations

import sqlite3

from PySide6.QtCore import Signal
from PySide6.QtWidgets import (
    QCheckBox,
    QFileDialog,
    QGridLayout,
    QGroupBox,
    QHBoxLayout,
    QLabel,
    QLineEdit,
    QListWidget,
    QMessageBox,
    QPushButton,
    QSplitter,
    QVBoxLayout,
    QWidget,
)

from models.connection_profile import ConnectionProfile
from storage.settings_repository import SettingsRepository
from ui.controls import NumberInput
from utils.validators import validate_existing_directory, validate_port, validate_required, validate_threads


class SettingsTab(QWidget):
    profiles_updated = Signal()
    profile_selected = Signal(ConnectionProfile)
    status_changed = Signal(str)

    def __init__(self, repository: SettingsRepository, parent: QWidget | None = None) -> None:
        super().__init__(parent)
        self.repository = repository
        self.profiles: list[ConnectionProfile] = []
        self.selected_profile_id: int | None = None
        self._build_ui()
        self.refresh()

    def _build_ui(self) -> None:
        self.profile_list = QListWidget()

        self.nome = QLineEdit()
        self.host = QLineEdit("127.0.0.1")
        self.porta = NumberInput(1, 65535, 3306)
        self.porta.setFixedWidth(150)
        self.usuario = QLineEdit("root")
        self.senha = QLineEdit()
        self.senha.setEchoMode(QLineEdit.EchoMode.Password)
        self.database = QLineEdit()
        self.ssl = QCheckBox("Usar SSL por padrao")
        self.threads = NumberInput(1, 256, 8)
        self.threads.setFixedWidth(130)

        form = QGridLayout()
        form.setContentsMargins(0, 4, 0, 0)
        form.setHorizontalSpacing(10)
        form.setVerticalSpacing(6)
        form.addWidget(QLabel("Nome"), 0, 0)
        form.addWidget(QLabel("Host"), 0, 2)
        form.addWidget(QLabel("Porta"), 0, 4)
        form.addWidget(self.nome, 1, 0, 1, 2)
        form.addWidget(self.host, 1, 2, 1, 2)
        form.addWidget(self.porta, 1, 4)
        form.addWidget(QLabel("Usuario"), 2, 0)
        form.addWidget(QLabel("Senha"), 2, 2)
        form.addWidget(QLabel("Threads"), 2, 4)
        form.addWidget(self.usuario, 3, 0, 1, 2)
        form.addWidget(self.senha, 3, 2, 1, 2)
        form.addWidget(self.threads, 3, 4)
        form.addWidget(QLabel("Database"), 4, 0)
        form.addWidget(self.database, 5, 0, 1, 4)
        form.addWidget(self.ssl, 5, 4)
        form.setColumnStretch(0, 2)
        form.setColumnStretch(1, 2)
        form.setColumnStretch(2, 2)
        form.setColumnStretch(3, 2)
        form.setColumnStretch(4, 1)

        self.new_button = QPushButton("Novo")
        self.save_button = QPushButton("Salvar")
        self.delete_button = QPushButton("Remover")
        self.use_button = QPushButton("Usar no backup")

        self.feedback = QLabel("")
        self.feedback.setMinimumHeight(24)
        self.feedback.setStyleSheet("color: #8391a5;")

        actions = QHBoxLayout()
        for button in (self.new_button, self.save_button, self.delete_button, self.use_button):
            actions.addWidget(button)
        actions.addStretch()

        profile_box = QGroupBox("Perfis de conexao")
        profile_layout = QVBoxLayout(profile_box)
        profile_layout.addLayout(form)
        profile_layout.addLayout(actions)
        profile_layout.addWidget(self.feedback)

        self.default_backup_dir = QLineEdit(self.repository.default_backup_dir())
        self.select_default_button = QPushButton("Selecionar")
        self.save_default_button = QPushButton("Salvar pasta padrao")

        default_row = QHBoxLayout()
        default_row.addWidget(self.default_backup_dir)
        default_row.addWidget(self.select_default_button)
        default_row.addWidget(self.save_default_button)

        defaults_box = QGroupBox("Configuracoes gerais")
        defaults_layout = QVBoxLayout(defaults_box)
        defaults_layout.addWidget(QLabel("Pasta padrao de backups"))
        defaults_layout.addLayout(default_row)

        right = QWidget()
        right_layout = QVBoxLayout(right)
        right_layout.addWidget(profile_box)
        right_layout.addWidget(defaults_box)
        right_layout.addStretch()

        splitter = QSplitter()
        splitter.addWidget(self.profile_list)
        splitter.addWidget(right)
        splitter.setSizes([280, 820])

        layout = QVBoxLayout(self)
        layout.addWidget(splitter)

        self.profile_list.currentRowChanged.connect(self._list_selection_changed)
        self.new_button.clicked.connect(self._new_profile)
        self.save_button.clicked.connect(self._save_profile)
        self.delete_button.clicked.connect(self._delete_profile)
        self.use_button.clicked.connect(self._emit_selected)
        self.select_default_button.clicked.connect(self._select_default_backup_dir)
        self.save_default_button.clicked.connect(self._save_default_backup_dir)

    def refresh(self, select_profile_id: int | None = None) -> None:
        self.profiles = self.repository.list_profiles()
        self.profile_list.clear()
        for profile in self.profiles:
            self.profile_list.addItem(profile.label)
        if self.profiles:
            row_to_select = 0
            if select_profile_id is not None:
                for index, profile in enumerate(self.profiles):
                    if profile.id == select_profile_id:
                        row_to_select = index
                        break
            self.profile_list.setCurrentRow(row_to_select)
        else:
            self._new_profile()

    def _list_selection_changed(self, row: int) -> None:
        if row < 0 or row >= len(self.profiles):
            return
        profile = self.profiles[row]
        self.selected_profile_id = profile.id
        self.nome.setText(profile.nome)
        self.host.setText(profile.host)
        self.porta.setValue(profile.porta)
        self.usuario.setText(profile.usuario)
        self.senha.setText(profile.senha)
        self.database.setText(profile.database)
        self.ssl.setChecked(profile.ssl)
        self.threads.setValue(profile.threads_default)

    def _new_profile(self) -> None:
        self.selected_profile_id = None
        self.profile_list.clearSelection()
        self.nome.clear()
        self.host.setText("127.0.0.1")
        self.porta.setValue(3306)
        self.usuario.setText("root")
        self.senha.clear()
        self.database.clear()
        self.ssl.setChecked(False)
        self.threads.setValue(8)

    def _profile_from_fields(self) -> ConnectionProfile:
        return ConnectionProfile(
            id=self.selected_profile_id,
            nome=self.nome.text().strip(),
            host=self.host.text().strip(),
            porta=self.porta.value(),
            usuario=self.usuario.text().strip(),
            senha=self.senha.text(),
            database=self.database.text().strip(),
            ssl=self.ssl.isChecked(),
            threads_default=self.threads.value(),
        )

    def _validate_profile(self) -> list[str]:
        errors = [
            validate_required(self.nome.text(), "Nome"),
            validate_required(self.host.text(), "Host"),
            validate_required(self.usuario.text(), "Usuario"),
            validate_port(self.porta.value()),
            validate_threads(self.threads.value()),
        ]
        return [err for err in errors if err]

    def _save_profile(self) -> None:
        errors = self._validate_profile()
        if errors:
            self._show_failure("Nao foi possivel salvar o perfil.", "\n".join(errors))
            return
        try:
            profile_id = self.repository.save_profile(self._profile_from_fields())
        except sqlite3.IntegrityError:
            self._show_failure("Nao foi possivel salvar o perfil.", "Ja existe um perfil com esse nome.")
            return
        self.selected_profile_id = profile_id
        self.status_changed.emit("Perfil salvo")
        self.profiles_updated.emit()
        self.refresh(select_profile_id=profile_id)
        self._show_success("Perfil salvo com sucesso.")

    def _delete_profile(self) -> None:
        if self.selected_profile_id is None:
            self._show_failure("Nao foi possivel remover o perfil.", "Selecione um perfil primeiro.")
            return
        answer = QMessageBox.question(self, "Remover perfil", "Remover o perfil selecionado?")
        if answer != QMessageBox.StandardButton.Yes:
            return
        self.repository.delete_profile(self.selected_profile_id)
        self.status_changed.emit("Perfil removido")
        self.profiles_updated.emit()
        self.refresh()
        self._show_success("Perfil removido com sucesso.")

    def _emit_selected(self) -> None:
        if self.selected_profile_id is None:
            self._show_failure("Nao foi possivel carregar o perfil.", "Selecione um perfil primeiro.")
            return
        profile = self.repository.get_profile(self.selected_profile_id)
        if profile:
            self.profile_selected.emit(profile)
            self.status_changed.emit("Perfil carregado na aba Backup")
            self._show_success("Perfil carregado na aba Backup.")

    def _select_default_backup_dir(self) -> None:
        folder = QFileDialog.getExistingDirectory(self, "Selecionar pasta padrao", self.default_backup_dir.text())
        if folder:
            self.default_backup_dir.setText(folder)

    def _save_default_backup_dir(self) -> None:
        error = validate_existing_directory(self.default_backup_dir.text(), "Pasta padrao de backups")
        if error:
            self._show_failure("Nao foi possivel salvar a pasta padrao.", error)
            return
        self.repository.set_setting("default_backup_dir", self.default_backup_dir.text().strip())
        self.status_changed.emit("Pasta padrao salva")
        self._show_success("Pasta padrao de backups salva com sucesso.")

    def _show_success(self, message: str) -> None:
        self.feedback.setStyleSheet("color: #5ce1a9;")
        self.feedback.setText(message)
        QMessageBox.information(self, "Sucesso", message)

    def _show_failure(self, title: str, message: str) -> None:
        self.feedback.setStyleSheet("color: #ff6b6b;")
        self.feedback.setText(f"{title} {message}")
        QMessageBox.warning(self, title, message)

