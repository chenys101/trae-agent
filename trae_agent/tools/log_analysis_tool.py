# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

import json
from pathlib import Path
from typing import Any, Dict, List, override

from trae_agent.tools.base import Tool, ToolCallArguments, ToolExecResult, ToolParameter
from trae_agent.tools.run import MAX_RESPONSE_LEN


class LogAnalysisTool(Tool):
    """Tool for analyzing logs and generating RCA reports."""

    def __init__(self, model_provider: str | None = None) -> None:
        super().__init__(model_provider)

    @override
    def get_model_provider(self) -> str | None:
        return self._model_provider

    @override
    def get_name(self) -> str:
        return "log_analysis"

    @override
    def get_description(self) -> str:
        return """Analyze logs and generate RCA reports.
* State is persistent across command calls and discussions with the user
* The `analyze_keyword` command extracts search keywords from fault descriptions (text processing only, does NOT search code)
* The `generate_report` command generates a structured RCA report from Agent-provided log data
* The `read_file` command reads code files for context
* If a `command` generates a long output, it will be truncated and marked with `<response clipped>`
"""

    @override
    def get_parameters(self) -> list[ToolParameter]:
        return [
            ToolParameter(
                name="command",
                type="string",
                description="The command to run. Allowed options are: analyze_keyword, generate_report, read_file.",
                required=True,
                enum=["analyze_keyword", "generate_report", "read_file"],
            ),
            ToolParameter(
                name="path",
                type="string",
                description="The base path to the codebase (for read_file).",
                required=True,
            ),
            ToolParameter(
                name="keyword",
                type="string",
                description="The keyword to analyze (for analyze_keyword command).",
                required=False,
            ),
            ToolParameter(
                name="log_keywords",
                type="string",
                description="JSON array string of pre-extracted log keywords from business_analysis (for analyze_keyword command).",
                required=False,
            ),
            ToolParameter(
                name="log_data",
                type="string",
                description="JSON string of log data for RCA report generation (for generate_report command). Must include root_cause_summary, timeline, evidence_logs.",
                required=False,
            ),
            ToolParameter(
                name="output_file",
                type="string",
                description="Optional output file path for saving the RCA report.",
                required=False,
            ),
            ToolParameter(
                name="file_path",
                type="string",
                description="File path to read (for read_file command, relative to path parameter or absolute).",
                required=False,
            ),
        ]

    @override
    async def execute(self, arguments: ToolCallArguments) -> ToolExecResult:
        command = str(arguments.get("command")) if "command" in arguments else None
        if command is None:
            return ToolExecResult(
                error=f"No command provided for the {self.get_name()} tool",
                error_code=-1,
            )

        path = str(arguments.get("path")) if "path" in arguments else None
        if path is None:
            return ToolExecResult(
                error=f"No path provided for the {self.get_name()} tool",
                error_code=-1,
            )

        codebase_path = Path(path)
        if not codebase_path.exists():
            return ToolExecResult(
                error=f"Codebase path {path} does not exist",
                error_code=-1,
            )
        if not codebase_path.is_dir():
            return ToolExecResult(
                error=f"Codebase path {path} is not a directory",
                error_code=-1,
            )

        match command:
            case "analyze_keyword":
                keyword = str(arguments.get("keyword")) if "keyword" in arguments else None
                log_keywords_str = str(arguments.get("log_keywords")) if "log_keywords" in arguments else None
                if keyword is None and log_keywords_str is None:
                    return ToolExecResult(
                        error=f"Either keyword or log_keywords must be provided for analyze_keyword command",
                        error_code=-1,
                    )
                return ToolExecResult(
                    output=self._analyze_keyword(keyword, log_keywords_str)
                )
            case "generate_report":
                log_data_str = str(arguments.get("log_data")) if "log_data" in arguments else None
                output_file = str(arguments.get("output_file")) if "output_file" in arguments else None

                if not log_data_str or log_data_str == "None" or log_data_str.strip() == "":
                    return ToolExecResult(
                        error="generate_report requires log_data parameter with root cause information. "
                              "Please provide log_data with root_cause_summary, timeline, and evidence_logs.",
                        error_code=-1,
                    )
                return ToolExecResult(
                    output=self._generate_report(log_data_str, output_file)
                )
            case "read_file":
                file_path = str(arguments.get("file_path")) if "file_path" in arguments else None
                if file_path is None:
                    return ToolExecResult(
                        error=f"file_path must be provided for read_file command",
                        error_code=-1,
                    )
                return ToolExecResult(
                    output=self._read_file(codebase_path, file_path)
                )
            case _:
                return ToolExecResult(error=f"Invalid command: {command}", error_code=-1)

    def _analyze_keyword(self, keyword: str | None, log_keywords_str: str | None) -> str:
        """Extract search keywords from fault description for log searching.

        This method performs text-based keyword extraction only:
        - English words and camelCase splitting
        - Chinese-to-English keyword mapping
        - Number extraction (order IDs, etc.)
        - Default error keywords

        It does NOT search code or query any database.
        """
        results = {
            "keyword": keyword,
            "modules": [],
            "submodules": [],
            "interfaces": [],
            "log_keywords": []
        }

        if log_keywords_str:
            try:
                pre_extracted_keywords = json.loads(log_keywords_str)
                if isinstance(pre_extracted_keywords, list):
                    results["log_keywords"].extend(pre_extracted_keywords)
            except Exception:
                pass

        if keyword:
            search_keywords = self._extract_keywords(keyword)
            results["log_keywords"].extend(search_keywords)

            if not results["log_keywords"]:
                return json.dumps({
                    "keyword": keyword,
                    "modules": [],
                    "submodules": [],
                    "interfaces": [],
                    "log_keywords": [],
                    "warning": "没有找到日志关键词，请补充更多信息（如订单号、错误信息等）"
                }, ensure_ascii=False, indent=2)

        results["log_keywords"] = list(set(results["log_keywords"]))

        output = json.dumps(results, ensure_ascii=False, indent=2)

        if len(output) > MAX_RESPONSE_LEN:
            output = output[:MAX_RESPONSE_LEN] + "\n<response clipped>"

        return output

    def _extract_keywords(self, text: str) -> list[str]:
        """从文本中提取搜索关键词"""
        keywords = []
        import re

        english_words = re.findall(r'[a-zA-Z_]+', text)
        keywords.extend(english_words)

        for word in english_words:
            subwords = self._split_camel_case(word)
            keywords.extend(subwords)

        numbers = re.findall(r'\d{6,}', text)
        keywords.extend(numbers)

        error_keywords = ['error', 'Error', 'ERROR', 'exception', 'Exception', 'EXCEPTION',
                         'warn', 'Warn', 'WARN', 'warning', 'Warning', 'WARNING',
                         'fail', 'Fail', 'FAIL', 'timeout', 'Timeout', 'TIMEOUT']
        keywords.extend(error_keywords)

        return list(set(keywords))

    def _split_camel_case(self, word: str) -> list[str]:
        """拆分驼峰命名为子词"""
        import re

        pattern = re.compile(
            r'([a-z0-9])([A-Z])|([A-Z]+)([A-Z][a-z])|([a-zA-Z])([0-9])'
        )

        parts = []
        current = word

        while current:
            match = pattern.search(current)
            if match:
                split_pos = match.start(1) + 1
                parts.append(current[:split_pos])
                current = current[split_pos:]
            else:
                parts.append(current)
                break

        result = []
        for part in parts:
            if len(part) >= 2:
                result.append(part)
                result.append(part.lower())
                result.append(part.upper())

        return list(set(result))

    def _generate_report(self, log_data_str: str | None, output_file: str | None) -> str:
        """Generate RCA report from Agent-provided structured log data."""
        report = {
            "report_type": "RCA",
            "sections": []
        }

        if log_data_str:
            try:
                log_data = json.loads(log_data_str)
            except json.JSONDecodeError:
                log_data = {"error": "Invalid log data format"}
        else:
            log_data = {}

        has_rca_data = any(key in log_data for key in ["root_cause_summary", "timeline", "evidence_logs", "root_cause_type"])

        if has_rca_data:
            report_sections = self._generate_report_from_agent_data(log_data)
            report["sections"] = report_sections
        else:
            report["sections"].append({
                "section": "提示",
                "content": "generate_report 需要结构化的日志数据（root_cause_summary, timeline, evidence_logs）。"
                           "请先使用 search_logs 搜索日志，然后将分析结果以结构化格式传入。"
            })

        markdown_report = self._convert_to_markdown(report)

        if output_file:
            try:
                with open(output_file, "w", encoding="utf-8") as f:
                    f.write(markdown_report)
                markdown_report += f"\n\n报告已保存到: {output_file}"
            except Exception as e:
                markdown_report += f"\n\n保存报告失败: {str(e)}"

        return markdown_report

    def _generate_report_from_agent_data(self, log_data: Dict[str, Any]) -> List[Dict[str, str]]:
        """Generate report sections from Agent-provided structured data."""
        sections = []

        if "root_cause_summary" in log_data:
            sections.append({
                "section": "根因摘要",
                "content": log_data["root_cause_summary"]
            })

        if "timeline" in log_data:
            timeline_content = []
            for item in log_data["timeline"]:
                if isinstance(item, dict):
                    time = item.get("time", "")
                    event = item.get("event", "")
                    trace = item.get("traceId", "")
                    timeline_content.append(f"- **{time}**{f' [{trace}]' if trace else ''}: {event}")
                else:
                    timeline_content.append(f"- {item}")
            sections.append({
                "section": "时间线",
                "content": "\n".join(timeline_content)
            })

        if "evidence_logs" in log_data:
            evidence_content = []
            for log in log_data["evidence_logs"]:
                evidence_content.append(f"- {log}")
            sections.append({
                "section": "证据日志",
                "content": "\n".join(evidence_content)
            })

        if "code_locations" in log_data:
            code_content = []
            for loc in log_data["code_locations"]:
                code_content.append(f"- {loc}")
            sections.append({
                "section": "代码位置",
                "content": "\n".join(code_content)
            })

        if "impact_scope" in log_data:
            sections.append({
                "section": "影响范围",
                "content": log_data["impact_scope"]
            })

        if "root_cause_type" in log_data:
            sections.append({
                "section": "根因类型",
                "content": log_data["root_cause_type"]
            })

        if "recommendations" in log_data:
            rec_content = []
            for rec in log_data["recommendations"]:
                rec_content.append(f"- {rec}")
            sections.append({
                "section": "修复建议",
                "content": "\n".join(rec_content)
            })

        if "confidence" in log_data:
            sections.append({
                "section": "置信度",
                "content": log_data["confidence"]
            })

        return sections

    def _read_file(self, base_path: Path, file_path: str) -> str:
        """Read file content directly.

        Args:
            base_path: Base codebase path
            file_path: File path to read (relative or absolute)

        Returns:
            File content as string, or error message
        """
        full_path = Path(file_path)
        if not full_path.is_absolute():
            full_path = base_path / file_path

        if not full_path.exists():
            doc_path = base_path / "doc" / file_path
            if doc_path.exists():
                full_path = doc_path
            else:
                doc_root = base_path / "doc"
                if doc_root.exists():
                    for subdir in doc_root.rglob("*"):
                        if subdir.is_dir():
                            test_path = subdir / Path(file_path).name
                            if test_path.exists():
                                full_path = test_path
                                break

        if not full_path.exists():
            return f"Error: File not found at {base_path / file_path} (tried doc/ directory too)"

        if not full_path.is_file():
            return f"Error: {full_path} is not a file"

        try:
            content = full_path.read_text(encoding='utf-8')

            if len(content) > MAX_RESPONSE_LEN:
                content = content[:MAX_RESPONSE_LEN] + "\n\n<file content truncated>"

            return content
        except Exception as e:
            return f"Error reading file {full_path}: {str(e)}"

    def _convert_to_markdown(self, report: Dict[str, Any]) -> str:
        """Convert report to Markdown format."""
        lines = []
        lines.append("# RCA 故障分析报告")
        lines.append("")

        for section in report["sections"]:
            lines.append(f"## {section['section']}")
            lines.append("")
            for line in section["content"].split("\n"):
                lines.append(line)
            lines.append("")

        lines.append("---")
        lines.append("*报告生成时间：自动生成*")

        return "\n".join(lines)
