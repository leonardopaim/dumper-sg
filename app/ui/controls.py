from __future__ import annotations

from PySide6.QtCore import Signal
from PySide6.QtWidgets import QHBoxLayout, QPushButton, QSpinBox, QWidget


class NumberInput(QWidget):
    valueChanged = Signal(int)

    def __init__(self, minimum: int, maximum: int, value: int, parent: QWidget | None = None) -> None:
        super().__init__(parent)
        self.input = QSpinBox()
        self.input.setRange(minimum, maximum)
        self.input.setValue(value)
        self.input.setButtonSymbols(QSpinBox.ButtonSymbols.NoButtons)
        self.input.setMinimumWidth(58)

        self.decrease_button = QPushButton("-")
        self.increase_button = QPushButton("+")
        for button in (self.decrease_button, self.increase_button):
            button.setFixedSize(28, 30)
            button.setFocusPolicy(self.focusPolicy())

        layout = QHBoxLayout(self)
        layout.setContentsMargins(0, 0, 0, 0)
        layout.setSpacing(4)
        layout.addWidget(self.decrease_button)
        layout.addWidget(self.input, 1)
        layout.addWidget(self.increase_button)

        self.decrease_button.clicked.connect(self.stepDown)
        self.increase_button.clicked.connect(self.stepUp)
        self.input.valueChanged.connect(self.valueChanged.emit)

    def setRange(self, minimum: int, maximum: int) -> None:
        self.input.setRange(minimum, maximum)

    def setValue(self, value: int) -> None:
        self.input.setValue(value)

    def value(self) -> int:
        return self.input.value()

    def stepUp(self) -> None:
        self.input.stepUp()

    def stepDown(self) -> None:
        self.input.stepDown()

