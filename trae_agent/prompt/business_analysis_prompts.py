# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
"""
Business Analysis Prompts
企业级故障业务识别助手提示词
"""

from pathlib import Path


def _load_prompt(filename: str) -> str:
    prompt_path = Path(__file__).parent / "templates" / filename
    if not prompt_path.exists():
        raise FileNotFoundError(f"Prompt template not found: {prompt_path}")
    return prompt_path.read_text(encoding="utf-8")


_SYSTEM_PROMPT_TEMPLATE = _load_prompt("business_analysis.md")

USER_PROMPT_TEMPLATE = _load_prompt("business_analysis_user.md")


def get_system_prompt(doc_path: str = "doc") -> str:
    """获取系统提示词，替换 doc_path 占位符"""
    return _SYSTEM_PROMPT_TEMPLATE.replace("{doc_path}", doc_path)


def build_user_prompt(description: str, project_index: str, business_docs: dict | None = None) -> str:
    """构建用户提示词"""
    business_docs_section = ""
    if business_docs:
        business_docs_section = "\n相关业务文档：\n"
        for doc_name, doc_content in business_docs.items():
            business_docs_section += f"\n--- {doc_name} ---\n{doc_content}\n"
    
    return USER_PROMPT_TEMPLATE.format(
        description=description,
        project_index=project_index,
        business_docs_section=business_docs_section
    )
