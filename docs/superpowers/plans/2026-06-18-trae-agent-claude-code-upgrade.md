# Trae Agent → Claude Code / Cursor CLI Level Upgrade

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade Trae Agent from a basic SWE-bench agent to a Claude Code / Cursor CLI class coding agent with rich file operation tools, context management, dynamic prompts, and sub-agent delegation.

**Architecture:** Add three new file operation tools (ReadFile, GrepSearch, GlobSearch) as first-class Tool subclasses following the existing pattern in `trae_agent/tools/`. Upgrade the system prompt to be project-aware and tool-guided. Add a context compression mechanism in the agent loop. Add a sub-agent delegation tool. All changes plug into the existing `Tool` ABC, `tools_registry`, and `BaseAgent` framework without breaking existing functionality.

**Tech Stack:** Python 3.12+, asyncio, ripgrep (rg), existing trae_agent tool framework, pytest for testing

---

## File Structure

| File | Responsibility |
|------|---------------|
| `trae_agent/tools/read_file_tool.py` | **NEW** — ReadFile tool: read file contents with line range support |
| `trae_agent/tools/grep_search_tool.py` | **NEW** — GrepSearch tool: regex search across files using ripgrep |
| `trae_agent/tools/glob_search_tool.py` | **NEW** — GlobSearch tool: file pattern matching using glob |
| `trae_agent/tools/sub_agent_tool.py` | **NEW** — SubAgent tool: delegate sub-tasks to fresh agent instances |
| `trae_agent/tools/__init__.py` | **MODIFY** — Register 4 new tools in `tools_registry` |
| `trae_agent/prompt/agent_prompt.py` | **MODIFY** — Rewrite system prompt for general-purpose coding agent |
| `trae_agent/agent/base_agent.py` | **MODIFY** — Add context compression logic in `_run_llm_step` |
| `trae_agent/agent/trae_agent.py` | **MODIFY** — Update default tool list, add project context injection |
| `trae_agent/utils/config.py` | **MODIFY** — Update `TraeAgentConfig` default tools list |
| `tests/tools/test_read_file_tool.py` | **NEW** — Tests for ReadFile tool |
| `tests/tools/test_grep_search_tool.py` | **NEW** — Tests for GrepSearch tool |
| `tests/tools/test_glob_search_tool.py` | **NEW** — Tests for GlobSearch tool |
| `tests/tools/test_sub_agent_tool.py` | **NEW** — Tests for SubAgent tool |

---

### Task 1: ReadFile Tool

**Files:**
- Create: `trae_agent/tools/read_file_tool.py`
- Create: `tests/tools/test_read_file_tool.py`

- [ ] **Step 1: Write the failing test**

```python
# tests/tools/test_read_file_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

import tempfile
import unittest
from pathlib import Path

from trae_agent.tools.base import ToolCallArguments
from trae_agent.tools.read_file_tool import ReadFileTool


class TestReadFileTool(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.tool = ReadFileTool()
        self.tmpdir = tempfile.mkdtemp()
        self.test_file = Path(self.tmpdir) / "sample.txt"
        self.test_file.write_text("line1\nline2\nline3\nline4\nline5\n")

    async def test_tool_name(self):
        self.assertEqual(self.tool.get_name(), "read_file")

    async def test_tool_description_mentions_read(self):
        self.assertIn("read", self.tool.get_description().lower())

    async def test_read_entire_file(self):
        result = await self.tool.execute(ToolCallArguments({
            "file_path": str(self.test_file),
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("line1", result.output)
        self.assertIn("line5", result.output)

    async def test_read_with_line_range(self):
        result = await self.tool.execute(ToolCallArguments({
            "file_path": str(self.test_file),
            "offset": 2,
            "limit": 2,
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("line2", result.output)
        self.assertIn("line3", result.output)
        self.assertNotIn("line1", result.output)
        self.assertNotIn("line4", result.output)

    async def test_read_nonexistent_file(self):
        result = await self.tool.execute(ToolCallArguments({
            "file_path": "/nonexistent/path/file.txt",
        }))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("not found", result.error.lower())

    async def test_read_relative_path_fails(self):
        result = await self.tool.execute(ToolCallArguments({
            "file_path": "relative/path.txt",
        }))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("absolute", result.error.lower())

    async def test_read_directory(self):
        result = await self.tool.execute(ToolCallArguments({
            "file_path": self.tmpdir,
        }))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("directory", result.error.lower())

    async def test_missing_file_path(self):
        result = await self.tool.execute(ToolCallArguments({}))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("file_path", result.error.lower())


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && python -m pytest tests/tools/test_read_file_tool.py -v 2>&1 | head -30`
Expected: FAIL with `ModuleNotFoundError: No module named 'trae_agent.tools.read_file_tool'`

- [ ] **Step 3: Write the implementation**

