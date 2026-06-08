# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""Business Analysis Tool
企业级故障业务识别路由助手
"""
import logging
import json
import re
from typing import override, Any, Dict, List, Optional, Tuple
from pathlib import Path

from trae_agent.tools.base import Tool, ToolParameter, ToolCallArguments, ToolExecResult
from trae_agent.utils.llm_clients.llm_client import LLMClient
from trae_agent.utils.llm_clients.llm_basics import LLMMessage
from trae_agent.utils.config import Config
from trae_agent.prompt.business_analysis_prompts import (
    get_system_prompt,
    build_user_prompt
)
from trae_agent.models.business_models import FaultBusinessResult
from trae_agent.config import get_languages, get_container_naming

logger = logging.getLogger(__name__)


def load_project_index(codebase: Optional[str], doc_path: str = "doc") -> tuple[Optional[str], Optional[str]]:
    """加载 project-index.md 内容
    
    Args:
        codebase: 代码库路径
        doc_path: 文档目录路径，相对于 codebase，默认为 "doc"
    
    Returns:
        (content, relative_path)
    """
    if not codebase:
        return None, None

    codebase_path = Path(codebase)
    doc_dir = doc_path
    codebase_index_name = f"{doc_path}/codebase-index.md"
    
    doc_full_path = codebase_path / doc_dir
    
    codebase_index = codebase_path / codebase_index_name
    if codebase_index.exists():
        try:
            logger.info(f"Found codebase-index.md at: {codebase_index}")
            rel_path = str(codebase_index.relative_to(codebase_path))
            return codebase_index.read_text(encoding='utf-8'), rel_path
        except Exception as e:
            logger.warning(f"Failed to load codebase-index.md: {e}")
    
    # 2. 找 doc/*/ 下的 project-index.md
    if doc_full_path.exists():
        for subdir in doc_full_path.iterdir():
            if subdir.is_dir():
                project_index = subdir / "project-index.md"
                if project_index.exists():
                    try:
                        logger.info(f"Found project-index.md at: {project_index}")
                        rel_path = str(project_index.relative_to(codebase_path))
                        return project_index.read_text(encoding='utf-8'), rel_path
                    except Exception as e:
                        logger.warning(f"Failed to load {project_index}: {e}")
    
    logger.warning(f"No project index found in {doc_full_path}")
    return None, None


def validate_and_fix_code_locations(codebase: Optional[str], code_locations: List[str]) -> List[str]:
    """验证并修正 code_locations 路径，确保文件实际存在"""
    if not codebase or not code_locations:
        return code_locations
    
    fixed_locations = []
    codebase_path = Path(codebase)
    
    for code_loc in code_locations:
        original_path = Path(code_loc)
        
        # 1. 直接尝试
        full_path = codebase_path / code_loc
        if full_path.exists():
            fixed_locations.append(code_loc)
            continue
        
        # 2. 尝试在 doc/ 目录下查找
        doc_path = codebase_path / "doc" / code_loc
        if doc_path.exists():
            fixed_locations.append(str(doc_path.relative_to(codebase_path)))
            continue
        
        # 3. 尝试查找可能的子目录
        if original_path.parts:
            # 尝试在各个子项目目录中查找
            subprojects = [p for p in codebase_path.iterdir() if p.is_dir() and not p.name.startswith('.')]
            found = False
            for subproject in subprojects:
                test_path = subproject.joinpath(*original_path.parts[1:]) if len(original_path.parts) > 1 else subproject.joinpath(original_path)
                if test_path.exists():
                    fixed_locations.append(str(test_path.relative_to(codebase_path)))
                    found = True
                    break
            if found:
                continue
        
        # 4. 在 doc/ 下的子目录中查找
        if doc_path.parent.exists():
            for subdir in doc_path.parent.iterdir():
                if subdir.is_dir():
                    test_path = subdir / original_path.name
                    if test_path.exists():
                        fixed_locations.append(str(test_path.relative_to(codebase_path)))
                        found = True
                        break
            if found:
                continue
        
        # 5. 尝试用 CKG 或目录查找类似文件
        # 简单策略：保留原样，但记录日志
        logger.warning(f"Code location not found, keeping as-is: {code_loc}")
        fixed_locations.append(code_loc)
    
    return fixed_locations


def extract_log_keywords_from_code(codebase: Optional[str], code_locations: List[str]) -> Tuple[List[str], List[str]]:
    """从业务代码中提取 logger 日志关键词和完整的消息模板
    
    Returns:
        (log_keywords, log_message_templates): 关键词列表和消息模板列表
    """
    log_keywords = []
    log_message_templates = []
    if not codebase or not code_locations:
        return log_keywords, log_message_templates

    codebase_path = Path(codebase)
    languages = get_languages()
    supported_extensions = set()
    if languages:
        for lang in languages:
            ext = lang.get("extension", "")
            if ext:
                supported_extensions.add(ext)
    if not supported_extensions:
        supported_extensions = {".java"}
    
    # 常见的 logger 调用模式
    log_patterns = [
        r'logger\.(?:debug|info|warn|warning|error|fatal)\s*\(\s*["\']([^"\']+)',
        r'log\.(?:debug|info|warn|warning|error|fatal)\s*\(\s*["\']([^"\']+)',
        r'LOGGER\.(?:debug|info|warn|warning|error|fatal)\s*\(\s*["\']([^"\']+)',
    ]
    
    # 专门提取【】包裹的日志标题模式
    title_patterns = [
        r'logger\.(?:debug|info|warn|warning|error|fatal)\s*\(\s*["\']【([^】]+)】',
        r'log\.(?:debug|info|warn|warning|error|fatal)\s*\(\s*["\']【([^】]+)】',
        r'LOGGER\.(?:debug|info|warn|warning|error|fatal)\s*\(\s*["\']【([^】]+)】',
    ]
    
    for code_loc in code_locations:
        try:
            code_file = codebase_path / code_loc
            if code_file.exists() and code_file.suffix in supported_extensions:
                content = code_file.read_text(encoding='utf-8')
                
                # 先提取标题模式
                for pattern in title_patterns:
                    matches = re.findall(pattern, content)
                    for match in matches:
                        if match and match not in log_message_templates:
                            log_message_templates.append(match)
                
                # 提取日志消息中的关键词
                for pattern in log_patterns:
                    matches = re.findall(pattern, content)
                    for match in matches:
                        # 提取日志消息中的英文单词和有意义的中文短语
                        # 英文单词
                        english_words = re.findall(r'[a-zA-Z_][a-zA-Z0-9_]*', match)
                        log_keywords.extend(english_words)
                        
                        # 中文关键短语（两个字以上的）
                        chinese_phrases = re.findall(r'[\u4e00-\u9fa5]{2,}', match)
                        log_keywords.extend(chinese_phrases)
                
                logger.info(f"Extracted log keywords from: {code_loc}")
                
        except Exception as e:
            logger.warning(f"Failed to extract log keywords from {code_loc}: {e}")
    
    # 去重
    return list(set(log_keywords)), list(set(log_message_templates))


def load_business_docs(codebase: Optional[str], guide_path: Optional[str]) -> Dict[str, str]:
    """加载相关业务文档"""
    docs = {}
    if not codebase or not guide_path:
        return docs

    codebase_path = Path(codebase)

    # 1. 先尝试加载 guide_path 指向的文档
    guide_full_path = codebase_path / guide_path
    if not guide_full_path.exists():
        guide_full_path_doc = codebase_path / "doc" / guide_path
        if guide_full_path_doc.exists():
            guide_full_path = guide_full_path_doc
    if guide_full_path.exists():
        try:
            docs[guide_path] = guide_full_path.read_text(encoding='utf-8')
            logger.info(f"Loaded guide: {guide_path}")
        except Exception as e:
            logger.warning(f"Failed to load guide {guide_path}: {e}")
    
    # 2. 如果 guide_path 指向子项目（codebase 第一级目录），加载子项目的 project-index 和 business-index
    # 判断 guide_path 是否在 codebase 第一级子目录中
    guide_relative = guide_full_path.relative_to(codebase_path) if codebase_path in guide_full_path.parents or guide_full_path.parent == codebase_path else None
    is_in_subproject = guide_relative and len(guide_relative.parts) >= 1 and guide_relative.parts[0] != "doc"
    
    if is_in_subproject or guide_path.endswith("/project-index.md"):
        # 提取子项目目录
        subproject_dir = guide_full_path.parent
        if subproject_dir.exists():
            # 加载子项目的 project-index
            subproject_index = subproject_dir / "project-index.md"
            if subproject_index.exists() and str(subproject_index) != str(guide_full_path):
                try:
                    rel_path = subproject_index.relative_to(codebase_path)
                    docs[str(rel_path)] = subproject_index.read_text(encoding='utf-8')
                    logger.info(f"Loaded subproject index: {rel_path}")
                except Exception as e:
                    logger.warning(f"Failed to load subproject index: {e}")
            
            # 加载 business 目录下的文档
            business_dir = subproject_dir / "business"
            if business_dir.exists():
                # 优先加载 business-index.md
                business_index = business_dir / "business-index.md"
                if business_index.exists():
                    try:
                        rel_path = business_index.relative_to(codebase_path)
                        docs[str(rel_path)] = business_index.read_text(encoding='utf-8')
                        logger.info(f"Loaded business index: {rel_path}")
                    except Exception as e:
                        logger.warning(f"Failed to load business index: {e}")
                
                doc_count = 0
                max_docs = 5
                all_doc_files = sorted(
                    [f for f in business_dir.glob("*.md") if f.name != "business-index.md"],
                    key=lambda f: f.name
                )
                for doc_file in all_doc_files:
                    if doc_count >= max_docs:
                        break
                    try:
                        rel_path = doc_file.relative_to(codebase_path)
                        docs[str(rel_path)] = doc_file.read_text(encoding='utf-8')
                        doc_count += 1
                        logger.info(f"Loaded business doc: {rel_path}")
                    except Exception as e:
                        logger.warning(f"Failed to load business doc {doc_file}: {e}")
    
    return docs


def load_business_entry_code_files(codebase: Optional[str], business_docs: Dict[str, str], business_domain: str = "", fault_description: str = "", project_config: dict[str, Any] | None = None) -> Dict[str, str]:
    """加载业务入口的代码文件，特别是ConsumerHandler、Service入口等会打印日志的地方
    
    Args:
        codebase: 代码库路径
        business_docs: 已加载的业务文档
        business_domain: 业务域名称，如 "order"，用于优先匹配对应子项目
        fault_description: 故障描述，用于优先匹配相关文档
        project_config: 从 project-index.md 解析的项目配置，如果为 None 则从配置文件读取
    
    Returns:
        Dict[str, str]: 代码文件路径到内容的映射
    """
    code_files = {}
    if not codebase:
        return code_files
    
    codebase_path = Path(codebase)
    codebase_str = str(codebase_path).replace('\\', '/')
    
    if project_config:
        lang_config = project_config
        entry_keywords = lang_config.get("entry_keywords", [])
        high_priority_keywords = lang_config.get("high_priority_keywords", [])
        entry_patterns = lang_config.get("entry_patterns", [])
        lang_extension = lang_config.get("extension", ".java")
    else:
        languages = get_languages()
        lang_config = languages[0] if languages else {}
        entry_keywords = lang_config.get("entry_keywords", ['Consumer', 'Handler', 'Service', 'Controller', 'Process'])
        high_priority_keywords = lang_config.get("high_priority_keywords", [])
        entry_patterns = lang_config.get("entry_patterns", [])
        lang_extension = lang_config.get("extension", ".java")
    
    # 策略1: 从业务文档中提取 file:/// 格式的Java文件链接
    # 优先处理包含高优先级关键词的文档，以及与故障描述相关的文档
    fault_keywords = set()
    if fault_description:
        fault_keywords = set(re.findall(r'[\u4e00-\u9fa5]{2,}|[a-zA-Z_]{3,}', fault_description))
    
    def doc_priority(item):
        doc_path_key, doc_content = item
        has_high = any(kw in doc_content for kw in high_priority_keywords)
        fault_match = len(fault_keywords & set(re.findall(r'[\u4e00-\u9fa5]{2,}|[a-zA-Z_]{3,}', doc_content)))
        return (0 if has_high else 1, -fault_match, doc_path_key)
    
    sorted_docs = sorted(business_docs.items(), key=doc_priority)
    
    for doc_path_key, doc_content in sorted_docs:
        escaped_ext = re.escape(lang_extension)
        file_link_pattern = rf'\[([^\]]*{escaped_ext})\]\(file:///([^)]+{escaped_ext})(?:#[^)]*)?\)'
        file_links = re.findall(file_link_pattern, doc_content)
        prioritized_links = sorted(
            file_links,
            key=lambda x: (0 if any(kw in x[0] for kw in high_priority_keywords) else 1, x[0])
        )
        for link_text, link_path in prioritized_links:
            if len(code_files) >= 10:
                break
            if not any(kw in link_text for kw in entry_keywords):
                continue
            normalized_path = link_path.replace('\\', '/')
            try:
                abs_path = Path(normalized_path)
                if abs_path.exists() and abs_path.suffix == lang_extension:
                    rel_path = str(abs_path.relative_to(codebase_path)).replace('\\', '/')
                    if rel_path not in code_files:
                        content = abs_path.read_text(encoding='utf-8')
                        code_files[rel_path] = content
                        logger.info(f"Loaded entry code from doc link: {rel_path}")
            except Exception as e:
                logger.warning(f"Failed to load entry code {link_path}: {e}")
        
        rel_link_pattern = rf'\[([^\]]*{escaped_ext})\]\((?!file:///)([^)]+{escaped_ext})(?:#[^)]*)?\)'
        rel_links = re.findall(rel_link_pattern, doc_content)
        for link_text, link_path in rel_links:
            if len(code_files) >= 10:
                break
            if not any(kw in link_text for kw in entry_keywords):
                continue
            normalized_path = link_path.replace('\\', '/')
            try:
                abs_path = codebase_path / normalized_path
                if abs_path.exists() and abs_path.suffix == lang_extension:
                    rel_path = str(abs_path.relative_to(codebase_path)).replace('\\', '/')
                    if rel_path not in code_files:
                        content = abs_path.read_text(encoding='utf-8')
                        code_files[rel_path] = content
                        logger.info(f"Loaded entry code from doc rel link: {rel_path}")
            except Exception as e:
                logger.warning(f"Failed to load entry code {link_path}: {e}")
    
    # 策略2: 从业务文档中提取类名，然后搜索对应文件
    # 匹配任何类名，而不只是包含 entry_keywords 的类名
    all_class_names = set()
    for doc_path_key, doc_content in sorted_docs:
        # 匹配 class 或 interface 定义
        class_names = re.findall(r'(?:class|interface)\s+(\w+)', doc_content)
        all_class_names.update(class_names)
    
    # 优先处理高优先级关键词匹配的类
    if high_priority_keywords:
        prioritized_classes = sorted(
            all_class_names,
            key=lambda x: (0 if any(kw in x for kw in high_priority_keywords) else 1, x)
        )
    else:
        prioritized_classes = sorted(all_class_names)
    
    for class_name in prioritized_classes[:50]:  # 限制搜索数量
        if len(code_files) >= 10:
            break
        class_filename = f"{class_name}{lang_extension}"
        for code_file in codebase_path.rglob(class_filename):
            if len(code_files) >= 10:
                break
            try:
                rel_path = str(code_file.relative_to(codebase_path)).replace('\\', '/')
                if rel_path not in code_files:
                    content = code_file.read_text(encoding='utf-8')
                    code_files[rel_path] = content
                    logger.info(f"Loaded entry code by class name: {rel_path}")
            except Exception as e:
                logger.warning(f"Failed to load code file {code_file}: {e}")
    
    # 策略3: 使用glob模式搜索关键入口文件（优先匹配业务域对应的子项目）
    common_entry_patterns = entry_patterns if entry_patterns else [
        "**/*ConsumerHandler*.java",
        "**/*Process*.java",
        "**/*Service*.java",
    ]
    
    # codebase 下所有第一级目录都是子项目
    subdirs = sorted(
        [d for d in codebase_path.iterdir() if d.is_dir()],
        key=lambda d: (0 if business_domain and business_domain in d.name else 1, d.name)
    )
    
    for subdir in subdirs:
        for pattern in common_entry_patterns:
            if len(code_files) >= 10:
                break
            for code_file in subdir.rglob(pattern):
                if len(code_files) >= 10:
                    break
                try:
                    rel_path = str(code_file.relative_to(codebase_path)).replace('\\', '/')
                    if rel_path not in code_files:
                        content = code_file.read_text(encoding='utf-8')
                        code_files[rel_path] = content
                        logger.info(f"Loaded entry code by glob: {rel_path}")
                except Exception as e:
                    logger.warning(f"Failed to load code file {code_file}: {e}")
        if len(code_files) >= 10:
            break
    
    return code_files


def _extract_container_names_from_yaml(yaml_content: str) -> List[str]:
    """从 deployment YAML 的 containers 段提取容器名称
    
    YAML 结构:
      containers:
        - name: xxx-server
          image: ...
    需要区分 containers 下的 - name: 和 env 下的 - name:
    """
    container_names = []
    in_containers = False
    for line in yaml_content.splitlines():
        stripped = line.strip()
        if stripped.startswith("containers:"):
            in_containers = True
            continue
        if in_containers:
            if stripped.startswith("- name:") and not stripped.startswith("- name: \"") and not stripped.startswith("- name: '"):
                match = re.match(r'-\s*name:\s*(\S+)', stripped)
                if match:
                    name = match.group(1).strip('"').strip("'")
                    if name and not name.startswith("productMode") and not name.startswith("disconf") and \
                       not name.startswith("datasource") and not name.startswith("RUNNING") and \
                       not name.startswith("SW_") and not name.startswith("SPRING") and \
                       not name.startswith("ENV") and not name.startswith("JAVA") and \
                       not name.startswith("SC_") and not name.startswith("disconfApp"):
                        if name not in container_names:
                            container_names.append(name)
                    in_containers = False
            elif not stripped.startswith("-") and not stripped.startswith("#") and stripped and not stripped.startswith("name:"):
                if not stripped.startswith("image:") and not stripped.startswith("ports:") and \
                   not stripped.startswith("env:") and not stripped.startswith("volumeMounts:") and \
                   not stripped.startswith("resources:") and not stripped.startswith("livenessProbe:") and \
                   not stripped.startswith("readinessProbe:") and not stripped.startswith("command:") and \
                   not stripped.startswith("args:") and not stripped.startswith("securityContext:") and \
                   not stripped.startswith("terminationMessagePath:"):
                    in_containers = False
    return container_names


def _extract_container_names_from_docs(doc_content: str, container_patterns: list[str] | None = None) -> List[str]:
    container_names = []
    if container_patterns:
        naming_patterns = container_patterns
    else:
        naming_patterns = get_container_naming().get("patterns", ["-server", "-gateway", "-web"])
    patterns = []
    for suffix in naming_patterns:
        escaped = re.escape(suffix)
        patterns.append(rf'\|\s*([a-z][a-z0-9-]*{escaped}\d*)\s*\|')
    patterns.extend([
        r'containers:\s*\n\s*-\s*name:\s*(\S+)',
    ])
    markdown_table_patterns = [
        r'\|\s*[a-z][a-z0-9-]+\s*\|\s*[a-z_]+\s*\|[^\n]*\|\s*`?([a-z][a-z0-9-]+)`?\s*\|',
        r'\|\s*[a-z][a-z0-9-]+\s*\|\s*[a-z_]+\s*\|\s*`[^`]+`\s*\|\s*`?([a-z][a-z0-9-]+)`?\s*\|',
    ]
    for table_pattern in markdown_table_patterns:
        table_matches = re.findall(table_pattern, doc_content)
        for m in table_matches:
            m = m.strip()
            if m and re.match(r'^[a-z][a-z0-9-]*$', m) and len(m) > 3 and m not in container_names:
                container_names.append(m)
    for pattern in patterns:
        matches = re.findall(pattern, doc_content)
        for m in matches:
            m = m.strip('"').strip("'").strip('|').strip()
            if m and re.match(r'^[a-z][a-z0-9-]*$', m) and len(m) > 3:
                if m not in container_names:
                    container_names.append(m)
    return container_names


class ProjectConfigError(Exception):
    """项目配置缺失异常"""
    pass


def parse_project_config(doc_content: str) -> dict[str, Any]:
    """从 project-index.md 解析项目配置
    
    必须配置项：
    - entry_keywords: 入口类关键词
    
    可选配置项：
    - extension: 代码文件后缀（默认 .java）
    - container_patterns: 容器命名模式
    - high_priority_keywords: 高优先级关键词
    - entry_patterns: 入口文件搜索模式
    
    Returns:
        {
            "extension": ".java",
            "entry_keywords": [...],
            "container_patterns": [...],
            "high_priority_keywords": [...],
            "entry_patterns": [...],
        }
    
    Raises:
        ProjectConfigError: 当必须配置项缺失时
    """
    config = {
        "extension": ".java",
        "entry_keywords": [],
        "container_patterns": [],
        "high_priority_keywords": [],
        "entry_patterns": [],
    }
    
    missing_required = []
    
    if 'extension:' in doc_content.lower():
        ext_match = re.search(r'extension:\s*(\.[a-zA-Z0-9]+)', doc_content, re.IGNORECASE)
        if ext_match:
            config["extension"] = ext_match.group(1)
    
    entry_kw_match = re.search(r'entry_keywords:\s*\n((?:\s*-\s*.+\n)+)', doc_content)
    if entry_kw_match:
        keywords = re.findall(r'-\s*(.+)', entry_kw_match.group(1))
        config["entry_keywords"] = [kw.strip() for kw in keywords]
    
    container_match = re.search(r'container_patterns:\s*\n((?:\s*-\s*.+\n)+)', doc_content)
    if container_match:
        patterns = re.findall(r'-\s*(.+)', container_match.group(1))
        config["container_patterns"] = [p.strip() for p in patterns]
    
    high_prio_match = re.search(r'high_priority_keywords:\s*\n((?:\s*-\s*.+\n)+)', doc_content)
    if high_prio_match:
        keywords = re.findall(r'-\s*(.+)', high_prio_match.group(1))
        config["high_priority_keywords"] = [kw.strip() for kw in keywords]
    
    entry_patterns_match = re.search(r'entry_patterns:\s*\n((?:\s*-\s*.+\n)+)', doc_content)
    if entry_patterns_match:
        patterns = re.findall(r'-\s*(.+)', entry_patterns_match.group(1))
        config["entry_patterns"] = [p.strip() for p in patterns]
    
    if not config["entry_keywords"]:
        missing_required.append("entry_keywords")
    
    if missing_required:
        raise ProjectConfigError(
            f"缺少必须的项目配置项：{', '.join(missing_required)}。"
            f"请在 project-index.md 中添加这些配置项后重试。"
        )
    
    return config


def _domain_matches_container(domain: str, container_name: str) -> bool:
    """判断业务域是否与容器名匹配（通用匹配）
    
    匹配逻辑:
    - 从 domain 中提取英文缩写/单词
    - 检查是否包含在容器名中
    """
    domain_lower = domain.lower()
    container_lower = container_name.lower()
    
    english_parts = re.findall(r'[a-zA-Z]+', domain)
    for part in english_parts:
        if len(part) >= 2 and part.lower() in container_lower:
            return True
    
    if domain_lower in container_lower:
        return True
    return False


def infer_container_names(codebase: Optional[str], domain: str, business_docs: Dict[str, str], project_config: dict[str, Any] | None = None) -> List[str]:
    """根据业务域从 deployment YAML 和文档中提取 K8s 容器名称
    
    Args:
        codebase: 代码库路径
        domain: 业务域名称
        business_docs: 业务文档
        project_config: 从 project-index.md 解析的项目配置，如果为 None 则从配置文件读取
    
    优先级:
    1.从 project-index.md 等文档中提取容器名称
    2.搜索 deployment YAML 文件中的 containers.name 字段
    如果以上策略都无法找到容器名，返回空列表
    """
    container_names = []
    if not domain or domain == "unknown":
        return container_names

    english_parts = re.findall(r'[a-zA-Z]{2,}', domain)
    domain_key = english_parts[0].lower() if english_parts else None

    # 策略1: 从 deployment YAML 文件中提取真实容器名
    if codebase:
        codebase_path = Path(codebase)
        all_yaml_containers = []
        deployment_pattern = "deployment/**/deploy*.yaml"
        for deploy_yaml in codebase_path.rglob(deployment_pattern):
            try:
                yaml_content = deploy_yaml.read_text(encoding="utf-8")
                yaml_containers = _extract_container_names_from_yaml(yaml_content)
                for c in yaml_containers:
                    if c not in all_yaml_containers:
                        all_yaml_containers.append(c)
            except Exception as e:
                logger.warning(f"Failed to parse deploy yaml {deploy_yaml}: {e}")
        
        if all_yaml_containers:
            for c in all_yaml_containers:
                if _domain_matches_container(domain, c):
                    if c not in container_names:
                        container_names.append(c)
            
            if not container_names and domain_key:
                for c in all_yaml_containers:
                    if domain_key in c.lower():
                        if c not in container_names:
                            container_names.append(c)
                        if len(container_names) >= 3:
                            break

    # 策略2: 从 project-index.md 等文档中提取容器名
    if not container_names:
        container_patterns = None
        if project_config:
            container_patterns = project_config.get("container_patterns", [])
        
        for doc_content in business_docs.values():
            doc_containers = _extract_container_names_from_docs(doc_content, container_patterns)
            for c in doc_containers:
                if _domain_matches_container(domain, c):
                    if c not in container_names:
                        container_names.append(c)
            
            if not container_names and doc_containers and domain_key:
                for c in doc_containers:
                    if domain_key in c.lower():
                        if c not in container_names:
                            container_names.append(c)
                        if len(container_names) >= 3:
                            break
                break

    return container_names
def extract_json_from_llm_response(content: str) -> Optional[str]:
    """从 LLM 响应中提取 JSON"""
    content = content.strip()

    # 移除可能的 markdown 代码块标记
    if content.startswith("```json"):
        content = content[7:]
    elif content.startswith("```"):
        content = content[3:]

    if content.endswith("```"):
        content = content[:-3]

    return content.strip()


class BusinessAnalysisTool(Tool):
    """业务分析工具：识别业务域并加载相关业务文档"""

    def __init__(
        self,
        model_provider: str | None = None,
        config_file: str = "trae_config.yaml"
    ):
        super().__init__(model_provider)
        self._name = "business_analysis"
        self._description = "根据故障描述识别业务域，加载相关业务文档，并提取日志关键词、入口点等信息"
        self._config_file = config_file
        self._llm_client: LLMClient | None = None
        self._doc_path: str | None = None

    @override
    def get_model_provider(self) -> str | None:
        return self._model_provider

    @override
    def get_name(self) -> str:
        return self._name

    @override
    def get_description(self) -> str:
        return self._description

    def _get_doc_path(self) -> str:
        """获取文档路径配置"""
        if self._doc_path is None:
            try:
                config = Config.create(config_file=self._config_file)
                if config.rca_agent and hasattr(config.rca_agent, 'doc_path'):
                    self._doc_path = config.rca_agent.doc_path
                else:
                    self._doc_path = "doc"
            except Exception as e:
                logger.warning(f"Failed to load doc_path config: {e}")
                self._doc_path = "doc"
        return self._doc_path

    def set_doc_path(self, doc_path: str) -> None:
        """设置文档路径配置（由 agent 调用）"""
        self._doc_path = doc_path

    @override
    def get_parameters(self) -> list[ToolParameter]:
        return [
            ToolParameter(
                name="description",
                type="string",
                description="故障描述，包含业务场景、错误现象等信息",
                required=True
            ),
            ToolParameter(
                name="codebase",
                type="string",
                description="代码库路径，用于查找 project-guide.md",
                required=False
            )
        ]

    @override
    async def execute(self, arguments: ToolCallArguments) -> ToolExecResult:
        description = str(arguments.get("description")) if "description" in arguments else None
        codebase = str(arguments.get("codebase")) if "codebase" in arguments else None

        if not description:
            return ToolExecResult(
                error="No description provided for business analysis",
                error_code=-1
            )

        try:
            result = await self._analyze_business(description, codebase)
            return ToolExecResult(output=json.dumps(result.model_dump(), ensure_ascii=False, indent=2))
        except Exception as e:
            return ToolExecResult(
                error=f"Error analyzing business: {e}",
                error_code=-1
            )

    async def _analyze_business(self, description: str, codebase: Optional[str]) -> FaultBusinessResult:
        """分析业务并返回结果 - 三阶段分析"""

        # 获取 doc_path 配置
        doc_path = self._get_doc_path()
        system_prompt = get_system_prompt(doc_path)

        # 1. 加载 project-index.md
        project_index, index_path = load_project_index(codebase, doc_path)

        if not project_index or not index_path:
            # 没有 project-index，返回 unknown
            return FaultBusinessResult(
                business="unknown",
                domain="unknown",
                guide="",
                confidence=0.0,
                reason="无法加载 project-index.md，请确保 codebase 路径正确",
                matched_keywords=[]
            )

        # 2. 第一阶段：初步识别业务域和 guide
        first_pass_prompt = build_user_prompt(description, project_index)
        first_pass_result = await self._call_llm(system_prompt, first_pass_prompt)
        
        if not first_pass_result:
            return FaultBusinessResult(
                business="unknown",
                domain="unknown",
                guide="",
                confidence=0.0,
                reason="LLM 调用失败",
                matched_keywords=[]
            )
        
        first_pass_data = self._parse_llm_result(first_pass_result)
        
        # 使用找到的 index_path 作为 guide，如果 LLM 没有提供更好的
        if not first_pass_data.guide:
            first_pass_data.guide = index_path
        
        # 3. 第二阶段：加载相关业务文档并进行深度分析
        business_docs = load_business_docs(codebase, first_pass_data.guide)
        
        # 3.1 验证 project-index.md 必须配置项
        project_config = None
        if first_pass_data.guide:
            guide_path = Path(codebase) / first_pass_data.guide if codebase else None
            if guide_path and guide_path.exists():
                guide_content = guide_path.read_text(encoding='utf-8')
                try:
                    project_config = parse_project_config(guide_content)
                except ProjectConfigError as e:
                    return FaultBusinessResult(
                        business="unknown",
                        domain="unknown",
                        guide=first_pass_data.guide,
                        confidence=0.0,
                        reason=str(e),
                        matched_keywords=[],
                        container_names=[],
                        log_keywords=[]
                    )
        
        # 3.5 阶段：加载业务入口的代码文件（提取日志模板的关键步骤）
        entry_code_files = {}
        if business_docs:
            entry_code_files = load_business_entry_code_files(codebase, business_docs, business_domain=first_pass_data.business, fault_description=description, project_config=project_config)
            logger.info(f"Loaded {len(entry_code_files)} entry code files for log extraction")
        
        final_result = first_pass_data
        if business_docs or entry_code_files:
            # 合并文档和代码内容
            combined_content = dict(business_docs)
            # 将代码文件内容添加到combined_content中供LLM分析
            for code_path, code_content in entry_code_files.items():
                if project_config:
                    _lang_ext = project_config.get("extension", ".java")
                else:
                    _languages = get_languages()
                    _lang_ext = _languages[0].get("extension", ".java") if _languages else ".java"
                lang_tag = _lang_ext.lstrip('.')
                combined_content[f"[代码文件]{code_path}"] = f"```{lang_tag}\n{code_content[:5000]}\n```"
            
            if combined_content:
                second_pass_prompt = build_user_prompt(description, project_index, combined_content)
                second_pass_result = await self._call_llm(system_prompt, second_pass_prompt)
                
                if second_pass_result:
                    second_pass_data = self._parse_llm_result(second_pass_result)
                    # 用第二阶段结果更新，但保留第一阶段找到的 guide（这是实际存在的文件路径！）
                    final_result = second_pass_data
                    final_result.guide = first_pass_data.guide  # 强制保留第一阶段找到的正确 guide
                    if final_result.confidence == 0:
                        final_result.confidence = first_pass_data.confidence
        
        # 4. 验证并修正 code_locations 路径
        if final_result.code_locations and codebase:
            final_result.code_locations = validate_and_fix_code_locations(codebase, final_result.code_locations)
            logger.info(f"Validated and fixed code_locations: {final_result.code_locations}")
        
        # 5. 从代码中提取日志关键词和模板
        all_code_locations = list(final_result.code_locations or [])
        
        # 添加入口代码文件路径
        for code_path in entry_code_files.keys():
            if code_path not in all_code_locations:
                all_code_locations.append(code_path)
        
        if all_code_locations and codebase:
            extracted_keywords, extracted_templates = extract_log_keywords_from_code(codebase, all_code_locations)
            if extracted_keywords:
                existing_keywords = set(final_result.log_keywords)
                existing_keywords.update(extracted_keywords)
                final_result.log_keywords = list(existing_keywords)
                logger.info(f"Extracted {len(extracted_keywords)} log keywords from code")
            if extracted_templates:
                final_result.log_message_templates = extracted_templates
                logger.info(f"Extracted {len(extracted_templates)} log message templates from code")

        # 清理 log_keywords：将 field:value 格式转换为普通字符串（避免 TLS 键值搜索）
        import re
        cleaned_keywords = []
        for kw in final_result.log_keywords:
            if ':' in kw:
                parts = kw.split(':', 1)
                if len(parts) == 2 and re.match(r'^[a-zA-Z_][a-zA-Z0-9_]*$', parts[0]) and re.match(r'^[a-zA-Z0-9]+$', parts[1]):
                    cleaned_keywords.append(parts[0])
                    cleaned_keywords.append(parts[1])
                else:
                    cleaned_keywords.append(kw)
            else:
                cleaned_keywords.append(kw)
        final_result.log_keywords = list(set(cleaned_keywords))
        
        # 6. 推导容器名称
        final_result.container_names = infer_container_names(codebase, final_result.domain, business_docs, project_config=project_config)
        logger.info(f"Inferred container names: {final_result.container_names}")
        
        # 7. 自动生成 search_logs 调用提醒
        # TLS Topic ID 从环境变量读取（配置在 trae_config.yaml）
        import os
        production_topic = os.environ.get("VOLCENGINE_TOPIC_ID_PRODUCTION", "43a4fcb6-a6e7-4102-bf62-1c7dc51adf61")
        final_result.topic_id = production_topic

        if final_result.container_names and final_result.log_keywords:
            container_str = ",".join(final_result.container_names)
            keywords_str = " ".join(final_result.log_keywords[:5])
            final_result.search_logs_reminder = f"""【重要提醒】必须立即调用 search_logs！

