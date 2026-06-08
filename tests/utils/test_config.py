# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

import unittest
from unittest.mock import patch

from trae_agent.utils.config import Config, ModelConfig, ModelProvider, ModelSpec
from trae_agent.utils.legacy_config import LegacyConfig
from trae_agent.utils.llm_clients.anthropic_client import AnthropicClient
from trae_agent.utils.llm_clients.openai_client import OpenAIClient


class TestNewConfigStructure(unittest.TestCase):
    """测试新的配置结构"""

    def get_new_config(self):
        """获取新配置结构的测试数据"""
        return """
default_provider: anthropic

model_providers:
  anthropic:
    api_key: test-anthropic-key
    provider: anthropic
    max_tokens: 4096
    temperature: 0.5
    top_p: 1
    top_k: 40
    max_retries: 10
    parallel_tool_calls: true
    default_model: claude-4-sonnet
    models:
      claude-4-sonnet:
        max_tokens: 4096
        temperature: 0.5
        top_p: 1
        top_k: 40
        max_retries: 10
        parallel_tool_calls: true
      claude-3.5-sonnet:
        max_tokens: 8192
        temperature: 0.6
        top_p: 1
        top_k: 40
        max_retries: 10
        parallel_tool_calls: true

  openai:
    api_key: test-openai-key
    provider: openai
    max_tokens: 4096
    temperature: 0.5
    top_p: 1
    top_k: 0
    max_retries: 10
    parallel_tool_calls: true
    default_model: gpt-4o
    models:
      gpt-4o:
        max_tokens: 4096
        temperature: 0.5
        top_p: 1
        top_k: 0
        max_retries: 10
        parallel_tool_calls: true

lakeview:
  provider: anthropic
  model: claude-3.5-sonnet

agents:
  trae_agent:
    enable_lakeview: true
    provider: anthropic
    model: claude-4-sonnet
    max_steps: 200
    tools:
      - bash
      - str_replace_based_edit_tool
      - sequentialthinking
      - task_done

  rca_agent:
    provider: openai
    model: gpt-4o
    max_steps: 15
    codebase: G:/workspace_master
    tools:
      - business_analysis
      - log_analysis
      - task_done

allow_mcp_servers: []
mcp_servers: {}
"""

    def test_new_config_structure_parsing(self):
        """测试新配置结构的解析"""
        config = Config.create(config_string=self.get_new_config())

        # 验证 default_provider
        self.assertEqual(config.default_provider, "anthropic")

        # 验证 model_providers
        self.assertIsNotNone(config.model_providers)
        self.assertIn("anthropic", config.model_providers)
        self.assertIn("openai", config.model_providers)

        # 验证 provider 的 default_model
        self.assertEqual(config.model_providers["anthropic"].default_model, "claude-4-sonnet")
        self.assertEqual(config.model_providers["openai"].default_model, "gpt-4o")

        # 验证 provider 的 models
        self.assertIsNotNone(config.model_providers["anthropic"].models)
        self.assertIn("claude-4-sonnet", config.model_providers["anthropic"].models)
        self.assertIn("claude-3.5-sonnet", config.model_providers["anthropic"].models)

    def test_model_config_from_provider_and_model(self):
        """测试从 provider 和 model 创建 ModelConfig"""
        config = Config.create(config_string=self.get_new_config())

        # 验证 trae_agent 的 model config
        self.assertIsNotNone(config.trae_agent)
        self.assertEqual(config.trae_agent.model.model, "claude-4-sonnet")
        self.assertEqual(config.trae_agent.model.model_provider.provider, "anthropic")
        self.assertEqual(config.trae_agent.model.max_tokens, 4096)
        self.assertEqual(config.trae_agent.model.temperature, 0.5)

        # 验证 rca_agent 的 model config
        self.assertIsNotNone(config.rca_agent)
        self.assertEqual(config.rca_agent.model.model, "gpt-4o")
        self.assertEqual(config.rca_agent.model.model_provider.provider, "openai")
        self.assertEqual(config.rca_agent.model.max_tokens, 4096)

    def test_model_specific_config_override(self):
        """测试模型特定配置覆盖 provider 默认配置"""
        config = Config.create(config_string=self.get_new_config())

        # 验证 lakeview 使用 claude-3.5-sonnet，应该有更高的 max_tokens
        self.assertIsNotNone(config.lakeview)
        self.assertEqual(config.lakeview.model.model, "claude-3.5-sonnet")
        self.assertEqual(config.lakeview.model.max_tokens, 8192)  # 模型特定配置
        self.assertEqual(config.lakeview.model.temperature, 0.6)  # 模型特定配置

    def test_default_provider_and_model_fallback(self):
        """测试 default_provider 和 default_model 的回退逻辑"""
        config_string = """
default_provider: anthropic

model_providers:
  anthropic:
    api_key: test-key
    provider: anthropic
    max_tokens: 4096
    temperature: 0.5
    top_p: 1
    top_k: 40
    max_retries: 10
    parallel_tool_calls: true
    default_model: claude-4-sonnet

agents:
  trae_agent:
    enable_lakeview: false
    max_steps: 200
    tools:
      - bash

allow_mcp_servers: []
mcp_servers: {}
"""
        config = Config.create(config_string=config_string)

        # 验证 agent 使用 default_provider 和 default_model
        self.assertIsNotNone(config.trae_agent)
        self.assertEqual(config.trae_agent.model.model, "claude-4-sonnet")
        self.assertEqual(config.trae_agent.model.model_provider.provider, "anthropic")

    def test_cli_override_provider_and_model(self):
        """测试 CLI 参数覆盖 provider 和 model"""
        config = Config.create(config_string=self.get_new_config())

        # 使用 CLI 参数覆盖
        config = config.resolve_config_values(
            provider="openai",
            model="gpt-4o"
        )

        # 验证 trae_agent 被覆盖为 openai/gpt-4o
        self.assertIsNotNone(config.trae_agent)
        self.assertEqual(config.trae_agent.model.model, "gpt-4o")
        self.assertEqual(config.trae_agent.model.model_provider.provider, "openai")

    def test_agent_level_provider_and_model(self):
        """测试 agent 级别的 provider 和 model 配置"""
        config = Config.create(config_string=self.get_new_config())

        # 验证 trae_agent 使用 agent 级别的配置
        self.assertIsNotNone(config.trae_agent)
        self.assertEqual(config.trae_agent.provider, "anthropic")
        self.assertEqual(config.trae_agent.model_name, "claude-4-sonnet")

        # 验证 rca_agent 使用不同的 provider
        self.assertIsNotNone(config.rca_agent)
        self.assertEqual(config.rca_agent.provider, "openai")
        self.assertEqual(config.rca_agent.model_name, "gpt-4o")

    def test_missing_required_parameters_error(self):
        """测试缺少必需参数时抛出错误"""
        config_string = """
default_provider: anthropic

model_providers:
  anthropic:
    api_key: test-key
    provider: anthropic
    # 缺少必需的参数：max_tokens, temperature, top_p, top_k, max_retries, parallel_tool_calls
    default_model: claude-4-sonnet

agents:
  trae_agent:
    enable_lakeview: false
    max_steps: 200
    tools:
      - bash

allow_mcp_servers: []
mcp_servers: {}
"""
        # 应该抛出 ConfigError
        with self.assertRaises(Exception) as context:
            Config.create(config_string=config_string)

        # 验证错误消息包含缺少的参数信息
        error_msg = str(context.exception)
        self.assertIn("CONFIGURATION ERROR", error_msg)
        self.assertIn("Missing required parameters", error_msg)
        self.assertIn("max_tokens", error_msg)
        self.assertIn("temperature", error_msg)
        self.assertIn("top_p", error_msg)
        self.assertIn("top_k", error_msg)
        self.assertIn("max_retries", error_msg)
        self.assertIn("parallel_tool_calls", error_msg)
        # 验证错误消息包含配置示例
        self.assertIn("Option 1", error_msg)
        self.assertIn("Option 2", error_msg)
        self.assertIn("CONFIGURATION GUIDE", error_msg)

    def test_model_config_none_uses_provider_config(self):
        """测试模型配置为 None 时使用 provider 级别的配置"""
        config_string = """
default_provider: anthropic

model_providers:
  anthropic:
    api_key: test-key
    provider: anthropic
    # provider 级别的配置
    max_tokens: 8192
    temperature: 0.7
    top_p: 0.95
    top_k: 40
    max_retries: 15
    parallel_tool_calls: true
    default_model: claude-4-sonnet
    models:
      # 模型配置为 None（所有字段都被注释掉）
      claude-4-sonnet:
        # max_tokens: 4096
        # temperature: 0.5
        # 所有字段都被注释掉，应该使用 provider 级别的配置

agents:
  trae_agent:
    enable_lakeview: false
    max_steps: 200
    tools:
      - bash

allow_mcp_servers: []
mcp_servers: {}
"""
        config = Config.create(config_string=config_string)

        # 验证使用了 provider 级别的配置
        self.assertIsNotNone(config.trae_agent)
        self.assertEqual(config.trae_agent.model.model, "claude-4-sonnet")
        self.assertEqual(config.trae_agent.model.max_tokens, 8192)  # 来自 provider
        self.assertEqual(config.trae_agent.model.temperature, 0.7)  # 来自 provider
        self.assertEqual(config.trae_agent.model.max_retries, 15)  # 来自 provider