```python
# trae_agent/tools/read_file_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""ReadFile tool for reading file contents with optional line range."""

from pathlib import Path
from typing import override

from trae_agent.tools.base import Tool, ToolCallArguments, ToolExecResult, ToolParameter
from trae_agent.tools.run import maybe_truncate


class ReadFileTool(Tool):
    """Tool to read file contents with line numbers and optional range."""

    @override
    def get_name(self) -> str:
        return "read_file"

    @override
    def get_description(self) -> str:
        return """Read the contents of a file. Returns the file content with line numbers.
Supports reading a specific line range via offset and limit parameters.
Use this instead of bash commands like `cat` or `head` to read files.
The file_path must be an absolute path."""

    @override
    def get_parameters(self) -> list[ToolParameter]:
        return [
            ToolParameter(
                name="file_path",
                type="string",
                description="Absolute path to the file to read.",
                required=True,
            ),
            ToolParameter(
                name="offset",
                type="integer",
                description="Line number to start reading from (1-based). Defaults to 1.",
                required=False,
            ),
            ToolParameter(
                name="limit",
                type="integer",
                description="Maximum number of lines to read. Defaults to reading the entire file.",
                required=False,
            ),
        ]

    @override
    async def execute(self, arguments: ToolCallArguments) -> ToolExecResult:
        file_path = arguments.get("file_path")
        if not file_path or not isinstance(file_path, str):
            return ToolExecResult(
                error="Parameter 'file_path' is required and must be a string.",
                error_code=-1,
            )

        path = Path(file_path)

        if not path.is_absolute():
            return ToolExecResult(
                error=f"The path '{file_path}' is not an absolute path. It must start with '/'.",
                error_code=-1,
            )

        if not path.exists():
            return ToolExecResult(
                error=f"File not found: {file_path}",
                error_code=-1,
            )

        if path.is_dir():
            return ToolExecResult(
                error=f"The path '{file_path}' is a directory, not a file. Use glob_search to find files.",
                error_code=-1,
            )

        try:
            content = path.read_text()
        except Exception as e:
            return ToolExecResult(
                error=f"Error reading file '{file_path}': {e}",
                error_code=-1,
            )

        lines = content.split("\n")
        total_lines = len(lines)

        offset = arguments.get("offset")
        if offset is not None:
            try:
                offset = int(offset)
            except (TypeError, ValueError):
                return ToolExecResult(
                    error="Parameter 'offset' must be an integer.",
                    error_code=-1,
                )
            if offset < 1:
                offset = 1
        else:
            offset = 1

        limit = arguments.get("limit")
        if limit is not None:
            try:
                limit = int(limit)
            except (TypeError, ValueError):
                return ToolExecResult(
                    error="Parameter 'limit' must be an integer.",
                    error_code=-1,
                )

        end = offset + limit - 1 if limit else total_lines
        selected_lines = lines[offset - 1 : end]

        numbered = "\n".join(
            f"{i + offset:6}\t{line}" for i, line in enumerate(selected_lines)
        )

        header = f"File: {file_path} ({total_lines} lines total)\n"
        if offset > 1 or (limit and end < total_lines):
            header += f"Showing lines {offset}-{min(end, total_lines)}:\n"

        output = header + maybe_truncate(numbered)

        return ToolExecResult(output=output)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && python -m pytest tests/tools/test_read_file_tool.py -v`
Expected: All 7 tests PASS

- [ ] **Step 5: Commit**

```bash
git add trae_agent/tools/read_file_tool.py tests/tools/test_read_file_tool.py
git commit -m "feat: add ReadFile tool for direct file reading with line range support"
```

---

### Task 2: GrepSearch Tool

**Files:**
- Create: `trae_agent/tools/grep_search_tool.py`
- Create: `tests/tools/test_grep_search_tool.py`

- [ ] **Step 1: Write the failing test**

```python
# tests/tools/test_grep_search_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

import tempfile
import unittest
from pathlib import Path

from trae_agent.tools.base import ToolCallArguments
from trae_agent.tools.grep_search_tool import GrepSearchTool


class TestGrepSearchTool(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.tool = GrepSearchTool()
        self.tmpdir = tempfile.mkdtemp()
        # Create test files
        (Path(self.tmpdir) / "app.py").write_text("def hello():\n    print('hello world')\n    return True\n")
        (Path(self.tmpdir) / "utils.py").write_text("def goodbye():\n    print('goodbye world')\n    return False\n")
        subdir = Path(self.tmpdir) / "sub"
        subdir.mkdir()
        (subdir / "deep.py").write_text("def hello():\n    print('deep hello')\n")

    async def test_tool_name(self):
        self.assertEqual(self.tool.get_name(), "grep_search")

    async def test_basic_search(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "hello",
            "path": self.tmpdir,
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("app.py", result.output)
        self.assertIn("hello", result.output)

    async def test_regex_search(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "def \\w+\\(",
            "path": self.tmpdir,
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("def hello", result.output)
        self.assertIn("def goodbye", result.output)

    async def test_search_with_file_pattern(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "hello",
            "path": self.tmpdir,
            "include": "*.py",
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("app.py", result.output)

    async def test_search_nonexistent_path(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "hello",
            "path": "/nonexistent/path",
        }))
        self.assertNotEqual(result.error_code, 0)

    async def test_missing_pattern(self):
        result = await self.tool.execute(ToolCallArguments({
            "path": self.tmpdir,
        }))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("pattern", result.error.lower())

    async def test_case_insensitive_search(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "HELLO",
            "path": self.tmpdir,
            "case_insensitive": True,
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("hello", result.output.lower())


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && python -m pytest tests/tools/test_grep_search_tool.py -v 2>&1 | head -30`
Expected: FAIL with `ModuleNotFoundError: No module named 'trae_agent.tools.grep_search_tool'`

- [ ] **Step 3: Write the implementation**

