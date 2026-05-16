from __future__ import annotations

from datetime import datetime

from PySide6.QtGui import QTextCursor
from PySide6.QtWidgets import QHBoxLayout, QPushButton, QTextEdit, QVBoxLayout, QWidget


class LogsWidget(QWidget):
    def __init__(self, parent: QWidget | None = None) -> None:
        super().__init__(parent)
        self.text = QTextEdit()
        self.text.setReadOnly(True)
        self.text.setMinimumHeight(120)

        self.clear_button = QPushButton("Limpar")
        self.clear_button.clicked.connect(self.text.clear)

        actions = QHBoxLayout()
        actions.addStretch()
        actions.addWidget(self.clear_button)

        layout = QVBoxLayout(self)
        layout.setContentsMargins(0, 0, 0, 0)
        layout.addWidget(self.text)
        layout.addLayout(actions)

    def append(self, message: str, level: str = "INFO") -> None:
        color = {
            "INFO": "#d7dde5",
            "WARN": "#ffd166",
            "ERROR": "#ff6b6b",
            "OK": "#5ce1a9",
        }.get(level, "#d7dde5")
        now = datetime.now().strftime("%H:%M:%S")
        escaped = message.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        self.text.append(f'<span style="color:#8391a5">{now}</span> <span style="color:{color}">{escaped}</span>')
        self.text.moveCursor(QTextCursor.MoveOperation.End)

