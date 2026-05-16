from __future__ import annotations

import logging
from logging.handlers import RotatingFileHandler

from utils.paths import logs_dir


LOG_FORMAT = "%(asctime)s [%(levelname)s] %(name)s - %(message)s"


def _file_handler(file_name: str) -> RotatingFileHandler:
    handler = RotatingFileHandler(
        logs_dir() / file_name,
        maxBytes=5 * 1024 * 1024,
        backupCount=5,
        encoding="utf-8",
    )
    handler.setFormatter(logging.Formatter(LOG_FORMAT))
    return handler


def configure_logging() -> None:
    logging.basicConfig(level=logging.INFO, format=LOG_FORMAT)

    for logger_name, file_name in (
        ("app", "app.log"),
        ("backup", "backup.log"),
        ("restore", "restore.log"),
    ):
        logger = logging.getLogger(logger_name)
        logger.setLevel(logging.INFO)
        logger.propagate = False
        if not logger.handlers:
            logger.addHandler(_file_handler(file_name))


def get_logger(name: str) -> logging.Logger:
    return logging.getLogger(name)

