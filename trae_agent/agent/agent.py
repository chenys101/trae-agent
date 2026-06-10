# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""Agent 工厂模块 - 统一创建 TraeAgent，通过配置驱动不同行为。

类似 Claude Code 的设计：一个 Agent 类，配置决定角色。
不再需要 AgentType 枚举和子类，所有场景统一使用 TraeAgent。
"""

import asyncio
import contextlib

from trae_agent.agent.trae_agent import TraeAgent
from trae_agent.utils.cli.cli_console import CLIConsole
from trae_agent.utils.config import AgentConfig, Config
from trae_agent.utils.trajectory_recorder import TrajectoryRecorder


class Agent:
    """Agent 工厂，根据配置创建 TraeAgent 实例。

    agent_type 参数保留向后兼容，但内部统一创建 TraeAgent。
    不同场景（coding / RCA / 自定义）通过配置区分：
    - tools: 决定可用工具
    - system_prompt: 决定 Agent 角色
    - 其他字段: 场景特定配置
    """

    def __init__(
        self,
        agent_type: str | None = None,
        config: Config | None = None,
        trajectory_file: str | None = None,
        cli_console: CLIConsole | None = None,
        docker_config: dict | None = None,
        docker_keep: bool = True,
    ):
        # 根据 agent_type 选择对应的 AgentConfig
        agent_config = self._resolve_agent_config(agent_type, config)

        # 设置轨迹记录
        if trajectory_file is not None:
            self.trajectory_file = trajectory_file
            self.trajectory_recorder = TrajectoryRecorder(trajectory_file)
        else:
            self.trajectory_recorder = TrajectoryRecorder()
            self.trajectory_file = self.trajectory_recorder.get_trajectory_path()

        # 统一创建 TraeAgent
        self.agent_config: AgentConfig = agent_config
        self.agent = TraeAgent(
            agent_config, docker_config=docker_config, docker_keep=docker_keep
        )
        self.agent.set_cli_console(cli_console)

        # Lakeview 配置
        if cli_console:
            if hasattr(agent_config, "enable_lakeview") and agent_config.enable_lakeview and config and config.lakeview:
                cli_console.set_lakeview(config.lakeview)
            else:
                cli_console.set_lakeview(None)

        self.agent.set_trajectory_recorder(self.trajectory_recorder)

    def _resolve_agent_config(self, agent_type: str | None, config: Config | None) -> AgentConfig:
        """根据 agent_type 选择对应的 AgentConfig"""
        if config is None:
            raise ValueError("config is required")

        # rca_agent 优先使用 config.rca_agent，回退到 config.trae_agent
        if agent_type == "rca_agent":
            if config.rca_agent is not None:
                return config.rca_agent
            if config.trae_agent is not None:
                return config.trae_agent
            raise ValueError("rca_agent_config or trae_agent_config is required for rca_agent")

        # 默认使用 trae_agent
        if config.trae_agent is None:
            raise ValueError("trae_agent config is required")
        return config.trae_agent

    async def run(
        self,
        task: str,
        extra_args: dict[str, str] | None = None,
        tool_names: list[str] | None = None,
    ):
        """运行 Agent 任务"""
        self.agent.new_task(task, extra_args, tool_names)

        # 初始化 MCP 工具
        if self.agent.allow_mcp_servers:
            if self.agent.cli_console:
                self.agent.cli_console.print("Initialising MCP tools...")
            await self.agent.initialise_mcp()

        # 打印任务详情
        if self.agent.cli_console:
            task_details = {
                "Task": task,
                "Model Provider": self.agent_config.model.model_provider.provider,
                "Model": self.agent_config.model.model,
                "Max Steps": str(self.agent_config.max_steps),
                "Trajectory File": self.trajectory_file,
                "Tools": ", ".join([tool.name for tool in self.agent.tools]),
            }
            if extra_args:
                for key, value in extra_args.items():
                    task_details[key.capitalize()] = value
            self.agent.cli_console.print_task_details(task_details)

        cli_console_task = (
            asyncio.create_task(self.agent.cli_console.start()) if self.agent.cli_console else None
        )

        try:
            execution = await self.agent.execute_task()
        finally:
            with contextlib.suppress(Exception):
                await self.agent.cleanup_mcp_clients()

        if cli_console_task:
            try:
                await cli_console_task
            except asyncio.CancelledError:
                pass

        return execution