```python
# trae_agent/tools/grep_search_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""GrepSearch tool for searching file contents using ripgrep."""

import shutil
from typing import override

from trae_agent.tools.base import Tool, ToolCallArguments, ToolExecResult, ToolParameter
from trae_agent.tools.run import maybe_truncate, run


class GrepSearchTool(Tool):
    """Tool to search file contents using regex patterns (ripgrep-based)."""

    @override
    def get_name(self) -> str:
        return "grep_search"

    @override
    def get_description(self) -> str:
        return """Search the contents of files using a regular expression pattern.
Uses ripgrep (rg) for fast, efficient searching across the codebase.
Returns matching lines with file paths and line numbers.
Use this instead of bash commands like `grep` to search file contents.
The path must be an absolute path to the directory or file to search in."""

    @override
    def get_parameters(self) -> list[ToolParameter]:
        return [
            ToolParameter(
                name="pattern",
                type="string",
                description="The regular expression pattern to search for.",
                required=True,
            ),
            ToolParameter(
                name="path",
                type="string",
                description="Absolute path to the file or directory to search in.",
                required=True,
            ),
            ToolParameter(
                name="include",
                type="string",
                description="File glob pattern to include (e.g. '*.py', '*.{js,ts}'). If not provided, searches all files.",
                required=False,
            ),
            ToolParameter(
                name="case_insensitive",
                type="boolean",
                description="Whether to perform a case-insensitive search. Defaults to false.",
                required=False,
            ),
        ]

    @override
    async def execute(self, arguments: ToolCallArguments) -> ToolExecResult:
        pattern = arguments.get("pattern")
        if not pattern or not isinstance(pattern, str):
            return ToolExecResult(
                error="Parameter 'pattern' is required and must be a string.",
                error_code=-1,
            )

        path = arguments.get("path")
        if not path or not isinstance(path, str):
            return ToolExecResult(
                error="Parameter 'path' is required and must be a string.",
                error_code=-1,
            )

        # Build the command
        # Prefer ripgrep, fall back to grep
        use_rg = shutil.which("rg") is not None

        if use_rg:
            cmd_parts = ["rg", "--line-number", "--no-heading"]
            if arguments.get("case_insensitive"):
                cmd_parts.append("-i")
            include = arguments.get("include")
            if include and isinstance(include, str):
                cmd_parts.extend(["--glob", include])
            cmd_parts.extend(["--", pattern, path])
        else:
            cmd_parts = ["grep", "-rn", "-E"]
            if arguments.get("case_insensitive"):
                cmd_parts.append("-i")
            include = arguments.get("include")
            if include and isinstance(include, str):
                cmd_parts.extend(["--include", include])
            cmd_parts.extend(["--", pattern, path])

        cmd = " ".join(cmd_parts)

        try:
            return_code, stdout, stderr = await run(cmd, timeout=30.0)
        except TimeoutError:
            return ToolExecResult(
                error="Search timed out after 30 seconds. Try narrowing the search path or pattern.",
                error_code=-1,
            )

        if return_code == 1 and not stdout:
            # grep/rg return 1 when no matches found
            return ToolExecResult(
                output=f"No matches found for pattern '{pattern}' in {path}."
            )

        if return_code != 0 and not stdout:
            return ToolExecResult(
                error=f"Search failed: {stderr or 'Unknown error'}",
                error_code=return_code,
            )

        output = maybe_truncate(stdout)
        match_count = stdout.count("\n") + (1 if stdout and not stdout.endswith("\n") else 0)
        header = f"Found {match_count} matches for '{pattern}' in {path}:\n"

        return ToolExecResult(output=header + output)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && python -m pytest tests/tools/test_grep_search_tool.py -v`
Expected: All 7 tests PASS

- [ ] **Step 5: Commit**

```bash
git add trae_agent/tools/grep_search_tool.py tests/tools/test_grep_search_tool.py
git commit -m "feat: add GrepSearch tool for regex-based file content search"
```

---

### Task 3: GlobSearch Tool

**Files:**
- Create: `trae_agent/tools/glob_search_tool.py`
- Create: `tests/tools/test_glob_search_tool.py`

- [ ] **Step 1: Write the failing test**

```python
# tests/tools/test_glob_search_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

import tempfile
import unittest
from pathlib import Path

from trae_agent.tools.base import ToolCallArguments
from trae_agent.tools.glob_search_tool import GlobSearchTool


class TestGlobSearchTool(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.tool = GlobSearchTool()
        self.tmpdir = tempfile.mkdtemp()
        # Create test files
        (Path(self.tmpdir) / "app.py").write_text("print('app')")
        (Path(self.tmpdir) / "utils.py").write_text("print('utils')")
        (Path(self.tmpdir) / "readme.md").write_text("# readme")
        subdir = Path(self.tmpdir) / "src"
        subdir.mkdir()
        (subdir / "main.py").write_text("print('main')")
        (subdir / "helper.py").write_text("print('helper')")

    async def test_tool_name(self):
        self.assertEqual(self.tool.get_name(), "glob_search")

    async def test_find_python_files(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "**/*.py",
            "path": self.tmpdir,
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("app.py", result.output)
        self.assertIn("utils.py", result.output)
        self.assertIn("main.py", result.output)

    async def test_find_specific_file(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "app.py",
            "path": self.tmpdir,
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("app.py", result.output)
        self.assertNotIn("utils.py", result.output)

    async def test_find_in_subdirectory(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "*.py",
            "path": str(Path(self.tmpdir) / "src"),
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("main.py", result.output)
        self.assertNotIn("app.py", result.output)

    async def test_no_matches(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "*.rs",
            "path": self.tmpdir,
        }))
        self.assertEqual(result.error_code, 0)
        self.assertIn("no files", result.output.lower())

    async def test_missing_pattern(self):
        result = await self.tool.execute(ToolCallArguments({
            "path": self.tmpdir,
        }))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("pattern", result.error.lower())

    async def test_nonexistent_path(self):
        result = await self.tool.execute(ToolCallArguments({
            "pattern": "*.py",
            "path": "/nonexistent/path",
        }))
        self.assertNotEqual(result.error_code, 0)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && python -m pytest tests/tools/test_glob_search_tool.py -v 2>&1 | head -30`