class TestConfigBaseURL(unittest.TestCase):
    def test_config_with_base_url_in_config(self):
        test_config = {
            "default_provider": "openai",
            "model_providers": {
                "openai": {
                    "model": "gpt-4o",
                    "api_key": "test-api-key",
                    "base_url": "https://custom-openai.example.com/v1",
                }
            },
        }

        config = Config.create_from_legacy_config(legacy_config=LegacyConfig(test_config))

        if config.trae_agent:
            trae_agent_config = config.trae_agent
        else:
            self.fail("trae_agent config is None")

        self.assertEqual(
            trae_agent_config.model.model_provider.base_url,
            "https://custom-openai.example.com/v1",
        )

    def test_config_without_base_url(self):
        test_config = {
            "default_provider": "openai",
            "model_providers": {
                "openai": {
                    "model": "gpt-4o",
                    "api_key": "test-api-key",
                }
            },
        }

        config = Config.create_from_legacy_config(legacy_config=LegacyConfig(test_config))

        if config.trae_agent:
            trae_agent_config = config.trae_agent
        else:
            self.fail("trae_agent config is None")

        self.assertIsNone(trae_agent_config.model.model_provider.base_url)

    def test_default_anthropic_base_url(self):
        config = Config.create_from_legacy_config(legacy_config=LegacyConfig({}))

        if config.trae_agent:
            trae_agent_config = config.trae_agent
        else:
            self.fail("trae_agent config is None")

        # If there are no model providers, the default provider is anthropic
        # and the default base_url is https://api.anthropic.com
        self.assertEqual(
            trae_agent_config.model.model_provider.base_url, "https://api.anthropic.com"
        )

    @patch("trae_agent.utils.llm_clients.openai_client.openai.OpenAI")
    def test_openai_client_with_custom_base_url(self, mock_openai):
        model_config = ModelConfig(
            model="gpt-4o",
            model_provider=ModelProvider(
                api_key="test-api-key",
                provider="openai",
                base_url="https://custom-openai.example.com/v1",
            ),
            max_tokens=4096,
            temperature=0.5,
            top_p=1,
            top_k=0,
            parallel_tool_calls=False,
            max_retries=10,
        )

        client = OpenAIClient(model_config)

        mock_openai.assert_called_once_with(
            api_key="test-api-key", base_url="https://custom-openai.example.com/v1"
        )
        self.assertEqual(client.base_url, "https://custom-openai.example.com/v1")

    @patch("trae_agent.utils.llm_clients.anthropic_client.anthropic.Anthropic")
    def test_anthropic_client_base_url_attribute_set(self, mock_anthropic):
        model_config = ModelConfig(
            model="claude-sonnet-4-20250514",
            model_provider=ModelProvider(
                api_key="test-api-key",
                provider="anthropic",
                base_url="https://custom-anthropic.example.com",
            ),
            max_tokens=4096,
            temperature=0.5,
            top_p=1,
            top_k=0,
            parallel_tool_calls=False,
            max_retries=10,
        )

        client = AnthropicClient(model_config)

        self.assertEqual(client.base_url, "https://custom-anthropic.example.com")

    @patch("trae_agent.utils.llm_clients.anthropic_client.anthropic.Anthropic")
    def test_anthropic_client_with_custom_base_url(self, mock_anthropic):
        model_config = ModelConfig(
            model="claude-sonnet-4-20250514",
            model_provider=ModelProvider(
                api_key="test-api-key",
                provider="anthropic",
                base_url="https://custom-anthropic.example.com",
            ),
            max_tokens=4096,
            temperature=0.5,
            top_p=1,
            top_k=0,
            parallel_tool_calls=False,
            max_retries=10,
        )

        client = AnthropicClient(model_config)

        mock_anthropic.assert_called_once_with(
            api_key="test-api-key", base_url="https://custom-anthropic.example.com"
        )
        self.assertEqual(client.base_url, "https://custom-anthropic.example.com")


