import asyncio
import json
import os
import time
from typing import Any, Dict, Optional

from mcp.server import FastMCP

# Fix SSL keylog permission issue by removing SSLKEYLOGFILE env var
if 'SSLKEYLOGFILE' in os.environ:
    del os.environ['SSLKEYLOGFILE']


class TLSClient:
    def __init__(self, access_key_id: str, secret_access_key: str, region: str = "cn-beijing", 
                 endpoint: Optional[str] = None, connection_timeout: int = 10, socket_timeout: int = 30):
        self.access_key_id = access_key_id
        self.secret_access_key = secret_access_key
        self.region = region
        self.endpoint = endpoint or f"tls-{self.region}.volces.com"
        self.connection_timeout = connection_timeout
        self.socket_timeout = socket_timeout
        self._client = None
        self._initialized = False

    def _init_client(self):
        try:
            from volcengine.tls.TLSService import TLSService
            from volcengine.tls.tls_requests import (
                DescribeTopicsRequest,
                SearchLogsRequest,
                DescribeTopicRequest,
                DescribeCursorRequest,
                ConsumeLogsRequest
            )
            
            # Create the client
            self._client = TLSService(
                endpoint=self.endpoint,
                access_key_id=self.access_key_id,
                access_key_secret=self.secret_access_key,
                region=self.region
            )
            
            # Set timeout settings - THIS IS KEY!
            self._client.set_connection_timeout(self.connection_timeout)
            self._client.set_socket_timeout(self.socket_timeout)
            
            self._initialized = True
        except ImportError:
            raise ImportError(
                "volcengine SDK not installed. Please install it with:\n"
                "  pip install volcengine\n"
                "Or for this project:\n"
                "  pip install -e '.[volc_tls]'"
            )
        except Exception as e:
            raise RuntimeError(f"Failed to initialize TLS client: {str(e)}")

    def list_topics(self, project_id: Optional[str] = None) -> Dict[str, Any]:
        if not self._initialized:
            self._init_client()
        
        from volcengine.tls.tls_requests import DescribeTopicsRequest, DescribeProjectsRequest
        
        try:
            if project_id:
                request = DescribeTopicsRequest(project_id=project_id)
                result = self._client.describe_topics(request)
                return self._convert_to_dict(result)
            else:
                projects_request = DescribeProjectsRequest()
                projects_result = self._client.describe_projects(projects_request)
                projects = self._convert_to_dict(projects_result).get("projects", [])
                
                all_topics = []
                for project in projects:
                    proj_id = project.get("project_id")
                    if proj_id:
                        try:
                            topics_request = DescribeTopicsRequest(project_id=proj_id)
                            topics_result = self._client.describe_topics(topics_request)
                            topics_data = self._convert_to_dict(topics_result)
                            for topic in topics_data.get("topics", []):
                                topic["project_id"] = proj_id
                                topic["project_name"] = project.get("project_name")
                            all_topics.extend(topics_data.get("topics", []))
                        except Exception:
                            continue
                
                return {"topics": all_topics}
        except Exception as e:
            raise RuntimeError(f"Failed to list topics: {str(e)}")

    def search_logs(
        self,
        topic_id: str,
        query: str,
        start_time: int,
        end_time: int,
        limit: int = 100,
        query_language: str = "Lucene"
    ) -> Dict[str, Any]:
        if not self._initialized:
            self._init_client()
        
        from volcengine.tls.tls_requests import SearchLogsRequest
        
        try:
            # Convert time to milliseconds if needed (volcengine expects ms)
            if start_time < 10000000000:
                start_time = start_time * 1000
            if end_time < 10000000000:
                end_time = end_time * 1000
            
            request = SearchLogsRequest(
                topic_id=topic_id,
                query=query,
                start_time=start_time,
                end_time=end_time,
                limit=limit
            )
            result = self._client.search_logs_v2(request)
            return self._convert_to_dict(result)
        except Exception as e:
            raise RuntimeError(f"Failed to search logs: {str(e)}")

    def get_topic(self, topic_id: str) -> Dict[str, Any]:
        if not self._initialized:
            self._init_client()
        
        from volcengine.tls.tls_requests import DescribeTopicRequest
        
        try:
            request = DescribeTopicRequest(topic_id)
            result = self._client.describe_topic(request)
            return self._convert_to_dict(result)
        except Exception as e:
            raise RuntimeError(f"Failed to get topic: {str(e)}")

    def get_log_stream(self, topic_id: str, shard_id: Optional[int] = None) -> Dict[str, Any]:
        if not self._initialized:
            self._init_client()
        
        from volcengine.tls.tls_requests import DescribeCursorRequest, ConsumeLogsRequest
        
        try:
            cursor_request = DescribeCursorRequest(topic_id, shard_id=shard_id or 0, from_time="begin")
            cursor_response = self._client.describe_cursor(cursor_request)
            
            consume_request = ConsumeLogsRequest(topic_id, shard_id=shard_id or 0, cursor=cursor_response.cursor)
            result = self._client.consume_logs(consume_request)
            return self._convert_to_dict(result)
        except Exception as e:
            raise RuntimeError(f"Failed to get log stream: {str(e)}")

    @staticmethod
    def _convert_to_dict(obj: Any) -> Dict[str, Any]:
        if hasattr(obj, '__dict__'):
            result = {}
            for k, v in obj.__dict__.items():
                if not k.startswith('_'):
                    result[k] = TLSClient._convert_to_dict(v)
            return result
        elif isinstance(obj, list):
            return [TLSClient._convert_to_dict(item) for item in obj]
        elif isinstance(obj, (int, str, bool, float, type(None))):
            return obj
        else:
            return str(obj)


