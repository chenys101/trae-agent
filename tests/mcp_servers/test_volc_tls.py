import asyncio
import json
import unittest
from unittest.mock import MagicMock, patch

from mcp_servers.volc_tls.main import TLSClient, VolcTLSMCP


class TestTLSClient(unittest.TestCase):
    def test_convert_to_dict(self):
        class MockObj:
            def __init__(self):
                self.name = "test"
                self.value = 123
                self._private = "secret"
        
        result = TLSClient._convert_to_dict(MockObj())
        self.assertEqual(result["name"], "test")
        self.assertEqual(result["value"], 123)
        self.assertNotIn("_private", result)

    def test_convert_to_dict_list(self):
        result = TLSClient._convert_to_dict([1, "test", None])
        self.assertEqual(result, [1, "test", None])

    def test_convert_to_dict_primitive(self):
        self.assertEqual(TLSClient._convert_to_dict(42), 42)
        self.assertEqual(TLSClient._convert_to_dict("hello"), "hello")
        self.assertEqual(TLSClient._convert_to_dict(None), None)


class TestVolcTLSMCP(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.mcp = VolcTLSMCP()

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    @patch("mcp_servers.volc_tls.main.TLSClient")
    async def test_list_topics_success(self, mock_tls_client):
        mock_instance = MagicMock()
        mock_tls_client.return_value = mock_instance
        mock_instance.list_topics.return_value = {"topics": [{"topic_id": "topic-1", "topic_name": "Test Topic"}]}

        result = await self.mcp.list_topics({})
        
        self.assertIsInstance(result, str)
        content = json.loads(result)
        self.assertIn("topics", content)
        self.assertEqual(len(content["topics"]), 1)

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    @patch("mcp_servers.volc_tls.main.TLSClient")
    async def test_list_topics_with_project_id(self, mock_tls_client):
        mock_instance = MagicMock()
        mock_tls_client.return_value = mock_instance
        mock_instance.list_topics.return_value = {"topics": []}

        result = await self.mcp.list_topics({"project_id": "project-1"})
        
        self.assertIsInstance(result, str)
        mock_instance.list_topics.assert_called_with("project-1")

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    @patch("mcp_servers.volc_tls.main.TLSClient")
    async def test_search_logs_success(self, mock_tls_client):
        mock_instance = MagicMock()
        mock_tls_client.return_value = mock_instance
        mock_instance.search_logs.return_value = {"logs": [], "total": 0}

        arguments = {
            "topic_id": "topic-1",
            "query": "level:ERROR",
            "start_time": 1704067200,
            "end_time": 1704153600,
            "limit": 100
        }
        
        result = await self.mcp.search_logs(arguments)
        
        self.assertIsInstance(result, str)
        content = json.loads(result)
        self.assertIn("logs", content)

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    async def test_search_logs_missing_parameters(self):
        arguments = {"topic_id": "topic-1", "query": "error"}
        with self.assertRaises(ValueError) as context:
            await self.mcp.search_logs(arguments)
        
        self.assertIn("Missing required parameters", str(context.exception))

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    @patch("mcp_servers.volc_tls.main.TLSClient")
    async def test_search_logs_exception(self, mock_tls_client):
        mock_instance = MagicMock()
        mock_tls_client.return_value = mock_instance
        mock_instance.search_logs.side_effect = RuntimeError("API Error")

        arguments = {
            "topic_id": "topic-1",
            "query": "error",
            "start_time": 1704067200,
            "end_time": 1704153600
        }
        
        with self.assertRaises(RuntimeError) as context:
            await self.mcp.search_logs(arguments)
        
        self.assertIn("Error: API Error", str(context.exception))

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    @patch("mcp_servers.volc_tls.main.TLSClient")
    async def test_get_topic_info_success(self, mock_tls_client):
        mock_instance = MagicMock()
        mock_tls_client.return_value = mock_instance
        mock_instance.get_topic.return_value = {"topic_id": "topic-1", "topic_name": "Test"}

        result = await self.mcp.get_topic_info({"topic_id": "topic-1"})
        
        self.assertIsInstance(result, str)
        content = json.loads(result)
        self.assertEqual(content["topic_id"], "topic-1")

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    async def test_get_topic_info_missing_topic_id(self):
        with self.assertRaises(ValueError) as context:
            await self.mcp.get_topic_info({})
        
        self.assertIn("Missing required parameter: topic_id", str(context.exception))

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    @patch("mcp_servers.volc_tls.main.TLSClient")
    async def test_get_log_stream_success(self, mock_tls_client):
        mock_instance = MagicMock()
        mock_tls_client.return_value = mock_instance
        mock_instance.get_log_stream.return_value = {"streams": []}

        result = await self.mcp.get_log_stream({"topic_id": "topic-1"})
        
        self.assertIsInstance(result, str)

    @patch.dict("os.environ", {"VOLC_ACCESS_KEY": "test_key", "VOLC_SECRET_KEY": "test_secret", "VOLC_REGION": "cn-beijing"})
    async def test_get_log_stream_missing_topic_id(self):
        with self.assertRaises(ValueError) as context:
            await self.mcp.get_log_stream({})
        
        self.assertIn("Missing required parameter: topic_id", str(context.exception))

    @patch.dict("os.environ", {}, clear=True)
    @patch("mcp_servers.volc_tls.main.TLSClient")
    async def test_missing_credentials(self, mock_tls_client):
        with self.assertRaises(RuntimeError) as context:
            await self.mcp.list_topics({})
        
        self.assertIn("VOLCENGINE_ACCESS_KEY_ID", str(context.exception))


if __name__ == "__main__":
    unittest.main()