class TestLakeviewConfig(unittest.TestCase):
    def get_base_config(self):
        return {
            "default_provider": "anthropic",
            "enable_lakeview": True,
            "model_providers": {
                "anthropic": {
                    "api_key": "anthropic-key",
                    "model": "claude-model",
                    "max_tokens": 4096,
                    "temperature": 0.5,
                    "top_p": 1,
                    "top_k": 0,
                    "max_retries": 10,
                },
                "doubao": {
                    "api_key": "doubao-key",
                    "model": "doubao-model",
                    "max_tokens": 8192,
                    "temperature": 0.5,
                    "top_p": 1,
                    "max_retries": 20,
                },
            },
        }

    def get_config_with_mcp_servers(self):
        return {
            "default_provider": "anthropic",
            "enable_lakeview": True,
            "model_providers": {
                "anthropic": {
                    "api_key": "anthropic-key",
                    "model": "claude-model",
                    "max_tokens": 4096,
                    "temperature": 0.5,
                    "top_p": 1,
                    "top_k": 0,
                    "max_retries": 10,
                },
                "doubao": {
                    "api_key": "doubao-key",
                    "model": "doubao-model",
                    "max_tokens": 8192,
                    "temperature": 0.5,
                    "top_p": 1,
                    "max_retries": 20,
                },
            },
            "mcp_servers": {"test_server": {"command": "echo", "args": [], "env": {}, "cwd": "."}},
        }

    def test_lakeview_defaults_to_main_provider(self):
        config_data = self.get_base_config()

        config = Config.create_from_legacy_config(legacy_config=LegacyConfig(config_data))
        assert config.lakeview is not None
        self.assertEqual(config.lakeview.model.model_provider.provider, "anthropic")
        self.assertEqual(config.lakeview.model.model, "claude-model")

    def test_lakeview_null_values_fallback(self):
        config_data = self.get_base_config()
        config_data["lakeview_config"] = {"model_provider": None, "model_name": None}

        config = Config.create_from_legacy_config(legacy_config=LegacyConfig(config_data))
        assert config.lakeview is not None
        self.assertEqual(config.lakeview.model.model_provider.provider, "anthropic")
        self.assertEqual(config.lakeview.model.model, "claude-model")

    def test_lakeview_disabled_ignores_config(self):
        config_data = self.get_base_config()
        config_data["enable_lakeview"] = False
        config_data["lakeview_config"] = {"model_provider": "doubao", "model_name": "some-model"}

        config = Config.create_from_legacy_config(legacy_config=LegacyConfig(config_data))
        self.assertIsNone(config.lakeview)

    def test_mcp_servers_config(self):
        config_data = self.get_config_with_mcp_servers()
        config = Config.create_from_legacy_config(legacy_config=LegacyConfig(config_data))
        self.assertIn("test_server", config.trae_agent.mcp_servers_config)
        self.assertEqual(config.trae_agent.mcp_servers_config["test_server"].command, "echo")
        self.assertEqual(config.trae_agent.mcp_servers_config["test_server"].args, [])
        self.assertEqual(config.trae_agent.mcp_servers_config["test_server"].env, {})
        self.assertEqual(config.trae_agent.mcp_servers_config["test_server"].cwd, ".")

    def test_mcp_servers_empty_config(self):
        config_data = self.get_base_config()
        config = Config.create_from_legacy_config(legacy_config=LegacyConfig(config_data))

        self.assertEqual(config.trae_agent.mcp_servers_config, {})


if __name__ == "__main__":
    unittest.main()