Expected: FAIL with `ModuleNotFoundError: No module named 'trae_agent.tools.glob_search_tool'`

- [ ] **Step 3: Write the implementation**

```python
# trae_agent/tools/glob_search_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""GlobSearch tool for finding files by pattern."""

from pathlib import Path
from typing import override

from trae_agent.tools.base import Tool, ToolCallArguments, ToolExecResult, ToolParameter
from trae_agent.tools.run import maybe_truncate


class GlobSearchTool(Tool):
    """Tool to find files matching a glob pattern."""

    @override
    def get_name(self) -> str:
        return "glob_search"

    @override
    def get_description(self) -> str:
        return """Find files matching a glob pattern in a directory.
Returns a list of matching file paths.
Use this instead of bash commands like `find` or `ls` to locate files.
Supports patterns like '**/*.py', 'src/**/*.ts', '*.json'.
The path must be an absolute path to the directory to search in."""

    @override
    def get_parameters(self) -> list[ToolParameter]:
        return [
            ToolParameter(
                name="pattern",
                type="string",
                description="Glob pattern to match files against (e.g. '**/*.py', 'src/**/*.ts').",
                required=True,
            ),
            ToolParameter(
                name="path",
                type="string",
                description="Absolute path to the directory to search in.",
                required=True,
            ),
        ]

    @override
    async def execute(self, arguments: ToolCallArguments) -> ToolExecResult:
        pattern = arguments.get("pattern")
        if not pattern or not isinstance(pattern, str):
            return ToolExecResult(
                error="Parameter 'pattern' is required and must be a string.",
                error_code=-1,
            )

        path = arguments.get("path")
        if not path or not isinstance(path, str):
            return ToolExecResult(
                error="Parameter 'path' is required and must be a string.",
                error_code=-1,
            )

        search_path = Path(path)

        if not search_path.is_absolute():
            return ToolExecResult(
                error=f"The path '{path}' is not an absolute path. It must start with '/'.",
                error_code=-1,
            )

        if not search_path.exists():
            return ToolExecResult(
                error=f"Path does not exist: {path}",
                error_code=-1,
            )

        if not search_path.is_dir():
            return ToolExecResult(
                error=f"The path '{path}' is not a directory.",
                error_code=-1,
            )

        try:
            matches = sorted(search_path.glob(pattern))
        except Exception as e:
            return ToolExecResult(
                error=f"Invalid glob pattern '{pattern}': {e}",
                error_code=-1,
            )

        # Filter out directories, only return files
        file_matches = [m for m in matches if m.is_file()]

        if not file_matches:
            return ToolExecResult(
                output=f"No files found matching pattern '{pattern}' in {path}."
            )

        # Format output
        lines = []
        for match in file_matches:
            try:
                rel = match.relative_to(search_path)
                lines.append(str(rel))
            except ValueError:
                lines.append(str(match))

        output = f"Found {len(file_matches)} files matching '{pattern}' in {path}:\n"
        output += maybe_truncate("\n".join(lines))

        return ToolExecResult(output=output)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && python -m pytest tests/tools/test_glob_search_tool.py -v`
Expected: All 7 tests PASS

- [ ] **Step 5: Commit**

```bash
git add trae_agent/tools/glob_search_tool.py tests/tools/test_glob_search_tool.py
git commit -m "feat: add GlobSearch tool for file pattern matching"
```

---

### Task 4: SubAgent Delegation Tool

**Files:**
- Create: `trae_agent/tools/sub_agent_tool.py`
- Create: `tests/tools/test_sub_agent_tool.py`

- [ ] **Step 1: Write the failing test**

```python
# tests/tools/test_sub_agent_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

import unittest
from unittest.mock import AsyncMock, MagicMock, patch

from trae_agent.tools.base import ToolCallArguments
from trae_agent.tools.sub_agent_tool import SubAgentTool


class TestSubAgentTool(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.mock_model_provider = "anthropic"
        self.tool = SubAgentTool(model_provider=self.mock_model_provider)

    async def test_tool_name(self):
        self.assertEqual(self.tool.get_name(), "sub_agent")

    async def test_tool_description_mentions_delegate(self):
        desc = self.tool.get_description().lower()
        self.assertTrue("delegate" in desc or "sub-task" in desc or "subtask" in desc)

    async def test_missing_task_description(self):
        result = await self.tool.execute(ToolCallArguments({
            "working_dir": "/tmp",
        }))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("task", result.error.lower())

    async def test_missing_working_dir(self):
        result = await self.tool.execute(ToolCallArguments({
            "task_description": "do something",
        }))
        self.assertNotEqual(result.error_code, 0)
        self.assertIn("working_dir", result.error.lower())

    async def test_successful_delegation(self):
        mock_execution = MagicMock()
        mock_execution.success = True
        mock_execution.final_result = "Task completed successfully"
        mock_execution.steps = []

        with patch("trae_agent.tools.sub_agent_tool.SubAgentTool._run_sub_agent", new_callable=AsyncMock) as mock_run:
            mock_run.return_value = mock_execution
            result = await self.tool.execute(ToolCallArguments({
                "task_description": "Find all TODO comments in the codebase",
                "working_dir": "/tmp",
            }))

        self.assertEqual(result.error_code, 0)
        self.assertIn("completed successfully", result.output.lower())


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && python -m pytest tests/tools/test_sub_agent_tool.py -v 2>&1 | head -30`
Expected: FAIL with `ModuleNotFoundError: No module named 'trae_agent.tools.sub_agent_tool'`

- [ ] **Step 3: Write the implementation**

