from __future__ import annotations

import sys

from PySide6.QtWidgets import QApplication

from storage.database import Database
from ui.main_window import MainWindow
from utils.logger import configure_logging, get_logger
from utils.paths import ensure_app_directories

def apply_theme(app: QApplication) -> None:
    input_fix = """
        QLineEdit, QSpinBox, QComboBox {
            min-height: 30px;
        }
        QSpinBox {
            padding: 4px 8px;
        }
    """
    try:
        import qdarktheme

        qdarktheme.setup_theme(
            "dark",
            custom_colors={
                "primary": "#4f8cff",
                "background": "#101418",
                "foreground": "#d7dde5",
            },
        )
        app.setStyleSheet(app.styleSheet() + input_fix)
    except Exception:
        app.setStyleSheet(
            """
            QWidget {
                background: #101418;
                color: #d7dde5;
                font-family: Segoe UI, Arial;
                font-size: 10pt;
            }
            QLineEdit, QSpinBox, QComboBox, QTextEdit {
                background: #171d24;
                border: 1px solid #2a3440;
                border-radius: 4px;
                padding: 6px;
            }
            QPushButton {
                background: #253143;
                border: 1px solid #34445a;
                border-radius: 4px;
                padding: 7px 12px;
            }
            QPushButton:hover { background: #314158; }
            QPushButton:disabled { color: #737f8c; background: #1b222b; }
            QProgressBar {
                border: 1px solid #2a3440;
                border-radius: 4px;
                text-align: center;
                background: #171d24;
            }
            QProgressBar::chunk { background: #4f8cff; }
            """
            + input_fix
        )


def main() -> int:
    ensure_app_directories()
    configure_logging()

    app = QApplication(sys.argv)
    app.setApplicationName("DumperSG")
    app.setOrganizationName("Sommus")
    apply_theme(app)

    db = Database()
    db.initialize()

    window = MainWindow(db)
    window.resize(1320, 820)
    window.show()

    get_logger("app").info("Aplicacao iniciada")
    return app.exec()


if __name__ == "__main__":
    raise SystemExit(main())

