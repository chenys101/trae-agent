# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
"""
Business Models
业务分析数据模型
"""
from typing import List, Optional
from pydantic import BaseModel, Field


class FaultBusinessResult(BaseModel):
    """业务识别结果"""

    business: str = Field(
        description="业务流程名称"
    )

    domain: str = Field(
        description="业务域"
    )

    guide: str = Field(
        description="business guide 路径"
    )

    confidence: float = Field(
        description="识别置信度"
    )

    reason: str = Field(
        description="识别原因"
    )

    matched_keywords: List[str] = Field(
        default_factory=list,
        description="命中的关键词"
    )

    log_keywords: List[str] = Field(
        default_factory=list,
        description="从业务文档中提取的日志搜索关键词"
    )

    business_entry_points: List[str] = Field(
        default_factory=list,
        description="业务入口点，如 MQ topic、API 路径、controller 等"
    )

    key_services: List[str] = Field(
        default_factory=list,
        description="相关的关键服务/模块"
    )

    code_locations: List[str] = Field(
        default_factory=list,
        description="相关的代码文件位置"
    )

    business_process: str = Field(
        default="",
        description="业务流程简要描述"
    )

    log_message_templates: List[str] = Field(
        default_factory=list,
        description="从代码中提取的日志消息模板，例如：'创建零售订单发货单相关记录V2'"
    )

    container_names: List[str] = Field(
        default_factory=list,
        description="相关的 K8s 容器名称，如 order-server, order-server-tms"
    )

    search_logs_reminder: str = Field(
        default="",
        description="自动生成的 search_logs 调用提醒"
    )

    topic_id: str = Field(
        default="",
        description="TLS Topic ID，用于 search_logs 调用。生产环境使用 VOLCENGINE_TOPIC_ID_PRODUCTION"
    )