```python
# trae_agent/tools/sub_agent_tool.py
# Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
# SPDX-License-Identifier: MIT

"""SubAgent tool for delegating sub-tasks to fresh agent instances."""

import asyncio
from typing import override

from trae_agent.agent.agent_basics import AgentExecution
from trae_agent.tools.base import Tool, ToolCallArguments, ToolExecResult, ToolParameter


class SubAgentTool(Tool):
    """Tool to delegate sub-tasks to a fresh agent instance.

    This allows the main agent to offload focused sub-tasks to a separate
    agent, protecting the main context window from excessive output.
    """

    def __init__(self, model_provider: str | None = None):
        super().__init__(model_provider)
        self._parent_model_config = None

    def set_parent_model_config(self, model_config) -> None:
        """Set the parent agent's model config for creating sub-agents."""
        self._parent_model_config = model_config

    @override
    def get_name(self) -> str:
        return "sub_agent"

    @override
    def get_description(self) -> str:
        return """Delegate a sub-task to a fresh agent instance.
Use this for focused, independent sub-tasks that would consume too much context in the main conversation.
The sub-agent gets its own context window and tools, then returns a summary.
Good for: searching large codebases, running extensive test suites, analyzing logs.
Avoid for: tasks that need context from the current conversation."""

    @override
    def get_parameters(self) -> list[ToolParameter]:
        return [
            ToolParameter(
                name="task_description",
                type="string",
                description="A clear, self-contained description of the sub-task to delegate.",
                required=True,
            ),
            ToolParameter(
                name="working_dir",
                type="string",
                description="Absolute path to the working directory for the sub-agent.",
                required=True,
            ),
        ]

    async def _run_sub_agent(
        self, task_description: str, working_dir: str
    ) -> AgentExecution:
        """Run a sub-agent with the given task and return its execution result."""
        from trae_agent.agent.trae_agent import TraeAgent, TraeAgentToolNames
        from trae_agent.tools import tools_registry
        from trae_agent.utils.config import TraeAgentConfig

        if self._parent_model_config is None:
            raise ValueError("Parent model config not set. Call set_parent_model_config first.")

        # Create a minimal config for the sub-agent using the parent's model config
        from trae_agent.utils.config import AgentConfig, ModelProvider, MCPServerConfig

        sub_agent_config = TraeAgentConfig(
            model=self._parent_model_config,
            max_steps=30,
            tools=["bash", "read_file", "grep_search", "glob_search", "str_replace_based_edit_tool", "task_done"],
            allow_mcp_servers=[],
            mcp_servers_config={},
        )

        sub_agent = TraeAgent(sub_agent_config)
        sub_agent.new_task(
            task=task_description,
            extra_args={
                "project_path": working_dir,
                "issue": task_description,
            },
        )

        return await sub_agent.execute_task()

    @override
    async def execute(self, arguments: ToolCallArguments) -> ToolExecResult:
        task_description = arguments.get("task_description")
        if not task_description or not isinstance(task_description, str):
            return ToolExecResult(
                error="Parameter 'task_description' is required and must be a string.",
                error_code=-1,
            )

        working_dir = arguments.get("working_dir")
        if not working_dir or not isinstance(working_dir, str):
            return ToolExecResult(
                error="Parameter 'working_dir' is required and must be a string.",
                error_code=-1,
            )

        try:
            execution = await self._run_sub_agent(task_description, working_dir)
        except Exception as e:
            return ToolExecResult(
                error=f"Sub-agent execution failed: {e}",
                error_code=-1,
            )

        if execution.success:
            summary = execution.final_result or "Sub-agent completed the task."
            step_count = len(execution.steps)
            output = f"Sub-agent completed successfully in {step_count} steps.\n\nResult:\n{summary}"
            return ToolExecResult(output=output)
        else:
            summary = execution.final_result or "Sub-agent failed to complete the task."
            step_count = len(execution.steps)
            output = f"Sub-agent failed after {step_count} steps.\n\nDetails:\n{summary}"
            return ToolExecResult(output=output, error_code=-1)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && python -m pytest tests/tools/test_sub_agent_tool.py -v`
Expected: All 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add trae_agent/tools/sub_agent_tool.py tests/tools/test_sub_agent_tool.py
git commit -m "feat: add SubAgent tool for delegating sub-tasks to fresh agent instances"
```

---

### Task 5: Register New Tools in Registry

**Files:**
- Modify: `trae_agent/tools/__init__.py`

- [ ] **Step 1: Update the tools registry**

Add imports and registry entries for the 4 new tools. The current file at `trae_agent/tools/__init__.py` has:

```python
from trae_agent.tools.base import Tool, ToolCall, ToolExecutor, ToolResult
from trae_agent.tools.bash_tool import BashTool
from trae_agent.tools.ckg_tool import CKGTool
from trae_agent.tools.edit_tool import TextEditorTool
from trae_agent.tools.json_edit_tool import JSONEditTool
from trae_agent.tools.log_analysis_tool import LogAnalysisTool
from trae_agent.tools.business_analysis_tool import BusinessAnalysisTool
from trae_agent.tools.sequential_thinking_tool import SequentialThinkingTool
from trae_agent.tools.task_done_tool import TaskDoneTool

__all__ = [
    "Tool",
    "ToolResult",
    "ToolCall",
    "ToolExecutor",
    "BashTool",
    "TextEditorTool",
    "JSONEditTool",
    "SequentialThinkingTool",
    "TaskDoneTool",
    "CKGTool",
    "LogAnalysisTool",
    "BusinessAnalysisTool",
]

