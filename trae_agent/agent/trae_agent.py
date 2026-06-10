# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""TraeAgent - 通用 Agent，通过 system_prompt + tools 配置驱动不同行为。

类似 Claude Code 的设计理念：一个 Agent 类，配置决定角色。
- Coding 场景：默认 prompt + bash/edit/task_done 等工具
- RCA 场景：RCA prompt + business_analysis/log_analysis/task_done 等工具
"""

import asyncio
import contextlib
import logging
import os
import subprocess
from pathlib import Path
from typing import override

from trae_agent.agent.agent_basics import AgentError, AgentExecution
from trae_agent.agent.base_agent import BaseAgent
from trae_agent.prompt.agent_prompt import TRAE_AGENT_SYSTEM_PROMPT
from trae_agent.tools import tools_registry
from trae_agent.tools.base import Tool, ToolResult
from trae_agent.utils.config import MCPServerConfig, TraeAgentConfig
from trae_agent.utils.llm_clients.llm_basics import LLMMessage, LLMResponse
from trae_agent.utils.mcp_client import MCPClient

logger = logging.getLogger(__name__)

# 默认工具集（coding 场景）
DEFAULT_TOOL_NAMES = [
    "bash",
    "str_replace_based_edit_tool",
    "sequentialthinking",
    "task_done",
]


class TraeAgent(BaseAgent):
    """通用 Agent，通过配置驱动行为，无需子类化。

    配置中的 system_prompt 和 tools 决定 Agent 的角色和能力：
    - system_prompt 为 None → 使用默认 coding prompt
    - system_prompt 为文件路径 → 加载文件内容作为 prompt
    - system_prompt 为内联文本 → 直接使用
    """

    def __init__(
        self,
        trae_agent_config: TraeAgentConfig,
        docker_config: dict | None = None,
        docker_keep: bool = True,
    ):
        self.project_path: str = ""
        self.base_commit: str | None = None
        self.must_patch: str = "false"
        self.patch_path: str | None = None
        self.mcp_servers_config: dict[str, MCPServerConfig] | None = (
            trae_agent_config.mcp_servers_config if trae_agent_config.mcp_servers_config else None
        )
        self.allow_mcp_servers: list[str] | None = (
            trae_agent_config.allow_mcp_servers if trae_agent_config.allow_mcp_servers else []
        )
        self.mcp_tools: list[Tool] = []
        self.mcp_clients: list[MCPClient] = []
        self.docker_config = docker_config
        super().__init__(
            agent_config=trae_agent_config, docker_config=docker_config, docker_keep=docker_keep
        )

    async def initialise_mcp(self):
        """异步初始化 MCP 工具"""
        await self._discover_mcp_tools()
        if self.mcp_tools:
            self._tools.extend(self.mcp_tools)

    async def _discover_mcp_tools(self):
        """发现并注册 MCP 工具"""
        if not self.mcp_servers_config:
            return
        for mcp_server_name, mcp_server_config in self.mcp_servers_config.items():
            if self.allow_mcp_servers is None:
                return
            if mcp_server_name not in self.allow_mcp_servers:
                continue
            mcp_client = MCPClient()
            try:
                await mcp_client.connect_and_discover(
                    mcp_server_name,
                    mcp_server_config,
                    self.mcp_tools,
                    self._llm_client.provider.value,
                )
                self.mcp_clients.append(mcp_client)
            except Exception:
                with contextlib.suppress(Exception):
                    await mcp_client.cleanup(mcp_server_name)
            except asyncio.CancelledError:
                with contextlib.suppress(Exception):
                    await mcp_client.cleanup(mcp_server_name)

    @override
    def new_task(
        self,
        task: str,
        extra_args: dict[str, str] | None = None,
        tool_names: list[str] | None = None,
    ):
        """创建新任务，通过 extra_args 中的键决定消息格式。

        支持两种场景：
        - Coding：extra_args 包含 project_path / issue / must_patch 等
        - RCA：extra_args 包含 codebase_path / topic_id / output_file 等
        """
        self._task = task

        # 初始化工具：优先使用传入的 tool_names，其次使用配置中的 tools
        if tool_names is None and len(self._tools) == 0:
            tool_names = self._agent_config.tools or DEFAULT_TOOL_NAMES
            provider = self._model_config.model_provider.provider
            self._tools = [
                tools_registry[name](model_provider=provider) for name in tool_names
            ]

        # 构建系统消息
        self._initial_messages = [
            LLMMessage(role="system", content=self.get_system_prompt())
        ]

        # 构建用户消息
        user_message = self._build_user_message(task, extra_args)
        self._initial_messages.append(LLMMessage(role="user", content=user_message))

        # 启动轨迹记录
        if self._trajectory_recorder:
            self._trajectory_recorder.start_recording(
                task=task,
                provider=self._llm_client.provider.value,
                model=self._model_config.model,
                max_steps=self._max_steps,
            )

    def _build_user_message(self, task: str, extra_args: dict[str, str] | None) -> str:
        """根据 extra_args 构建用户消息，支持 coding 和 RCA 两种场景。"""
        if not extra_args:
            return task

        # 处理 doc_path（RCA 场景需要更新到工具实例）
        if "doc_path" in extra_args:
            if hasattr(self._agent_config, 'doc_path'):
                self._agent_config.doc_path = extra_args["doc_path"]
            for tool in self._tools:
                if hasattr(tool, 'set_doc_path'):
                    tool.set_doc_path(extra_args["doc_path"])

        # Coding 场景：project_path + issue
        if "project_path" in extra_args:
            return self._build_coding_message(task, extra_args)

        # RCA 场景：codebase_path + fault description
        if "codebase_path" in extra_args:
            return self._build_rca_message(task, extra_args)

        # 通用场景：直接拼接 extra_args
        return self._build_generic_message(task, extra_args)

    def _build_coding_message(self, task: str, extra_args: dict[str, str]) -> str:
        """构建 coding 场景的用户消息"""
        self.project_path = extra_args.get("project_path", "")
        message = ""
        if self.docker_config:
            message += r"[Project root path]:\workspace\n\n"
        else:
            message += f"[Project root path]:\n{self.project_path}\n\n"

        if "issue" in extra_args:
            message += (
                f"[Problem statement]: We're currently solving the following issue "
                f"within our repository. Here's the issue text:\n{extra_args['issue']}\n"
            )

        # 设置 coding 相关属性
        for attr in ["base_commit", "must_patch", "patch_path"]:
            if attr in extra_args:
                setattr(self, attr, extra_args[attr])

        return message

    def _build_rca_message(self, task: str, extra_args: dict[str, str]) -> str:
        """构建 RCA 场景的用户消息"""
        self.project_path = extra_args["codebase_path"]
        message = f"[Codebase path]:\n{self.project_path}\n\n"
        message += f"[Fault description]:\n{task}\n"

        if "topic_id" in extra_args:
            message += f"[Topic ID for log query]:\n{extra_args['topic_id']}\n\n"
        if "output_file" in extra_args:
            message += f"[Output file path]:\n{extra_args['output_file']}\n\n"

        return message

    def _build_generic_message(self, task: str, extra_args: dict[str, str]) -> str:
        """构建通用场景的用户消息，将 extra_args 格式化拼接"""
        parts = []
        for key, value in extra_args.items():
            parts.append(f"[{key}]:\n{value}")
        parts.append(f"[Task]:\n{task}")
        return "\n\n".join(parts)

    @override
    async def execute_task(self) -> AgentExecution:
        """执行任务并完成轨迹记录"""
        execution = await super().execute_task()

        # 完成轨迹记录
        if self._trajectory_recorder:
            self._trajectory_recorder.finalize_recording(
                success=execution.success, final_result=execution.final_result
            )

        # Coding 场景：写入 patch 文件
        if self.patch_path is not None:
            with open(self.patch_path, "w") as patch_f:
                _ = patch_f.write(self.get_git_diff())

        return execution

    def get_system_prompt(self) -> str:
        """获取系统提示词，优先使用配置中的自定义 prompt"""
        custom_prompt = self._agent_config.system_prompt
        if custom_prompt is None:
            return TRAE_AGENT_SYSTEM_PROMPT

        # 如果是文件路径，加载文件内容
        prompt_path = Path(custom_prompt)
        if prompt_path.exists():
            return prompt_path.read_text(encoding="utf-8")

        # 否则作为内联文本直接使用
        return custom_prompt

    @override
    def reflect_on_result(self, tool_results: list[ToolResult]) -> str | None:
        return None

    def get_git_diff(self) -> str:
        """获取项目的 git diff"""
        pwd = os.getcwd()
        if not os.path.isdir(self.project_path):
            return ""
        os.chdir(self.project_path)
        try:
            if not self.base_commit:
                stdout = subprocess.check_output(["git", "--no-pager", "diff"]).decode()
            else:
                stdout = subprocess.check_output(
                    ["git", "--no-pager", "diff", self.base_commit, "HEAD"]
                ).decode()
        except (subprocess.CalledProcessError, FileNotFoundError):
            stdout = ""
        finally:
            os.chdir(pwd)
        return stdout

    # Copyright (c) 2024 paul-gauthier
    # SPDX-License-Identifier: Apache-2.0
    # Original remove_patches_to_tests function was released under Apache-2.0 License, with the full license text
    # available at https://github.com/Aider-AI/aider-swe-bench/blob/6e98cd6c3b2cbcba12976d6ae1b07f847480cb74/LICENSE.txt
    # Original function is at https://github.com/Aider-AI/aider-swe-bench/blob/6e98cd6c3b2cbcba12976d6ae1b07f847480cb74/tests.py#L45

    def remove_patches_to_tests(self, model_patch: str) -> str:
        """从 patch 中移除对 tests 目录的修改"""
        lines = model_patch.splitlines(keepends=True)
        filtered_lines: list[str] = []
        test_patterns = ["/test/", "/tests/", "/testing/", "test_", "tox.ini"]
        is_tests = False

        for line in lines:
            if line.startswith("diff --git a/"):
                target_path = line.split()[-1]
                is_tests = target_path.startswith("b/") and any(
                    p in target_path for p in test_patterns
                )

            if not is_tests:
                filtered_lines.append(line)

        return "".join(filtered_lines)

    @override
    def llm_indicates_task_completed(self, llm_response: LLMResponse) -> bool:
        """通过 task_done 工具调用判断任务是否完成"""
        if llm_response.tool_calls is None:
            return False
        return any(tool_call.name == "task_done" for tool_call in llm_response.tool_calls)

    @override
    def _is_task_completed(self, llm_response: LLMResponse) -> bool:
        """增强的任务完成检测：must_patch 模式下检查 patch 是否非空"""
        if self.must_patch == "true":
            model_patch = self.get_git_diff()
            patch = self.remove_patches_to_tests(model_patch)
            if not patch.strip():
                return False
        return True

    @override
    def task_incomplete_message(self) -> str:
        """任务未完成时的提示消息"""
        return "ERROR! Your Patch is empty. Please provide a patch that fixes the problem."

    @override
    async def cleanup_mcp_clients(self) -> None:
        """清理 MCP 客户端，防止异步上下文泄漏"""
        for client in self.mcp_clients:
            try:
                await client.cleanup("cleanup")
            except (Exception, asyncio.CancelledError) as e:
                logger.warning(f"Error cleaning up MCP client: {e}")
        self.mcp_clients.clear()

    async def analyze_incident(
        self,
        fault_description: str,
        codebase_path: str,
        topic_id: str | None = None,
        output_file: str | None = None,
    ) -> str:
        """便捷方法：执行 RCA 分析（从原 RCAAgent 迁移）"""
        extra_args = {"codebase_path": codebase_path}
        if topic_id:
            extra_args["topic_id"] = topic_id
        if output_file:
            extra_args["output_file"] = output_file

        self.new_task(fault_description, extra_args)
        execution = await self.execute_task()
        return execution.final_result or ""
