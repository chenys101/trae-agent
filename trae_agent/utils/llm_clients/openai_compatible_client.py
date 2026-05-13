# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""OpenAI-compatible provider client for services like DeepSeek."""

import openai

from trae_agent.utils.config import ModelConfig
from trae_agent.utils.llm_clients.openai_compatible_base import (
    OpenAICompatibleClient,
    ProviderConfig,
)


class OpenAICompatibleProvider(ProviderConfig):
    """OpenAI-compatible provider configuration."""

    def create_client(
        self, api_key: str, base_url: str | None, api_version: str | None
    ) -> openai.OpenAI:
        """Create OpenAI client with custom base URL."""
        return openai.OpenAI(api_key=api_key, base_url=base_url)

    def get_service_name(self) -> str:
        """Get the service name for retry logging."""
        return "OpenAI-Compatible"

    def get_provider_name(self) -> str:
        """Get the provider name for trajectory recording."""
        return "openai_compatible"

    def get_extra_headers(self) -> dict[str, str]:
        """Get any extra headers needed for the API call."""
        return {}

    def supports_tool_calling(self, model_name: str) -> bool:
        """Check if the model supports tool calling."""
        return True


class OpenAICompatibleAPIClient(OpenAICompatibleClient):
    """OpenAI-compatible client wrapper that uses standard /v1/chat/completions API."""

    def __init__(self, model_config: ModelConfig):
        super().__init__(model_config, OpenAICompatibleProvider())