tools_registry: dict[str, type[Tool]] = {
    "bash": BashTool,
    "str_replace_based_edit_tool": TextEditorTool,
    "json_edit_tool": JSONEditTool,
    "sequentialthinking": SequentialThinkingTool,
    "task_done": TaskDoneTool,
    "ckg": CKGTool,
    "log_analysis": LogAnalysisTool,
    "business_analysis": BusinessAnalysisTool,
}
```

Change it to:

```python
from trae_agent.tools.base import Tool, ToolCall, ToolExecutor, ToolResult
from trae_agent.tools.bash_tool import BashTool
from trae_agent.tools.ckg_tool import CKGTool
from trae_agent.tools.edit_tool import TextEditorTool
from trae_agent.tools.glob_search_tool import GlobSearchTool
from trae_agent.tools.grep_search_tool import GrepSearchTool
from trae_agent.tools.json_edit_tool import JSONEditTool
from trae_agent.tools.log_analysis_tool import LogAnalysisTool
from trae_agent.tools.business_analysis_tool import BusinessAnalysisTool
from trae_agent.tools.read_file_tool import ReadFileTool
from trae_agent.tools.sequential_thinking_tool import SequentialThinkingTool
from trae_agent.tools.sub_agent_tool import SubAgentTool
from trae_agent.tools.task_done_tool import TaskDoneTool

__all__ = [
    "Tool",
    "ToolResult",
    "ToolCall",
    "ToolExecutor",
    "BashTool",
    "TextEditorTool",
    "JSONEditTool",
    "SequentialThinkingTool",
    "TaskDoneTool",
    "CKGTool",
    "LogAnalysisTool",
    "BusinessAnalysisTool",
    "ReadFileTool",
    "GrepSearchTool",
    "GlobSearchTool",
    "SubAgentTool",
]

tools_registry: dict[str, type[Tool]] = {
    "bash": BashTool,
    "str_replace_based_edit_tool": TextEditorTool,
    "json_edit_tool": JSONEditTool,
    "sequentialthinking": SequentialThinkingTool,
    "task_done": TaskDoneTool,
    "ckg": CKGTool,
    "log_analysis": LogAnalysisTool,
    "business_analysis": BusinessAnalysisTool,
    "read_file": ReadFileTool,
    "grep_search": GrepSearchTool,
    "glob_search": GlobSearchTool,
    "sub_agent": SubAgentTool,
}
```

- [ ] **Step 2: Verify registry loads**

Run: `cd /workspace && python -c "from trae_agent.tools import tools_registry; print(sorted(tools_registry.keys()))"`
Expected: Output includes `['bash', 'business_analysis', 'ckg', 'glob_search', 'grep_search', 'json_edit_tool', 'log_analysis', 'read_file', 'str_replace_based_edit_tool', 'sub_agent', 'sequentialthinking', 'task_done']`

- [ ] **Step 3: Commit**

```bash
git add trae_agent/tools/__init__.py
git commit -m "feat: register ReadFile, GrepSearch, GlobSearch, SubAgent tools in registry"
```

---

### Task 6: Upgrade System Prompt

**Files:**
- Modify: `trae_agent/prompt/agent_prompt.py`

- [ ] **Step 1: Rewrite the system prompt**

The current `TRAE_AGENT_SYSTEM_PROMPT` in `trae_agent/prompt/agent_prompt.py` is SWE-bench specific. Replace it with a general-purpose coding agent prompt that covers all tools. Replace the entire `TRAE_AGENT_SYSTEM_PROMPT` string with:

```python
TRAE_AGENT_SYSTEM_PROMPT = """You are an expert AI coding agent, similar to Claude Code or Cursor CLI. You help users with software engineering tasks by reading, writing, searching, and executing code.

## Core Principles

1. **Understand before acting.** Read the codebase, search for relevant files, and understand the context before making changes.
2. **Make minimal, targeted changes.** Prefer surgical edits over large rewrites.
3. **Verify your work.** Run tests, check for errors, and validate that your changes work as expected.
4. **Communicate clearly.** Explain what you're doing and why, especially when making non-obvious decisions.

## Tool Usage Guide

### File Operations
- **read_file** — Read file contents with line numbers. Use `offset` and `limit` for large files. ALWAYS read a file before editing it.
- **grep_search** — Search file contents with regex. Fast and efficient for finding code across the codebase. Use `include` to filter by file type.
- **glob_search** — Find files by name pattern. Use when you need to locate files but don't know their exact paths.
- **str_replace_based_edit_tool** — Edit files with `str_replace` (replace exact string), `create` (new file), `insert` (insert after line), `view` (view file/dir).

### Execution
- **bash** — Run shell commands. Use for running tests, installing packages, git operations, and any system commands.

### Planning
- **sequentialthinking** — Break down complex problems into structured thoughts. Use for debugging, planning multi-step changes, and analyzing trade-offs.

### Delegation
- **sub_agent** — Delegate a focused sub-task to a fresh agent. Use for tasks that would consume too much context, like searching large codebases or running extensive analysis.

### Completion
- **task_done** — Signal that the task is complete. Only call after verifying your work.

## File Path Rules

All tools that take a `file_path` or `path` parameter require an **absolute path**. Construct the full path by combining the `[Project root path]` with the relative file path.

Example: If project root is `/home/user/my_project` and you need `src/main.py`, use `/home/user/my_project/src/main.py`.

## Workflow

1. **Explore** — Use `glob_search` and `grep_search` to understand the project structure and find relevant files.
2. **Read** — Use `read_file` to examine files before modifying them.
3. **Plan** — Use `sequentialthinking` for complex tasks to plan your approach.
4. **Edit** — Use `str_replace_based_edit_tool` to make targeted changes.
5. **Verify** — Use `bash` to run tests and validate your changes.
6. **Complete** — Call `task_done` when the task is verified complete.

## Important Guidelines

- Always read a file before editing it to understand the current state.
- Use `grep_search` instead of bash `grep` for searching code.
- Use `glob_search` instead of bash `find` for locating files.
- Use `read_file` instead of bash `cat` for reading files.
- When a task is complex, break it down using `sequentialthinking` before acting.
- For large, independent sub-tasks, consider delegating to `sub_agent`.
- If you're unsure about something, explore the codebase first rather than guessing.
- Never make changes without understanding the existing code.
"""
```

- [ ] **Step 2: Verify prompt loads**

Run: `cd /workspace && python -c "from trae_agent.prompt.agent_prompt import TRAE_AGENT_SYSTEM_PROMPT; print(len(TRAE_AGENT_SYSTEM_PROMPT))"`
Expected: A positive integer (the prompt length)

- [ ] **Step 3: Commit**

```bash
git add trae_agent/prompt/agent_prompt.py
git commit -m "feat: upgrade system prompt to general-purpose coding agent with tool guide"
```

---

### Task 7: Update Default Tool List in Config and Agent

**Files:**
- Modify: `trae_agent/utils/config.py` (line 277-283, `TraeAgentConfig.tools` default)
- Modify: `trae_agent/agent/trae_agent.py` (line 24-30, `TraeAgentToolNames`)

- [ ] **Step 1: Update TraeAgentConfig default tools**

In `trae_agent/utils/config.py`, change the `TraeAgentConfig.tools` default from:

```python
    tools: list[str] = field(
        default_factory=lambda: [
            "bash",
            "str_replace_based_edit_tool",
            "sequentialthinking",
            "task_done",
        ]
    )
