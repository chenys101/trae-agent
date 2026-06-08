# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

import os
from dataclasses import dataclass, field

import yaml

from trae_agent.utils.legacy_config import LegacyConfig


class ConfigError(Exception):
    pass


@dataclass
class ModelSpec:
    """
    Model specification within a provider.
    Inherits default values from provider level, can override them.
    """
    max_tokens: int | None = None
    temperature: float | None = None
    top_p: float | None = None
    top_k: int | None = None
    max_retries: int | None = None
    parallel_tool_calls: bool | None = None
    supports_tool_calling: bool = True
    candidate_count: int | None = None
    stop_sequences: list[str] | None = None
    max_completion_tokens: int | None = None


@dataclass
class ModelProvider:
    """
    Model provider configuration. For official model providers such as OpenAI and Anthropic,
    the base_url is optional. api_version is required for Azure.
    """

    api_key: str
    provider: str
    base_url: str | None = None
    api_version: str | None = None
    # 新增字段
    default_model: str | None = None
    max_tokens: int | None = None
    temperature: float | None = None
    top_p: float | None = None
    top_k: int | None = None
    max_retries: int | None = None
    parallel_tool_calls: bool | None = None
    models: dict[str, ModelSpec] | None = None


@dataclass
class ModelConfig:
    """
    Model configuration.
    """

    model: str
    model_provider: ModelProvider
    temperature: float
    top_p: float
    top_k: int
    parallel_tool_calls: bool
    max_retries: int
    max_tokens: int | None = None  # Legacy max_tokens parameter, optional
    supports_tool_calling: bool = True
    candidate_count: int | None = None  # Gemini specific field
    stop_sequences: list[str] | None = None
    max_completion_tokens: int | None = None  # Azure OpenAI specific field

    @classmethod
    def from_provider_and_model(
        cls,
        provider: ModelProvider,
        model_name: str,
    ) -> "ModelConfig":
        """
        Create ModelConfig from provider and model name.
        Model-specific config overrides provider-level defaults.
        Raises ConfigError if required parameters are not configured.
        """
        # Get model-specific config if exists
        model_spec = None
        if provider.models and model_name in provider.models:
            model_spec = provider.models[model_name]

        # Collect all required parameters and check for missing ones
        required_params = {
            "max_tokens": (model_spec.max_tokens if model_spec else None, provider.max_tokens),
            "temperature": (model_spec.temperature if model_spec else None, provider.temperature),
            "top_p": (model_spec.top_p if model_spec else None, provider.top_p),
            "top_k": (model_spec.top_k if model_spec else None, provider.top_k),
            "max_retries": (model_spec.max_retries if model_spec else None, provider.max_retries),
            "parallel_tool_calls": (model_spec.parallel_tool_calls if model_spec else None, provider.parallel_tool_calls),
        }

        # Check for missing parameters
        missing_params = []
        for param_name, (model_value, provider_value) in required_params.items():
            if model_value is None and provider_value is None:
                missing_params.append(param_name)

        # If there are missing parameters, raise detailed error
        if missing_params:
            missing_list = ", ".join(missing_params)
            error_msg = (
                f"\n{'='*80}\n"
                f"❌ CONFIGURATION ERROR\n"
                f"{'='*80}\n\n"
                f"Model: {model_name}\n"
                f"Provider: {provider.provider}\n\n"
                f"⚠️  Missing required parameters: {missing_list}\n\n"
                f"{'='*80}\n"
                f"CONFIGURATION GUIDE\n"
                f"{'='*80}\n\n"
                f"Option 1 - Configure at provider level (recommended):\n"
                f"  model_providers:\n"
                f"    {provider.provider}:\n"
            )
            for param in missing_params:
                error_msg += f"      {param}: <value>\n"
            
            error_msg += (
                f"\nOption 2 - Configure at model level:\n"
                f"  model_providers:\n"
                f"    {provider.provider}:\n"
                f"      models:\n"
                f"        {model_name}:\n"
            )
            for param in missing_params:
                error_msg += f"          {param}: <value>\n"
            
            error_msg += f"\n{'='*80}\n"
            raise ConfigError(error_msg)

        # Build ModelConfig with validated parameters
        return cls(
            model=model_name,
            model_provider=provider,
            max_tokens=required_params["max_tokens"][0] if required_params["max_tokens"][0] is not None else required_params["max_tokens"][1],
            temperature=required_params["temperature"][0] if required_params["temperature"][0] is not None else required_params["temperature"][1],
            top_p=required_params["top_p"][0] if required_params["top_p"][0] is not None else required_params["top_p"][1],
            top_k=required_params["top_k"][0] if required_params["top_k"][0] is not None else required_params["top_k"][1],
            max_retries=required_params["max_retries"][0] if required_params["max_retries"][0] is not None else required_params["max_retries"][1],
            parallel_tool_calls=required_params["parallel_tool_calls"][0] if required_params["parallel_tool_calls"][0] is not None else required_params["parallel_tool_calls"][1],
            supports_tool_calling=model_spec.supports_tool_calling if model_spec else True,
            candidate_count=model_spec.candidate_count if model_spec else None,
            stop_sequences=model_spec.stop_sequences if model_spec else None,
            max_completion_tokens=model_spec.max_completion_tokens if model_spec else None,
        )

    def get_max_tokens_param(self) -> int:
        """Get the maximum tokens parameter value.Prioritizes max_completion_tokens, falls back to max_tokens if not available."""
        if self.max_completion_tokens is not None:
            return self.max_completion_tokens
        elif self.max_tokens is not None:
            return self.max_tokens
        else:
            # Return default value if neither is set
            return 4096

    def should_use_max_completion_tokens(self) -> bool:
        """Determine whether to use the max_completion_tokens parameter.Primarily used for Azure OpenAI's newer models (e.g., gpt-5)."""
        return (
            self.max_completion_tokens is not None
            and self.model_provider.provider == "azure"
            and ("gpt-5" in self.model or "o3" in self.model or "o4-mini" in self.model)
        )

    def resolve_config_values(
        self,
        *,
        model_providers: dict[str, ModelProvider] | None = None,
        provider: str | None = None,
        model: str | None = None,
        model_base_url: str | None = None,
        api_key: str | None = None,
    ):
        """
        When some config values are provided through CLI or environment variables,
        they will override the values in the config file.
        """
        self.model = str(resolve_config_value(cli_value=model, config_value=self.model))

        # If the user wants to change the model provider, they should either:
        # * Make sure the provider name is available in the model_providers dict;
        # * If not, base url and api key should be provided to register a new model provider.
        if provider:
            if model_providers and provider in model_providers:
                self.model_provider = model_providers[provider]
            elif api_key is None:
                raise ConfigError("To register a new model provider, an api_key should be provided")
            else:
                self.model_provider = ModelProvider(
                    api_key=api_key,
                    provider=provider,
                    base_url=model_base_url,
                )

        # Map providers to their environment variable names
        env_var_api_key = str(self.model_provider.provider).upper() + "_API_KEY"
        env_var_api_base_url = str(self.model_provider.provider).upper() + "_BASE_URL"

        resolved_api_key = resolve_config_value(
            cli_value=api_key,
            config_value=self.model_provider.api_key,
            env_var=env_var_api_key,
        )

        resolved_api_base_url = resolve_config_value(
            cli_value=model_base_url,
            config_value=self.model_provider.base_url,
            env_var=env_var_api_base_url,
        )

        if resolved_api_key:
            self.model_provider.api_key = str(resolved_api_key)

        if resolved_api_base_url:
            self.model_provider.base_url = str(resolved_api_base_url)


