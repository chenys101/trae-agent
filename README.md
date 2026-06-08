# Trae Agent

[![arXiv:2507.23370](https://img.shields.io/badge/TechReport-arXiv%3A2507.23370-b31a1b)](https://arxiv.org/abs/2507.23370)
[![Python 3.12+](https://img.shields.io/badge/python-3.12+-blue.svg)](https://www.python.org/downloads/) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Pre-commit](https://github.com/bytedance/trae-agent/actions/workflows/pre-commit.yml/badge.svg)](https://github.com/bytedance/trae-agent/actions/workflows/pre-commit.yml)
[![Unit Tests](https://github.com/bytedance/trae-agent/actions/workflows/unit-test.yml/badge.svg)](https://github.com/bytedance/trae-agent/actions/workflows/unit-test.yml)
[![Discord](https://img.shields.io/discord/1320998163615846420?label=Join%20Discord&color=7289DA)](https://discord.gg/VwaQ4ZBHvC)

**Trae Agent** is an LLM-based agent for general purpose software engineering tasks. It provides a powerful CLI interface that can understand natural language instructions and execute complex software engineering workflows using various tools and LLM providers.

For technical details please refer to [our technical report](https://arxiv.org/abs/2507.23370).

**Project Status:** The project is still being actively developed. Please refer to [docs/roadmap.md](docs/roadmap.md) and [CONTRIBUTING](CONTRIBUTING.md) if you are willing to help us improve Trae Agent.

**Difference with Other CLI Agents:** Trae Agent offers a transparent, modular architecture that researchers and developers can easily modify, extend, and analyze, making it an ideal platform for **studying AI agent architectures, conducting ablation studies, and developing novel agent capabilities**. This **_research-friendly design_** enables the academic and open-source communities to contribute to and build upon the foundational agent framework, fostering innovation in the rapidly evolving field of AI agents.

## ✨ Features

- 🌊 **Lakeview**: Provides short and concise summarisation for agent steps
- 🤖 **Multi-LLM Support**: Works with OpenAI, Anthropic, Doubao, Azure, OpenRouter, Ollama and Google Gemini APIs
- 🛠️ **Rich Tool Ecosystem**: File editing, bash execution, sequential thinking, and more
- 🎯 **Interactive Mode**: Conversational interface for iterative development
- 📊 **Trajectory Recording**: Detailed logging of all agent actions for debugging and analysis
- ⚙️ **Flexible Configuration**: YAML-based configuration with environment variable support
- 🚀 **Easy Installation**: Simple pip-based installation

## 🚀 Installation

### Requirements
- UV (https://docs.astral.sh/uv/)
- API key for your chosen provider (OpenAI, Anthropic, Google Gemini, OpenRouter, etc.)

### Setup

```bash
git clone https://github.com/bytedance/trae-agent.git
cd trae-agent
uv sync --all-extras
source .venv/bin/activate
```

## ⚙️ Configuration

### YAML Configuration (Recommended)

1. Copy the example configuration file:
   ```bash
   cp trae_config.yaml.example trae_config.yaml
   ```

2. Edit `trae_config.yaml` with your API credentials and preferences:

```yaml
# Global default provider
default_provider: anthropic

# Model providers configuration
model_providers:
  anthropic:
    api_key: your_anthropic_api_key
    provider: anthropic
    # Provider-level model parameters (recommended for all models)
    max_tokens: 4096
    temperature: 0.5
    top_p: 1
    top_k: 40
    max_retries: 10
    parallel_tool_calls: true
    # Provider's default model
    default_model: claude-sonnet-4-20250514
    # Model-specific configurations (optional)
    models:
      claude-sonnet-4-20250514:
        max_tokens: 4096
        temperature: 0.5
        top_p: 1
        top_k: 40
        max_retries: 10
        parallel_tool_calls: true

  openai:
    api_key: your_openai_api_key
    provider: openai
    # Provider-level model parameters
    max_tokens: 4096
    temperature: 0.5
    top_p: 1
    top_k: 0
    max_retries: 10
    parallel_tool_calls: true
    default_model: gpt-4o

# Lakeview configuration (optional)
lakeview:
  provider: anthropic  # Optional, uses default_provider if not specified
  model: claude-3.5-sonnet  # Optional, uses provider's default_model if not specified

# Agents configuration
agents:
  trae_agent:
    enable_lakeview: true
    # Agent-level provider and model configuration (optional)
    provider: anthropic  # Optional, uses default_provider if not specified
    model: claude-sonnet-4-20250514  # Optional, uses provider's default_model if not specified
    max_steps: 200
    tools:
      - bash
      - str_replace_based_edit_tool
      - sequentialthinking
      - task_done

  rca_agent:
    provider: openai  # Different provider for RCA agent
    model: gpt-4o
    max_steps: 15
    tools:
      - business_analysis
      - log_analysis
      - task_done
```

**Key Configuration Features:**

- **Default Provider**: Set a global default provider to avoid repeating configuration
- **Provider-Level Parameters**: Configure model parameters at the provider level (recommended)
- **Model-Specific Parameters**: Override provider-level parameters for specific models
- **Agent-Level Configuration**: Each agent can use a different provider and model
- **Flexible Priority**: CLI arguments > Agent config > Default provider/model

**Note:** The `trae_config.yaml` file is ignored by git to protect your API keys.

### Configuration Priority

**Provider Selection Priority:**
1. CLI `--provider` argument
2. Agent configuration `provider` field
3. Global `default_provider` configuration
4. Error if not configured

**Model Selection Priority:**
1. CLI `--model` argument
2. Agent configuration `model` field
3. Provider's `default_model` configuration
4. Error if not configured

**Model Parameters Priority:**
1. Model-specific configuration (in `provider.models.<model_name>`)
2. Provider-level configuration
3. Error if required parameters not configured

### Using Base URL

In some cases, we need to use a custom URL for the api. Just add the `base_url` field after `provider`:

```yaml
model_providers:
  openai:
    api_key: your_openrouter_api_key
    provider: openai
    base_url: https://openrouter.ai/api/v1
    max_tokens: 4096
    temperature: 0.5
    top_p: 1
    top_k: 0
    max_retries: 10
    parallel_tool_calls: true
    default_model: gpt-4o
```

**Note:** For field formatting, use spaces only. Tabs (\t) are not allowed.

### Environment Variables (Alternative)

You can also configure API keys using environment variables and store them in the .env file:

```bash
export OPENAI_API_KEY="your-openai-api-key"
export OPENAI_BASE_URL="your-openai-base-url"
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export ANTHROPIC_BASE_URL="your-anthropic-base-url"
export GOOGLE_API_KEY="your-google-api-key"
export GOOGLE_BASE_URL="your-google-base-url"
export OPENROUTER_API_KEY="your-openrouter-api-key"
export OPENROUTER_BASE_URL="https://openrouter.ai/api/v1"
export DOUBAO_API_KEY="your-doubao-api-key"
export DOUBAO_BASE_URL="https://ark.cn-beijing.volces.com/api/v3/"
```

### MCP Services (Optional)

To enable Model Context Protocol (MCP) services, add an `mcp_servers` section to your configuration:

```yaml
mcp_servers:
  playwright:
    command: npx
    args:
      - "@playwright/mcp@0.0.27"
```

**Legacy JSON Configuration:** If using the older JSON format, see [docs/legacy_config.md](docs/legacy_config.md). We recommend migrating to YAML.

## 📖 Usage

### Basic Commands

```bash
# Simple task execution (uses default provider and model)
trae-cli run "Create a hello world Python script"

# Check configuration
trae-cli show-config

# Interactive mode
trae-cli interactive
```

### CLI Parameter Override

You can override provider and model settings via CLI arguments:

```bash
# Override provider and model
trae-cli run "Fix the bug in main.py" --provider openai --model gpt-4o

# Override only provider (uses provider's default_model)
trae-cli run "Add unit tests" --provider anthropic

# Override only model (uses default_provider)
trae-cli run "Optimize algorithm" --model claude-sonnet-4-20250514
```

**Note:** CLI arguments have the highest priority and will override all configuration file settings.

### Provider-Specific Examples

```bash
# OpenAI
trae-cli run "Fix the bug in main.py" --provider openai --model gpt-4o

# Anthropic
trae-cli run "Add unit tests" --provider anthropic --model claude-sonnet-4-20250514

# Google Gemini
trae-cli run "Optimize this algorithm" --provider google --model gemini-2.5-flash

# OpenRouter (access to multiple providers)
trae-cli run "Review this code" --provider openrouter --model "anthropic/claude-3-5-sonnet"
trae-cli run "Generate documentation" --provider openrouter --model "openai/gpt-4o"

# Doubao
trae-cli run "Refactor the database module" --provider doubao --model doubao-seed-1.6

# Ollama (local models)
trae-cli run "Comment this code" --provider ollama --model qwen3
```

### Advanced Options

```bash
# Custom working directory
trae-cli run "Add tests for utils module" --working-dir /path/to/project

# Save execution trajectory
trae-cli run "Debug authentication" --trajectory-file debug_session.json

# Force patch generation
trae-cli run "Update API endpoints" --must-patch

# Interactive mode with custom settings
trae-cli interactive --provider openai --model gpt-4o --max-steps 30
```

### RCA Agent (Root Cause Analysis)

Trae Agent includes a specialized RCA (Root Cause Analysis) agent for analyzing and diagnosing issues in your codebase:

```bash
# Basic RCA analysis
trae-cli rca-report "Authentication failure in login service" -c /path/to/codebase

# Save RCA report to file
trae-cli rca-report "Database connection timeout" -c /path/to/codebase -o rca_report.md

# Use documentation path for context
trae-cli rca-report "API response delay" -c /path/to/codebase -dp documentation

# Load fault description from file
trae-cli rca-report --file fault_description.txt -c /path/to/codebase

# Specify provider and model for RCA
trae-cli rca-report "Memory leak in worker process" -c /path/to/codebase --provider anthropic --model claude-sonnet-4-20250514

# Save execution trajectory for debugging
trae-cli rca-report "Service crash analysis" -c /path/to/codebase --trajectory-file rca_debug.json
```

**RCA Agent Features:**
- **Business Analysis**: Analyzes business logic and system architecture
- **Log Analysis**: Examines logs for error patterns and anomalies
- **Root Cause Identification**: Uses LLM reasoning to identify potential root causes
- **Comprehensive Reports**: Generates detailed Markdown reports with findings and recommendations

**Configuration:**
```yaml
agents:
  rca_agent:
    provider: openai  # Can use different provider than trae_agent
    model: gpt-4o
    max_steps: 15
    codebase: /path/to/default/codebase  # Default codebase path
    tools:
      - business_analysis
      - log_analysis
      - task_done
```

## Docker Mode Commands
### Preparation
**Important**: You need to make sure Docker is configured in your environment.

### Usage
```bash
# Specify a Docker image to run the task in a new container
trae-cli run "Add tests for utils module" --docker-image python:3.11

# Specify a Docker image to run the task in a new container and mount the directory
trae-cli run "write a script to print helloworld" --docker-image python:3.12 --working-dir test_workdir/

# Attach to an existing Docker container by ID (`--working-dir` is invalid with `--docker-container-id`)
trae-cli run "Update API endpoints" --docker-container-id 91998a56056c

# Specify an absolute path to a Dockerfile to build an environment
trae-cli run "Debug authentication" --dockerfile-path test_workspace/Dockerfile

# Specify a path to a local Docker image file (tar archive) to load
trae-cli run "Fix the bug in main.py" --docker-image-file test_workspace/trae_agent_custom.tar

# Remove the Docker container after finishing the task (keep default)
trae-cli run "Add tests for utils module" --docker-image python:3.11 --docker-keep false
```

### Interactive Mode Commands

In interactive mode, you can use:
- Type any task description to execute it
- `status` - Show agent information
- `help` - Show available commands
- `clear` - Clear the screen
- `exit` or `quit` - End the session

## 🛠️ Advanced Features

### Available Tools

Trae Agent provides a comprehensive toolkit for software engineering tasks including file editing, bash execution, structured thinking, and task completion. For detailed information about all available tools and their capabilities, see [docs/tools.md](docs/tools.md).

### Trajectory Recording

Trae Agent automatically records detailed execution trajectories for debugging and analysis:

```bash
# Auto-generated trajectory file
trae-cli run "Debug the authentication module"
# Saves to: trajectories/trajectory_YYYYMMDD_HHMMSS.json

# Custom trajectory file
trae-cli run "Optimize database queries" --trajectory-file optimization_debug.json
```

Trajectory files contain LLM interactions, agent steps, tool usage, and execution metadata. For more details, see [docs/TRAJECTORY_RECORDING.md](docs/TRAJECTORY_RECORDING.md).

## 🔧 Development

### Contributing

For contribution guidelines, please refer to [CONTRIBUTING.md](CONTRIBUTING.md).

### Troubleshooting

**Import Errors:**
```bash
PYTHONPATH=. trae-cli run "your task"
```

**API Key Issues:**
```bash
# Verify API keys
echo $OPENAI_API_KEY
trae-cli show-config
```

**Command Not Found:**
```bash
uv run trae-cli run "your task"
```

**Permission Errors:**
```bash
chmod +x /path/to/your/project
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## ✍️ Citation

```bibtex
@article{traeresearchteam2025traeagent,
      title={Trae Agent: An LLM-based Agent for Software Engineering with Test-time Scaling},
      author={Trae Research Team and Pengfei Gao and Zhao Tian and Xiangxin Meng and Xinchen Wang and Ruida Hu and Yuanan Xiao and Yizhou Liu and Zhao Zhang and Junjie Chen and Cuiyun Gao and Yun Lin and Yingfei Xiong and Chao Peng and Xia Liu},
      year={2025},
      eprint={2507.23370},
      archivePrefix={arXiv},
      primaryClass={cs.SE},
      url={https://arxiv.org/abs/2507.23370},
}
```

## 🙏 Acknowledgments

We thank Anthropic for building the [anthropic-quickstart](https://github.com/anthropics/anthropic-quickstarts) project that served as a valuable reference for the tool ecosystem.
