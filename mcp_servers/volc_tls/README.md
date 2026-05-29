# VolcTLS MCP Server

火山云 TLS（日志服务）MCP 服务器，用于与 Trae Agent 集成，实现日志查询功能。

## 功能特性

- **list_topics**: 列出所有日志主题
- **search_logs**: 根据查询条件检索日志
- **get_topic_info**: 获取日志主题详情
- **get_log_stream**: 获取日志流信息

## 环境要求

- Python 3.10+
- 火山云账号及 API 密钥

## 安装

```bash
cd mcp_servers/volc_tls
pip install -e .
```

## 配置

设置环境变量：

```bash
export VOLC_ACCESS_KEY=your_access_key
export VOLC_SECRET_KEY=your_secret_key
export VOLC_REGION=cn-beijing
export VOLC_TOPIC_ID=your_default_topic_id
```

| 环境变量 | 说明 | 必填 |
|----------|------|------|
| VOLC_ACCESS_KEY | 火山云 Access Key | 是 |
| VOLC_SECRET_KEY | 火山云 Secret Key | 是 |
| VOLC_REGION | 区域（如 cn-beijing） | 是 |
| VOLC_TOPIC_ID | 默认日志主题 ID（可选） | 否 |

## 运行

```bash
python -m mcp_servers.volc_tls.main
```

## Trae Agent 配置

在 `trae_config.yaml` 中添加：

```yaml
allow_mcp_servers:
  - volc_tls

mcp_servers:
  volc_tls:
    command: python
    args:
      - -m
      - mcp_servers.volc_tls.main
    env:
      VOLC_ACCESS_KEY: ${VOLC_ACCESS_KEY}
      VOLC_SECRET_KEY: ${VOLC_SECRET_KEY}
      VOLC_REGION: cn-beijing
    cwd: ${PROJECT_ROOT}
```

## API 说明

### list_topics

列出所有日志主题

**参数**:
- `project_id` (可选): 项目 ID

### search_logs

检索日志

**参数**:
- `topic_id` (可选): 日志主题 ID，未指定时使用 VOLC_TOPIC_ID 环境变量
- `query` (必填): 查询语句，支持 Lucene 语法
- `start_time` (必填): 开始时间戳（秒）
- `end_time` (必填): 结束时间戳（秒）
- `limit` (可选): 返回条数限制，默认 100
- `query_language` (可选): 查询语言，支持 Lucene 或 SQL，默认 Lucene

### get_topic_info

获取主题详情

**参数**:
- `topic_id` (可选): 日志主题 ID，未指定时使用 VOLC_TOPIC_ID 环境变量

### get_log_stream

获取日志流信息

**参数**:
- `topic_id` (可选): 日志主题 ID，未指定时使用 VOLC_TOPIC_ID 环境变量
- `shard_id` (可选): 分片 ID

## 查询语法

支持 Lucene 查询语法：

| 语法 | 示例 | 说明 |
|------|------|------|
| 精确匹配 | `level:ERROR` | 查询 level 字段为 ERROR 的日志 |
| 模糊匹配 | `message:*error*` | 查询 message 包含 error 的日志 |
| 范围查询 | `time:[1704067200 TO 1704153600]` | 查询时间范围内的日志 |
| 逻辑运算 | `level:ERROR AND service:api` | 多条件 AND 查询 |

## 测试

```bash
cd mcp_servers/volc_tls
pip install -e ".[dev]"
pytest
```

## 打包

```bash
cd mcp_servers/volc_tls
pip install pyinstaller
pyinstaller --onefile main.py --name volc-tls-mcp
```