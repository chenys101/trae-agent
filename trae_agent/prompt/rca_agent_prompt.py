# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

from pathlib import Path


def _load_prompt(filename: str) -> str:
    prompt_path = Path(__file__).parent / "templates" / filename
    if not prompt_path.exists():
        raise FileNotFoundError(f"Prompt template not found: {prompt_path}")
    return prompt_path.read_text(encoding="utf-8")


RCA_AGENT_SYSTEM_PROMPT = _load_prompt("rca_agent.md")