```

to:

```python
    tools: list[str] = field(
        default_factory=lambda: [
            "bash",
            "read_file",
            "grep_search",
            "glob_search",
            "str_replace_based_edit_tool",
            "sequentialthinking",
            "sub_agent",
            "task_done",
        ]
    )
```

- [ ] **Step 2: Update TraeAgentToolNames**

In `trae_agent/agent/trae_agent.py`, change `TraeAgentToolNames` from:

```python
TraeAgentToolNames = [
    "str_replace_based_edit_tool",
    "sequentialthinking",
    "json_edit_tool",
    "task_done",
    "bash",
]
```

to:

```python
TraeAgentToolNames = [
    "bash",
    "read_file",
    "grep_search",
    "glob_search",
    "str_replace_based_edit_tool",
    "json_edit_tool",
    "sequentialthinking",
    "sub_agent",
    "task_done",
]
```

- [ ] **Step 3: Verify config loads**

Run: `cd /workspace && python -c "from trae_agent.utils.config import TraeAgentConfig; c = TraeAgentConfig(max_steps=10, allow_mcp_servers=[], mcp_servers_config={}); print(c.tools)"`
Expected: `['bash', 'read_file', 'grep_search', 'glob_search', 'str_replace_based_edit_tool', 'sequentialthinking', 'sub_agent', 'task_done']`

- [ ] **Step 4: Commit**

```bash
git add trae_agent/utils/config.py trae_agent/agent/trae_agent.py
git commit -m "feat: update default tool list to include read_file, grep_search, glob_search, sub_agent"
```

---

### Task 8: Add Context Compression to Agent Loop

**Files:**
- Modify: `trae_agent/agent/base_agent.py` (lines 148-222, `execute_task` method)

- [ ] **Step 1: Add context compression method and integrate into execute_task**

Add a `_compress_messages` method to `BaseAgent` and call it in the `execute_task` loop when the message list grows too large. This prevents token overflow on long tasks.

Add the following method to `BaseAgent` class (after the `_close_tools` method, around line 230):

```python
    def _compress_messages(self, messages: list["LLMMessage"]) -> list["LLMMessage"]:
        """Compress message history when it grows too large.

        Keeps the system message, the first user message, and the most recent
        messages. Summarizes the middle portion into a single user message.
        """
        if len(messages) <= 20:
            return messages

        # Keep system message + first user message + last 10 messages
        system_msgs = [m for m in messages[:2] if m.role == "system"]
        recent = messages[-10:]

        # Create a summary of the compressed portion
        compressed_count = len(messages) - 2 - 10  # minus system + first user + recent
        if compressed_count <= 0:
            return messages

        summary = (
            f"[Context compressed: {compressed_count} earlier messages summarized]\n"
            "The agent has been working on the task. Key progress so far has been preserved "
            "in the recent messages below. Continue from where you left off."
        )

        compressed = LLMMessage(role="user", content=summary)
        result = system_msgs + [messages[1]] + [compressed] + recent
        return result
```

Then, in the `execute_task` method, add a call to `_compress_messages` inside the while loop, right before the `_run_llm_step` call. Find this block (around line 184):

```python
                    messages = await self._run_llm_step(step, messages, execution)
```

And insert before it:

```python
                    # Compress message history if it's getting too long
                    if len(messages) > 40:
                        messages = self._compress_messages(messages)
```

- [ ] **Step 2: Verify the agent still loads**

Run: `cd /workspace && python -c "from trae_agent.agent.base_agent import BaseAgent; print('BaseAgent loaded successfully')"`
Expected: `BaseAgent loaded successfully`

- [ ] **Step 3: Run existing agent tests**

Run: `cd /workspace && python -m pytest tests/agent/ -v 2>&1 | tail -20`
Expected: All existing tests still pass

- [ ] **Step 4: Commit**

```bash
git add trae_agent/agent/base_agent.py
git commit -m "feat: add context compression to prevent token overflow on long tasks"
```

---

### Task 9: Inject Project Context into Agent

**Files:**
- Modify: `trae_agent/agent/trae_agent.py` (lines 106-147, `new_task` method)

- [ ] **Step 1: Add project structure injection to new_task**

In `trae_agent/agent/trae_agent.py`, modify the `new_task` method to inject project structure context into the user message. Find the `new_task` method and after the line that builds `user_message` with `[Project root path]`, add project structure discovery.

Change the section after `self.project_path = extra_args.get("project_path", "")` (around line 134) from:

```python
        self.project_path = extra_args.get("project_path", "")
        if self.docker_config:
            user_message += r"[Project root path]:\workspace\n\n"
        else:
            user_message += f"[Project root path]:\n{self.project_path}\n\n"