在继续任何代码分析之前，你必须先搜索 TLS 日志：

```python
search_logs(
    container_name="{container_str}",
    query="{keywords_str}",
    topic_id="{production_topic}"
)
```

⚠️ 重要说明：
- topic_id 已由 business_analysis 返回，直接使用
- 只有当搜索返回 0 结果时才考虑切换测试环境 topic_id
- 不要调用 list_topics 来找 topic_id - 这是浪费步骤！

禁止行为：
- ❌ 不要在调用 search_logs 之前阅读代码文件
- ❌ 不要使用 log_analysis.analyze_keyword 来搜索日志
- ❌ 不要调用 list_topics 来找 topic_id
- ❌ 不要连续调用多次 read_file

正确流程：
1. business_analysis ← 已完成
2. search_logs ← 必须立即执行！
3. 如果找到错误日志 → generate_report → task_done
"""
        elif final_result.container_names:
            container_str = ",".join(final_result.container_names)
            final_result.search_logs_reminder = f"""【重要提醒】必须立即调用 search_logs！

```python
search_logs(
    container_name="{container_str}",
    query="<从故障描述中提取的关键词>",
    topic_id="{production_topic}"
)
```

⚠️ 重要说明：
- topic_id 已由 business_analysis 返回，直接使用
- 不要调用 list_topics 来找 topic_id！

