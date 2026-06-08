# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""RCAAgent - using LLM reasoning + tool calls for root cause analysis.
"""

import asyncio
import contextlib
from typing import override

from trae_agent.agent.agent_basics import AgentExecution
from trae_agent.agent.base_agent import BaseAgent
from trae_agent.prompt.rca_agent_prompt import RCA_AGENT_SYSTEM_PROMPT
from trae_agent.tools import tools_registry
from trae_agent.tools.base import Tool
from trae_agent.utils.config import MCPServerConfig, RCAAgentConfig
from trae_agent.utils.llm_clients.llm_basics import LLMMessage, LLMResponse
from trae_agent.utils.mcp_client import MCPClient


class RCAAgent(BaseAgent):
    """RCAAgent - using LLM reasoning + tool calls for root cause analysis.
    """

    RCAAgentToolNames = [
        "business_analysis",
        "log_analysis",
        "task_done",
    ]

    def __init__(
        self,
        rca_agent_config: RCAAgentConfig,
        docker_config: dict | None = None,
        docker_keep: bool = True,
    ):
        """Initialize RCAAgent.

        Args:
            rca_agent_config: Configuration for the RCA agent.
            docker_config: Optional configuration for running in a Docker environment.
            docker_keep: Whether to keep the Docker container after finishing.
        """
        self.project_path: str = ""
        self.mcp_servers_config: dict[str, MCPServerConfig] | None = (
            rca_agent_config.mcp_servers_config if rca_agent_config.mcp_servers_config else None
        )
        self.allow_mcp_servers: list[str] | None = (
            rca_agent_config.allow_mcp_servers if rca_agent_config.allow_mcp_servers else []
        )
        self.mcp_tools: list[Tool] = []
        self.mcp_clients: list[MCPClient] = []
        super().__init__(
            agent_config=rca_agent_config,
            docker_config=docker_config,
            docker_keep=docker_keep,
        )

    async def initialise_mcp(self):
        """Async factory to create and initialize RCAAgent with MCP tools."""
        await self.discover_mcp_tools()

        if self.mcp_tools:
            self._tools.extend(self.mcp_tools)

    async def discover_mcp_tools(self):
        """Discover MCP tools."""
        if self.mcp_servers_config:
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
                        self._model_config.model_provider.provider,
                    )
                    self.mcp_clients.append(mcp_client)
                except Exception:
                    with contextlib.suppress(Exception):
                        await mcp_client.cleanup(mcp_server_name)
                    continue
                except asyncio.CancelledError:
                    with contextlib.suppress(Exception):
                        await mcp_client.cleanup(mcp_server_name)
                    continue
        else:
            return

    @override
    def new_task(
        self,
        task: str,
        extra_args: dict[str, str] | None = None,
        tool_names: list[str] | None = None,
    ):
        """Create a new RCA task.

        Args:
            task: The fault description to analyze.
            extra_args: Additional arguments.
            tool_names: Optional list of tool names to use.
        """
        self._task = task

        if tool_names is None and len(self._tools) == 0:
            tool_names = self.RCAAgentToolNames

            provider = self._model_config.model_provider.provider
            self._tools: list[Tool] = [
                tools_registry[tool_name](model_provider=provider) for tool_name in tool_names
            ]

        self._initial_messages: list[LLMMessage] = []
        self._initial_messages.append(LLMMessage(role="system", content=self.get_system_prompt()))

        user_message = ""
        if extra_args and "codebase_path" in extra_args:
            self.project_path = extra_args["codebase_path"]
            user_message += f"[Codebase path]:\n{self.project_path}\n\n"

        # Update doc_path from extra_args if provided
        if extra_args and "doc_path" in extra_args:
            if hasattr(self._agent_config, 'doc_path'):
                self._agent_config.doc_path = extra_args["doc_path"]
            # Update doc_path in BusinessAnalysisTool instance
            for tool in self._tools:
                if hasattr(tool, 'set_doc_path'):
                    tool.set_doc_path(extra_args["doc_path"])

        user_message += f"[Fault description]:\n{task}\n"

        if extra_args:
            if "topic_id" in extra_args:
                user_message += f"[Topic ID for log query]:\n{extra_args['topic_id']}\n\n"
            if "output_file" in extra_args:
                user_message += f"[Output file path]:\n{extra_args['output_file']}\n\n"

        self._initial_messages.append(LLMMessage(role="user", content=user_message))

        if self._trajectory_recorder:
            self._trajectory_recorder.start_recording(
                task=task,
                provider=self._model_config.model_provider.provider,
                model=self._model_config.model,
                max_steps=self._max_steps,
            )

    @override
    async def execute_task(self) -> AgentExecution:
        """Execute the task and finalize trajectory recording."""
        execution = await super().execute_task()

        # Finalize trajectory recording if recorder is available
        if self._trajectory_recorder:
            self._trajectory_recorder.finalize_recording(
                success=execution.success, final_result=execution.final_result
            )

        return execution

    def get_system_prompt(self) -> str:
        """Get the system prompt for RCAAgent."""
        return RCA_AGENT_SYSTEM_PROMPT

    @override
    def llm_indicates_task_completed(self, llm_response: LLMResponse) -> bool:
        """Check if the LLM indicates that the task is completed via task_done tool call."""
        if llm_response.tool_calls is None:
            return False
        return any(tool_call.name == "task_done" for tool_call in llm_response.tool_calls)

    @override
    async def cleanup_mcp_clients(self) -> None:
        """Clean up MCP clients."""
        for client in self.mcp_clients:
            with contextlib.suppress(Exception, asyncio.CancelledError):
                await client.cleanup("cleanup")
        self.mcp_clients.clear()

    async def analyze_incident(
        self,
        fault_description: str,
        codebase_path: str,
        topic_id: str | None = None,
        output_file: str | None = None,
    ) -> str:
        """
        Convenience method: Perform RCA analysis.

        This method encapsulates the new_task + execute_task flow,
        for use by CLI or other external callers.

        Args:
            fault_description: Description of the fault/incident.
            codebase_path: Path to the codebase.
            topic_id: Optional Volc TLS topic ID for log query.
            output_file: Optional path to save the report.

        Returns:
            The RCA report content.
        """
        extra_args = {"codebase_path": codebase_path}
        if topic_id:
            extra_args["topic_id"] = topic_id
        if output_file:
            extra_args["output_file"] = output_file

        self.new_task(fault_description, extra_args)

        # MCP initialization will be handled by the Agent factory if needed
        execution = await self.execute_task()
        return execution.final_result or ""