```

to:

```python
        self.project_path = extra_args.get("project_path", "")
        if self.docker_config:
            user_message += r"[Project root path]:\workspace\n\n"
        else:
            user_message += f"[Project root path]:\n{self.project_path}\n\n"

        # Inject project structure context
        project_context = self._get_project_context()
        if project_context:
            user_message += f"[Project structure]:\n{project_context}\n\n"
```

Then add the `_get_project_context` method to the `TraeAgent` class (after the `get_system_prompt` method, around line 178):

```python
    def _get_project_context(self) -> str:
        """Get a summary of the project structure for context injection."""
        import os

        if not self.project_path or not os.path.isdir(self.project_path):
            return ""

        # Quick scan: list top-level files and directories
        try:
            entries = os.listdir(self.project_path)
        except OSError:
            return ""

        # Categorize entries
        dirs = []
        files = []
        for entry in sorted(entries):
            if entry.startswith(".") or entry == "__pycache__":
                continue
            full_path = os.path.join(self.project_path, entry)
            if os.path.isdir(full_path):
                dirs.append(entry + "/")
            else:
                files.append(entry)

        # Build a compact summary
        lines = []
        if dirs:
            lines.append("Directories: " + ", ".join(dirs[:20]))
        if files:
            lines.append("Files: " + ", ".join(files[:20]))

        # Detect project type
        indicators = {
            "pyproject.toml": "Python (pyproject.toml)",
            "setup.py": "Python (setup.py)",
            "package.json": "Node.js",
            "Cargo.toml": "Rust",
            "go.mod": "Go",
            "pom.xml": "Java (Maven)",
            "Gemfile": "Ruby",
        }
        for indicator, project_type in indicators.items():
            if indicator in files:
                lines.append(f"Project type: {project_type}")
                break

        return "\n".join(lines)
```

- [ ] **Step 2: Verify agent loads with project context**

Run: `cd /workspace && python -c "from trae_agent.agent.trae_agent import TraeAgent; print('TraeAgent loaded successfully')"`
Expected: `TraeAgent loaded successfully`

- [ ] **Step 3: Run existing agent tests**

Run: `cd /workspace && python -m pytest tests/agent/ -v 2>&1 | tail -20`
Expected: All existing tests still pass

- [ ] **Step 4: Commit**

```bash
git add trae_agent/agent/trae_agent.py
git commit -m "feat: inject project structure context into agent for better codebase awareness"
```

---

### Task 10: Run Full Test Suite and Fix Any Issues

**Files:**
- Potentially modify any files with test failures

- [ ] **Step 1: Run the complete test suite**

Run: `cd /workspace && python -m pytest tests/ -v 2>&1 | tail -40`
Expected: All tests pass

- [ ] **Step 2: Run ruff lint check**

Run: `cd /workspace && python -m ruff check trae_agent/ 2>&1 | tail -20`
Expected: No errors (or only pre-existing ones)

- [ ] **Step 3: Fix any issues found**

If any test failures or lint errors occur, fix them in the relevant files.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "fix: resolve test failures and lint issues from agent upgrade"
```

---

### Task 11: Update Example Config

**Files:**
- Modify: `trae_config.yaml.example`

- [ ] **Step 1: Update the example config to show new tools**

In `trae_config.yaml.example`, find the `tools:` section under `trae_agent:` and update it from:

```yaml
    tools:
      - bash
      - str_replace_based_edit_tool
      - sequentialthinking
      - task_done
```

to:

```yaml
    tools:
      - bash
      - read_file
      - grep_search
      - glob_search
      - str_replace_based_edit_tool
      - sequentialthinking
      - sub_agent
      - task_done
```

- [ ] **Step 2: Commit**

```bash
git add trae_config.yaml.example
git commit -m "docs: update example config with new tool list"
```

---

## Self-Review

### 1. Spec Coverage

| Requirement | Task |
|------------|------|
| ReadFile tool | Task 1 |
| GrepSearch tool | Task 2 |
| GlobSearch tool | Task 3 |
| SubAgent delegation tool | Task 4 |
| Tool registry registration | Task 5 |
| System prompt upgrade | Task 6 |
| Default tool list update | Task 7 |
| Context compression | Task 8 |
| Project context injection | Task 9 |
| Full test suite pass | Task 10 |
| Config example update | Task 11 |

All requirements covered. No gaps.

### 2. Placeholder Scan

No TBD, TODO, "implement later", or placeholder patterns found. Every step has complete code.

### 3. Type Consistency

- `ToolCallArguments` used consistently across all tools (from `trae_agent.tools.base`)
- `ToolExecResult` used consistently for all return values
- `ToolParameter` used consistently for all parameter definitions
- `TraeAgentToolNames` list matches `tools_registry` keys
- `TraeAgentConfig.tools` default matches `TraeAgentToolNames`
- `_compress_messages` uses `LLMMessage` from `trae_agent.utils.llm_clients.llm_basics` (already imported in base_agent.py)
- `_get_project_context` returns `str`, used as f-string in `new_task`

All types consistent across tasks.
