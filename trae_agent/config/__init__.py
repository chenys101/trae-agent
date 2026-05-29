import logging
import functools
from pathlib import Path
from typing import Any

import yaml

logger = logging.getLogger(__name__)

_CONFIG_DIR = Path(__file__).parent


@functools.lru_cache(maxsize=4)
def _load_yaml(filename: str) -> dict[str, Any]:
    path = _CONFIG_DIR / filename
    if not path.exists():
        logger.debug(f"Config file not found: {path}")
        return {}
    try:
        content = yaml.safe_load(path.read_text(encoding="utf-8"))
        return content if isinstance(content, dict) else {}
    except Exception as e:
        logger.warning(f"Failed to load config {path}: {e}")
        return {}


def load_conventions() -> dict[str, Any]:
    return _load_yaml("project_conventions.yaml")


def get_languages() -> list[dict[str, Any]]:
    conventions = load_conventions()
    return conventions.get("languages", [])


def get_container_naming() -> dict[str, Any]:
    conventions = load_conventions()
    return conventions.get("container_naming", {})