@dataclass
class MCPServerConfig:
    # For stdio transport
    command: str | None = None
    args: list[str] | None = None
    env: dict[str, str] | None = None
    cwd: str | None = None

    # For sse transport
    url: str | None = None

    # For streamable http transport
    http_url: str | None = None
    headers: dict[str, str] | None = None

    # For websocket transport
    tcp: str | None = None

    # Common
    timeout: int | None = None
    trust: bool | None = None

    # Metadata
    description: str | None = None


@dataclass
class AgentConfig:
    """
    Base class for agent configurations.
    """

    allow_mcp_servers: list[str]
    mcp_servers_config: dict[str, MCPServerConfig]
    max_steps: int
    tools: list[str]
    # model 字段设为可选，将在创建后设置
    model: ModelConfig | None = None
    # 新增字段
    provider: str | None = None
    model_name: str | None = None


@dataclass
class TraeAgentConfig(AgentConfig):
    """
    Trae agent configuration.
    """

    enable_lakeview: bool = True
    tools: list[str] = field(
        default_factory=lambda: [
            "bash",
            "str_replace_based_edit_tool",
            "sequentialthinking",
            "task_done",
        ]
    )

    def resolve_config_values(
        self,
        *,
        max_steps: int | None = None,
    ):
        resolved_value = resolve_config_value(cli_value=max_steps, config_value=self.max_steps)
        if resolved_value:
            self.max_steps = int(resolved_value)