class VolcTLSMCP(FastMCP):
    PROD_TOPIC_ID = "43a4fcb6-a6e7-4102-bf62-1c7dc51adf61"

    def __init__(self):
        super().__init__(
            name="VolcTLS",
            instructions="火山云 TLS 日志服务 MCP 服务器，提供日志查询功能"
        )
        self._client = None
        self._default_topic_id = os.environ.get("VOLC_TOPIC_ID", os.environ.get("VOLCENGINE_TOPIC_ID", ""))
        self._default_project_id = os.environ.get("VOLC_PROJECT_ID", os.environ.get("VOLCENGINE_PROJECT_ID", None))

    def _get_client(self) -> TLSClient:
        if self._client is None:
            # Use official VOLCENGINE_ prefix first, then VOLC_ for backward compatibility
            access_key = os.environ.get("VOLCENGINE_ACCESS_KEY_ID") or os.environ.get("VOLC_ACCESS_KEY")
            secret_key = os.environ.get("VOLCENGINE_ACCESS_KEY_SECRET") or os.environ.get("VOLC_SECRET_KEY")
            region = os.environ.get("VOLCENGINE_REGION") or os.environ.get("VOLC_REGION", "cn-beijing")
            endpoint = os.environ.get("VOLCENGINE_ENDPOINT") or os.environ.get("VOLC_ENDPOINT")
            
            if not access_key or not secret_key:
                raise RuntimeError(
                    "Missing required environment variables for VolcTLS:\n"
                    "  VOLCENGINE_ACCESS_KEY_ID (or VOLC_ACCESS_KEY) and \n"
                    "  VOLCENGINE_ACCESS_KEY_SECRET (or VOLC_SECRET_KEY) must be set\n"
                    "\n"
                    "Please set them before running the agent:\n"
                    "  export VOLCENGINE_ACCESS_KEY_ID=your_access_key\n"
                    "  export VOLCENGINE_ACCESS_KEY_SECRET=your_secret_key\n"
                    "  export VOLCENGINE_REGION=cn-beijing  # optional\n"
                    "\n"
                    "Or configure in trae_config.yaml"
                )

            self._client = TLSClient(
                access_key_id=access_key,
                secret_access_key=secret_key,
                region=region,
                endpoint=endpoint
            )
        return self._client

    def _get_topic_id(self, arguments: Dict[str, Any]) -> str:
        """Get topic_id from arguments or environment variable, fallback to production"""
        topic_id = arguments.get("topic_id")
        if topic_id:
            return topic_id
        if self._default_topic_id:
            return self._default_topic_id
        return self.PROD_TOPIC_ID

    async def list_topics(self, arguments: Dict[str, Any]) -> str:
        """List all log topics. Parameters: project_id (optional, project ID)"""
        try:
            project_id = arguments.get("project_id", self._default_project_id)
            client = self._get_client()
            result = await asyncio.to_thread(client.list_topics, project_id)
            
            result_dict = self._parse_result(result)
            topics = result_dict.get("topics", [])
            simplified_topics = [{"topic_name": t.get("topic_name"), "topic_id": t.get("topic_id")} for t in topics]
            
            return json.dumps({"topics": simplified_topics}, ensure_ascii=False, indent=2)
        except Exception as e:
            raise RuntimeError(f"Error: {str(e)}")

    async def search_logs(self, arguments: Dict[str, Any]) -> str:
        """Search logs in Volcengine TLS service. 
        
        Parameters:
        - topic_id (str, recommended): Log topic ID. Production: "43a4fcb6-a6e7-4102-bf62-1c7dc51adf61", UAT: "8480d34d-2262-4ddc-b3a8-6125969a7991", Test: "7e4cfbe2-91dd-4f84-8298-d9cbc3475760". If not provided, uses VOLCENGINE_TOPIC_ID env var.
        - query (str, required): Search query. Supports Lucene syntax. 
          NOTE: TLS full-text search AND operator does NOT work reliably for Chinese keywords.
          When using combined keywords like "关键词1 AND 关键词2", the system will automatically
          fall back to searching each keyword separately and filtering results on the backend.
          Recommended: just pass the most specific keyword (e.g., order number "95358532945").
        - start_time (int, optional): Start time in seconds since epoch. If not provided, uses progressive search.
        - end_time (int, optional): End time in seconds since epoch. Default: now.
        - limit (int, optional): Max results. Default: 100.
        - query_language (str, optional): Query language. Default: "Lucene".
        - container_name (str or list, optional): K8s container name(s) to filter results.
          - Single: "order-server"
          - Multiple: ["order-server", "order-gateway"] or "order-server,order-gateway"
          When specified, results are filtered to only include logs from these containers.
          This is applied as a backend filter since TLS AND operator does not work reliably with __container_name__.
          If business_analysis returns container_names, use ALL of them (comma-separated or as a list).
        - progressive (bool, optional): Enable progressive time range search (2h→12h→24h→48h→72h→168h). Default: true.
        - max_content_length (int, optional): Max characters per log entry content. Default: 2000. Set to 0 to disable truncation.
        
        IMPORTANT: Always pass topic_id explicitly for reliability. For combined search, use "keyword1 AND keyword2" 
        syntax - the system will auto-fallback to single-keyword search + backend filtering if AND returns 0 results.
        Use container_name to narrow results to a specific service (e.g., "order-server").
        """
        try:
            topic_id = self._get_topic_id(arguments)
            query = arguments.get("query")
            start_time = arguments.get("start_time")
            end_time = arguments.get("end_time")
            limit = arguments.get("limit", 100)
            query_language = arguments.get("query_language", "Lucene")
            progressive = arguments.get("progressive", True)
            container_name = arguments.get("container_name")
            max_content_length = arguments.get("max_content_length", 2000)

            # MANDATORY: container_name is required for search_logs
            if not container_name:
                return json.dumps({
                    "error": "container_name is MANDATORY for search_logs! "
                             "Please provide container_name parameter from business_analysis result. "
                             "Example: container_name='order-server' or container_name='order-server,order-gateway'",
                    "error_code": -1,
                    "hint": "This is a mandatory parameter to ensure search results are filtered to the correct service."
                }, ensure_ascii=False)

            # Convert container_name to list format for multi-container support
            container_names = []
            if container_name:
                if isinstance(container_name, list):
                    container_names = container_name
                elif isinstance(container_name, str):
                    if "," in container_name:
                        container_names = [c.strip() for c in container_name.split(",") if c.strip()]
                    else:
                        container_names = [container_name]

            if not query:
                raise ValueError("Missing required parameter: query")

            # Get current time if end_time not provided
            now = int(time.time())
            if end_time is None:
                end_time = now
            
            # Define time ranges in hours for progressive search
            time_ranges = [2, 12, 24, 48, 72, 168]  # 2h, 12h, 24h, 48h, 72h, 7d
            
            import re as _re
            and_keywords = []
            and_match = _re.split(r'\s+AND\s+', query)
            if len(and_match) > 1:
                and_keywords = [k.strip().strip('"').strip("'") for k in and_match if k.strip()]
            
            client = self._get_client()
            
            def simplify_log_result(raw_result):
                result_dict = self._parse_result(raw_result)
                
                search_result = result_dict.get("search_result", {})
                if not isinstance(search_result, dict):
                    search_result = {}
                
                hit_count = result_dict.get("hit_count", 0)
                if hit_count == 0:
                    hit_count = search_result.get("hit_count", 0)
                
                logs = result_dict.get("logs", [])
                if not logs:
                    logs = search_result.get("logs", [])
                if not logs:
                    logs = result_dict.get("result", [])
                if not logs:
                    logs = result_dict.get("items", [])
                
                simplified_logs = []
                for log in logs:
                    log_dict = None
                    if isinstance(log, dict):
                        log_dict = log
                    elif isinstance(log, str):
                        try:
                            log_dict = json.loads(log)
                        except (json.JSONDecodeError, ValueError):
                            try:
                                import ast
                                log_dict = ast.literal_eval(log)
                                if not isinstance(log_dict, dict):
                                    log_dict = None
                            except Exception:
                                simplified_logs.append({"content": log})
                                continue
                    
                    if log_dict is None:
                        simplified_logs.append({"content": str(log)})
                        continue
                    
                    content = log_dict.get("content", "")
                    if not content:
                        content = log_dict.get("log", "")
                    if not content:
                        content = log_dict.get("__content__", "")
                    if not content:
                        content = log_dict.get("message", "")
                    if not content:
                        content = log_dict.get("msg", "")
                    if not content:
                        non_meta = {k: v for k, v in log_dict.items() if not k.startswith("__")}
                        if non_meta:
                            content = json.dumps(non_meta, ensure_ascii=False)
                        else:
                            content = json.dumps(log_dict, ensure_ascii=False)
                    
                    time_val = log_dict.get("time", "")
                    if not time_val:
                        time_val = log_dict.get("@timestamp", "")
                    if not time_val:
                        time_val = log_dict.get("timestamp", "")
                    if not time_val:
                        time_val = log_dict.get("__time__", "")
                    
                    container_val = log_dict.get("__container_name__", "")
                    
                    # Truncate content if max_content_length is set
                    if max_content_length > 0 and len(content) > max_content_length:
                        content = content[:max_content_length] + f"... [truncated {len(content) - max_content_length} chars]"
                    
                    simplified_logs.append({"time": str(time_val), "container_name": container_val, "content": content})
                
                return {"hit_count": hit_count, "logs": simplified_logs, "_raw_keys": list(result_dict.keys())}
            
            def filter_logs_by_keywords(simplified_result, keywords):
                if not keywords or len(keywords) <= 1:
                    return simplified_result
                filtered_logs = []
                for log_entry in simplified_result.get("logs", []):
                    log_text = log_entry.get("content", "")
                    if all(kw in log_text for kw in keywords):
                        filtered_logs.append(log_entry)
                simplified_result["logs"] = filtered_logs
                simplified_result["hit_count"] = len(filtered_logs)
                simplified_result["filtered_by_keywords"] = keywords
                return simplified_result
            def filter_logs_by_container(simplified_result, cnames):
                if not cnames:
                    return simplified_result
                # Support both single string and list
                if isinstance(cnames, str):
                    cnames = [cnames]
                if not cnames:
                    return simplified_result
                filtered_logs = []
                for log_entry in simplified_result.get("logs", []):
                    log_container = log_entry.get("container_name", "")
                    if not log_container:
                        log_container = log_entry.get("__container_name__", "")
                    # Match any of the container names
                    for cname in cnames:
                        if cname in log_container or log_container == cname:
                            filtered_logs.append(log_entry)
                            break
                simplified_result["logs"] = filtered_logs
                simplified_result["hit_count"] = len(filtered_logs)
                simplified_result["filtered_by_container"] = cnames if isinstance(cnames, list) else [cnames]
                return simplified_result
            
            async def do_search(search_query, search_start, search_end, search_limit):
                result = await asyncio.to_thread(
                    client.search_logs,
                    topic_id=topic_id,
                    query=search_query,
                    start_time=search_start,
                    end_time=search_end,
                    limit=search_limit,
                    query_language=query_language
                )
                return simplify_log_result(result)
            
            # If start_time is specified, do a single search
            if start_time is not None:
                simplified = await do_search(query, start_time, end_time, limit)
                
                if simplified.get("hit_count", 0) == 0 and and_keywords:
                    primary_keyword = and_keywords[0]
                    simplified = await do_search(primary_keyword, start_time, end_time, limit)
                    if simplified.get("hit_count", 0) > 0:
                        simplified = filter_logs_by_keywords(simplified, and_keywords)
                        simplified["fallback_search"] = f"AND query returned 0, fell back to single keyword: {primary_keyword}"
                
                if container_names:
                    simplified = filter_logs_by_container(simplified, container_names)
                simplified["topic_id"] = topic_id
                simplified["query"] = query
                simplified["container_name"] = container_names if container_names else ""
                simplified["time_range"] = {"start": start_time, "end": end_time}
                return json.dumps(simplified, ensure_ascii=False, indent=2)
            
            # Progressive search: start with smallest time range and expand
            if progressive:
                for hours in time_ranges:
                    range_start_time = end_time - (hours * 3600)
                    simplified = await do_search(query, range_start_time, end_time, limit)
                    
                    if simplified.get("hit_count", 0) > 0:
                        if container_names:
                            simplified = filter_logs_by_container(simplified, container_names)
                        simplified["search_time_range_hours"] = hours
                        simplified["topic_id"] = topic_id
                        simplified["query"] = query
                        simplified["container_name"] = container_names if container_names else ""
                        simplified["time_range"] = {"start": range_start_time, "end": end_time}
                        return json.dumps(simplified, ensure_ascii=False, indent=2)
                
                if and_keywords:
                    primary_keyword = and_keywords[0]
                    for hours in time_ranges:
                        range_start_time = end_time - (hours * 3600)
                        simplified = await do_search(primary_keyword, range_start_time, end_time, limit)
                        
                        if simplified.get("hit_count", 0) > 0:
                            simplified = filter_logs_by_keywords(simplified, and_keywords)
                            simplified["search_time_range_hours"] = hours
                            simplified["topic_id"] = topic_id
                            simplified["query"] = query
                            simplified["time_range"] = {"start": range_start_time, "end": end_time}
                            simplified["fallback_search"] = f"AND query returned 0 across all time ranges, fell back to single keyword: {primary_keyword}"
                            if container_names:
                                simplified = filter_logs_by_container(simplified, container_names)
                            simplified["container_name"] = container_names if container_names else ""
                            if simplified.get("hit_count", 0) > 0:
                                return json.dumps(simplified, ensure_ascii=False, indent=2)
                
                result = await asyncio.to_thread(
                    client.search_logs,
                    topic_id=topic_id,
                    query=query,
                    start_time=end_time - (168 * 3600),
                    end_time=end_time,
                    limit=limit,
                    query_language=query_language
                )
                simplified = simplify_log_result(result)
                if container_names:
                    simplified = filter_logs_by_container(simplified, container_names)
                simplified["search_time_range_hours"] = 168
                simplified["progressive_search_completed"] = True
                simplified["topic_id"] = topic_id
                simplified["query"] = query
                simplified["container_name"] = container_names if container_names else ""
                simplified["time_range"] = {"start": end_time - (168 * 3600), "end": end_time}
                simplified["fallback_search"] = f"AND query returned 0, fell back to single keyword: {primary_keyword}"
                return json.dumps(simplified, ensure_ascii=False, indent=2)
            else:
                # Non-progressive: default to 24 hours
                default_start_time = end_time - (24 * 3600)
                simplified = await do_search(query, default_start_time, end_time, limit)
                
                if simplified.get("hit_count", 0) == 0 and and_keywords:
                    primary_keyword = and_keywords[0]
                    simplified = await do_search(primary_keyword, default_start_time, end_time, limit)
                    if simplified.get("hit_count", 0) > 0:
                        simplified = filter_logs_by_keywords(simplified, and_keywords)
                        simplified["fallback_search"] = f"AND query returned 0, fell back to single keyword: {primary_keyword}"
                
                if container_names:
                    simplified = filter_logs_by_container(simplified, container_names)
                simplified["topic_id"] = topic_id
                simplified["query"] = query
                simplified["container_name"] = container_names if container_names else ""
                simplified["time_range"] = {"start": default_start_time, "end": end_time}
                return json.dumps(simplified, ensure_ascii=False, indent=2)
                
        except ValueError as e:
            raise e
        except Exception as e:
            raise RuntimeError(f"Error: {str(e)}")
    
    def _parse_result(self, result: Dict[str, Any]) -> Dict[str, Any]:
        """Parse the search result dict"""
        if isinstance(result, str):
            try:
                return json.loads(result)
            except:
                return {"error": "Invalid result format"}
        return result

    async def get_topic_info(self, arguments: Dict[str, Any]) -> str:
        """Get topic information. Parameters: topic_id (optional)"""
        try:
            topic_id = self._get_topic_id(arguments)

            client = self._get_client()
            result = await asyncio.to_thread(client.get_topic, topic_id=topic_id)
            return json.dumps(result, ensure_ascii=False, indent=2)
        except ValueError as e:
            raise e
        except Exception as e:
            raise RuntimeError(f"Error: {str(e)}")

    async def get_log_stream(self, arguments: Dict[str, Any]) -> str:
        """Get log stream. Parameters: topic_id (optional), shard_id (optional)"""
        try:
            topic_id = self._get_topic_id(arguments)
            shard_id = arguments.get("shard_id")

            client = self._get_client()
            result = await asyncio.to_thread(client.get_log_stream, topic_id=topic_id, shard_id=shard_id)
            return json.dumps(result, ensure_ascii=False, indent=2)
        except ValueError as e:
            raise e
        except Exception as e:
            raise RuntimeError(f"Error: {str(e)}")


if __name__ == "__main__":
    mcp_server = VolcTLSMCP()
    mcp_server.add_tool(mcp_server.list_topics, name="list_topics", description="List all log topics. Parameters: project_id (optional, project ID)")
    mcp_server.add_tool(mcp_server.search_logs, name="search_logs", description="Search logs by query. Parameters: topic_id (optional, uses VOLC_TOPIC_ID/VOLCENGINE_TOPIC_ID by default), query (required, Lucene query), start_time (optional, start timestamp in seconds), end_time (optional, end timestamp in seconds, default: now), limit (default 100), query_language (default Lucene), progressive (optional, boolean, default: true for progressive search from 2h to 48h)")
    mcp_server.add_tool(mcp_server.get_topic_info, name="get_topic_info", description="Get topic information. Parameters: topic_id (optional, uses VOLC_TOPIC_ID/VOLCENGINE_TOPIC_ID by default)")
    mcp_server.add_tool(mcp_server.get_log_stream, name="get_log_stream", description="Get log stream. Parameters: topic_id (optional, uses VOLC_TOPIC_ID/VOLCENGINE_TOPIC_ID by default), shard_id (optional)")
    mcp_server.run()