container_names 已找到，请立即搜索日志！"""

        return final_result

    def _get_llm_client(self) -> LLMClient | None:
        """获取 LLM 客户端"""
        if self._llm_client is None:
            try:
                config = Config.create(config_file=self._config_file)
                if config.trae_agent:
                    self._llm_client = LLMClient(config.trae_agent.model)
            except Exception as e:
                logger.warning(f"Failed to create LLM client: {e}")
        return self._llm_client

    async def _call_llm(self, system_prompt: str, user_prompt: str) -> Optional[str]:
        """调用 LLM"""
        llm_client = self._get_llm_client()
        if not llm_client:
            logger.warning("No LLM client available")
            return None

        try:
            messages = [
                LLMMessage(role="system", content=system_prompt),
                LLMMessage(role="user", content=user_prompt)
            ]

            # 加载配置
            config = Config.create(config_file=self._config_file)
            model_config = config.trae_agent.model if config.trae_agent else None

            if not model_config:
                logger.warning("No model config available")
                return None

            response = llm_client.chat(messages, model_config)
            return response.content if response else None

        except Exception as e:
            logger.warning(f"LLM call failed: {e}")
            return None

    def _parse_llm_result(self, llm_content: str) -> FaultBusinessResult:
        """解析 LLM 返回的结果"""
        try:
            json_str = extract_json_from_llm_response(llm_content)
            data = json.loads(json_str)
            return FaultBusinessResult(**data)
        except Exception as e:
            logger.warning(f"Failed to parse LLM result: {e}")
            # 返回默认值
            return FaultBusinessResult(
                business="unknown",
                domain="unknown",
                guide="",
                confidence=0.0,
                reason=f"结果解析失败: {str(e)}",
                matched_keywords=[]
            )