@dataclass
class RCAAgentConfig(AgentConfig):
    """
    RCA agent configuration.
    """

    tools: list[str] = field(
        default_factory=lambda: [
            "business_analysis",
            "log_analysis",
            "task_done",
        ]
    )

    codebase: str = ""
    doc_path: str = "doc"

    def resolve_config_values(
        self,
        *,
        max_steps: int | None = None,
    ):
        resolved_value = resolve_config_value(cli_value=max_steps, config_value=self.max_steps)
        if resolved_value:
            self.max_steps = int(resolved_value)


@dataclass
class LakeviewConfig:
    """
    Lakeview configuration.
    """

    model: ModelConfig
    provider: str | None = None
    model_name: str | None = None


@dataclass
class Config:
    """
    Configuration class for agents, models and model providers.
    """

    default_provider: str | None = None
    lakeview: LakeviewConfig | None = None
    model_providers: dict[str, ModelProvider] | None = None
    # 移除 models 字段
    # models: dict[str, ModelConfig] | None = None

    trae_agent: TraeAgentConfig | None = None
    rca_agent: RCAAgentConfig | None = None

    @classmethod
    def create(
        cls,
        *,
        config_file: str | None = None,
        config_string: str | None = None,
    ) -> "Config":
        if config_file and config_string:
            raise ConfigError("Only one of config_file or config_string should be provided")

        # Parse YAML config from file or string
        try:
            if config_file is not None:
                if config_file.endswith(".json"):
                    return cls.create_from_legacy_config(config_file=config_file)
                with open(config_file, "r") as f:
                    yaml_config = yaml.safe_load(f)
            elif config_string is not None:
                yaml_config = yaml.safe_load(config_string)
            else:
                raise ConfigError("No config file or config string provided")
        except yaml.YAMLError as e:
            raise ConfigError(f"Error parsing YAML config: {e}") from e

        config = cls()

        # Parse default_provider
        config.default_provider = yaml_config.get("default_provider", None)

        # Parse model providers with models
        model_providers = yaml_config.get("model_providers", None)
        if model_providers is not None and len(model_providers.keys()) > 0:
            config_model_providers: dict[str, ModelProvider] = {}
            for provider_name, provider_config in model_providers.items():
                # Extract models from provider config
                models_dict = provider_config.pop("models", None) if isinstance(provider_config, dict) else None
                model_specs = None
                if models_dict:
                    model_specs = {}
                    for model_name, model_spec_config in models_dict.items():
                        # Skip if model config is None (e.g., all fields commented out)
                        if model_spec_config is not None:
                            model_specs[model_name] = ModelSpec(**model_spec_config)

                config_model_providers[provider_name] = ModelProvider(
                    **provider_config,
                    models=model_specs
                )
            config.model_providers = config_model_providers
        else:
            raise ConfigError("No model providers provided")

        # Helper function to resolve provider and model
        def resolve_model_config(
            provider_name: str | None,
            model_name: str | None,
            context: str = ""
        ) -> ModelConfig:
            # Priority: specified provider/model -> default_provider -> error
            resolved_provider_name = provider_name or config.default_provider
            if not resolved_provider_name:
                raise ConfigError(f"No provider specified and no default_provider configured{context}")

            if resolved_provider_name not in config_model_providers:
                raise ConfigError(f"Provider '{resolved_provider_name}' not found{context}")

            provider = config_model_providers[resolved_provider_name]

            # Resolve model name
            resolved_model_name = model_name or provider.default_model
            if not resolved_model_name:
                raise ConfigError(
                    f"No model specified and no default_model configured for provider '{resolved_provider_name}'{context}"
                )

            return ModelConfig.from_provider_and_model(provider, resolved_model_name)

        # Parse lakeview config
        lakeview = yaml_config.get("lakeview", None)
        if lakeview is not None:
            lakeview_provider = lakeview.get("provider", None)
            lakeview_model = lakeview.get("model", None)
            lakeview_model_config = resolve_model_config(
                lakeview_provider,
                lakeview_model,
                context=" for lakeview"
            )
            config.lakeview = LakeviewConfig(
                model=lakeview_model_config,
                provider=lakeview_provider,
                model_name=lakeview_model
            )
        else:
            config.lakeview = None

        mcp_servers_config = {
            k: MCPServerConfig(**v) for k, v in yaml_config.get("mcp_servers", {}).items()
        }
        allow_mcp_servers = yaml_config.get("allow_mcp_servers", [])

        # Parse agents
        agents = yaml_config.get("agents", None)
        if agents is not None and len(agents.keys()) > 0:
            for agent_name, agent_config in agents.items():
                agent_provider = agent_config.pop("provider", None) if isinstance(agent_config, dict) else None
                agent_model = agent_config.pop("model", None) if isinstance(agent_config, dict) else None
                agent_model_config = resolve_model_config(
                    agent_provider,
                    agent_model,
                    context=f" for agent '{agent_name}'"
                )

                match agent_name:
                    case "trae_agent":
                        trae_agent_config = TraeAgentConfig(
                            **agent_config,
                            mcp_servers_config=mcp_servers_config,
                            allow_mcp_servers=allow_mcp_servers,
                            provider=agent_provider,
                            model_name=agent_model
                        )
                        trae_agent_config.model = agent_model_config
                        if trae_agent_config.enable_lakeview and config.lakeview is None:
                            raise ConfigError("Lakeview is enabled but no lakeview config provided")
                        config.trae_agent = trae_agent_config
                    case "rca_agent":
                        rca_agent_config = RCAAgentConfig(
                            **agent_config,
                            mcp_servers_config=mcp_servers_config,
                            allow_mcp_servers=allow_mcp_servers,
                            provider=agent_provider,
                            model_name=agent_model
                        )
                        rca_agent_config.model = agent_model_config
                        config.rca_agent = rca_agent_config
                    case _:
                        raise ConfigError(f"Unknown agent: {agent_name}")
        else:
            raise ConfigError("No agent configs provided")
        return config

    def resolve_config_values(
        self,
        *,
        provider: str | None = None,
        model: str | None = None,
        model_base_url: str | None = None,
        api_key: str | None = None,
        max_steps: int | None = None,
    ):
        """
        Resolve configuration values with priority: CLI > config file > defaults.
        """
        if self.trae_agent:
            self.trae_agent.resolve_config_values(max_steps=max_steps)

            # Resolve provider and model with priority: CLI > agent config > default_provider
            resolved_provider_name = provider or self.trae_agent.provider or self.default_provider
            if resolved_provider_name and self.model_providers:
                if resolved_provider_name in self.model_providers:
                    provider_obj = self.model_providers[resolved_provider_name]
                    resolved_model_name = model or self.trae_agent.model_name or provider_obj.default_model

                    if resolved_model_name:
                        # Create new ModelConfig with resolved provider and model
                        self.trae_agent.model = ModelConfig.from_provider_and_model(
                            provider_obj,
                            resolved_model_name
                        )
                        # Override API key and base URL if provided via CLI
                        self.trae_agent.model.resolve_config_values(
                            model_providers=self.model_providers,
                            provider=resolved_provider_name,
                            model=resolved_model_name,
                            model_base_url=model_base_url,
                            api_key=api_key,
                        )
                    else:
                        raise ConfigError(
                            f"No model specified and no default_model for provider '{resolved_provider_name}'"
                        )
                else:
                    raise ConfigError(f"Provider '{resolved_provider_name}' not found")

        if self.rca_agent:
            self.rca_agent.resolve_config_values(max_steps=max_steps)

            # Same logic for rca_agent
            resolved_provider_name = provider or self.rca_agent.provider or self.default_provider
            if resolved_provider_name and self.model_providers:
                if resolved_provider_name in self.model_providers:
                    provider_obj = self.model_providers[resolved_provider_name]
                    resolved_model_name = model or self.rca_agent.model_name or provider_obj.default_model

                    if resolved_model_name:
                        self.rca_agent.model = ModelConfig.from_provider_and_model(
                            provider_obj,
                            resolved_model_name
                        )
                        self.rca_agent.model.resolve_config_values(
                            model_providers=self.model_providers,
                            provider=resolved_provider_name,
                            model=resolved_model_name,
                            model_base_url=model_base_url,
                            api_key=api_key,
                        )
                    else:
                        raise ConfigError(
                            f"No model specified and no default_model for provider '{resolved_provider_name}'"
                        )
                else:
                    raise ConfigError(f"Provider '{resolved_provider_name}' not found")

        return self

    @classmethod
    def create_from_legacy_config(
        cls,
        *,
        legacy_config: LegacyConfig | None = None,
        config_file: str | None = None,
    ) -> "Config":
        if legacy_config and config_file:
            raise ConfigError("Only one of legacy_config or config_file should be provided")

        if config_file:
            legacy_config = LegacyConfig(config_file)
        elif not legacy_config:
            raise ConfigError("No legacy_config or config_file provided")

        model_provider = ModelProvider(
            api_key=legacy_config.model_providers[legacy_config.default_provider].api_key,
            base_url=legacy_config.model_providers[legacy_config.default_provider].base_url,
            api_version=legacy_config.model_providers[legacy_config.default_provider].api_version,
            provider=legacy_config.default_provider,
        )

        model_config = ModelConfig(
            model=legacy_config.model_providers[legacy_config.default_provider].model,
            model_provider=model_provider,
            max_tokens=legacy_config.model_providers[legacy_config.default_provider].max_tokens,
            temperature=legacy_config.model_providers[legacy_config.default_provider].temperature,
            top_p=legacy_config.model_providers[legacy_config.default_provider].top_p,
            top_k=legacy_config.model_providers[legacy_config.default_provider].top_k,
            parallel_tool_calls=legacy_config.model_providers[
                legacy_config.default_provider
            ].parallel_tool_calls,
            max_retries=legacy_config.model_providers[legacy_config.default_provider].max_retries,
            candidate_count=legacy_config.model_providers[
                legacy_config.default_provider
            ].candidate_count,
            stop_sequences=legacy_config.model_providers[
                legacy_config.default_provider
            ].stop_sequences,
        )
        mcp_servers_config = {
            k: MCPServerConfig(**vars(v)) for k, v in legacy_config.mcp_servers.items()
        }
        trae_agent_config = TraeAgentConfig(
            max_steps=legacy_config.max_steps,
            enable_lakeview=legacy_config.enable_lakeview,
            model=model_config,
            allow_mcp_servers=legacy_config.allow_mcp_servers,
            mcp_servers_config=mcp_servers_config,
        )

        if trae_agent_config.enable_lakeview:
            lakeview_config = LakeviewConfig(
                model=model_config,
            )
        else:
            lakeview_config = None

        return cls(
            trae_agent=trae_agent_config,
            lakeview=lakeview_config,
            model_providers={
                legacy_config.default_provider: model_provider,
            },
            models={
                "default_model": model_config,
            },
        )


def resolve_config_value(
    *,
    cli_value: int | str | float | None,
    config_value: int | str | float | None,
    env_var: str | None = None,
) -> int | str | float | None:
    """Resolve configuration value with priority: CLI > ENV > Config > Default."""
    if cli_value is not None:
        return cli_value

    if env_var and os.getenv(env_var):
        return os.getenv(env_var)

    if config_value is not None:
        return config_value

    return None
