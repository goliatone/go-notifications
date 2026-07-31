from __future__ import annotations

import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path
from unittest import mock

import ctx_hook_utils as utils


HOOKS_DIR = Path(__file__).resolve().parent
CODEX_DIR = HOOKS_DIR.parent
SOURCE_HOOKS_JSON = CODEX_DIR / "hooks.json"
# Present when running inside the skill repo; absent in installed repo copies.
INIT_CODEX_REPO = HOOKS_DIR.parents[2] / "scripts" / "init_codex_repo.py"


def write_file(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def create_ctx_repo(
    root: Path,
    *,
    manifest: dict | None = None,
    history: list[dict] | None = None,
    features: dict[str, dict[str, str]] | None = None,
    include_codebase: bool = False,
) -> None:
    write_file(root / ".ctx" / "README.md", "# Project Context\n")
    features = features or {}
    for feature_id, docs in features.items():
        feature_root = root / ".ctx" / "specs" / feature_id
        for name, content in docs.items():
            write_file(feature_root / name, content)

    if manifest is not None:
        write_file(root / ".ctx" / "manifest.json", json.dumps(manifest, indent=2))

    if history is not None:
        lines = "\n".join(json.dumps(item, separators=(",", ":"), sort_keys=True) for item in history)
        if lines:
            lines += "\n"
        write_file(root / ".ctx" / "history" / "events.jsonl", lines)

    if include_codebase:
        write_file(root / ".ctx" / "codebase" / "OVERVIEW.md", "# Overview\n")


def copy_hook_assets(target_root: Path) -> None:
    target_hooks = target_root / ".codex" / "hooks"
    target_hooks.mkdir(parents=True, exist_ok=True)
    for source in HOOKS_DIR.iterdir():
        if source.name == "__pycache__":
            continue
        if source.is_file():
            shutil.copy2(source, target_hooks / source.name)
    shutil.copy2(SOURCE_HOOKS_JSON, target_root / ".codex" / "hooks.json")


def feature_docs(tasks: str | None = None, reviews: str | None = None) -> dict[str, str]:
    return {
        "REQUIREMENTS.md": "# Requirements\n",
        "DESIGN.md": "# Design\n",
        "TASKS.md": tasks
        or textwrap.dedent(
            """\
            # Feature Implementation Plan

            ## In Progress
            - [~] T03 Harden hook utility behavior

            ## Next
            - [ ] T04 Implement deterministic active feature resolution
            """
        ),
        **({"REVIEWS.md": reviews} if reviews is not None else {}),
    }


def last_event(root: Path) -> dict:
    lines = (root / ".ctx" / "history" / "events.jsonl").read_text(encoding="utf-8").splitlines()
    return json.loads(lines[-1])


class HookUtilsTest(unittest.TestCase):
    def test_hooks_json_uses_repo_dispatch_without_git_preflight(self) -> None:
        hooks_payload = json.loads(SOURCE_HOOKS_JSON.read_text(encoding="utf-8"))
        commands = []
        for entries in hooks_payload["hooks"].values():
            for entry in entries:
                for hook in entry["hooks"]:
                    commands.append(hook["command"])

        self.assertTrue(commands)
        for command in commands:
            self.assertIn("ctx_hook_dispatch.sh", command)
            self.assertNotIn("git rev-parse --show-toplevel", command)

    def test_hooks_json_post_tool_use_matches_all_tools(self) -> None:
        hooks_payload = json.loads(SOURCE_HOOKS_JSON.read_text(encoding="utf-8"))
        groups = hooks_payload["hooks"]["PostToolUse"]
        self.assertEqual(len(groups), 1)
        self.assertNotIn("matcher", groups[0])

    def test_text_preview_bounds(self) -> None:
        self.assertIsNone(utils.text_preview("anything", 0))
        self.assertIsNone(utils.text_preview("anything", -5))
        self.assertEqual(utils.text_preview("  hello  ", 100), "hello")
        self.assertEqual(utils.text_preview("a\r\nb\rc", 100), "a\nb\nc")
        multiline = "first line\n\nsecond paragraph"
        self.assertEqual(utils.text_preview(multiline, 100), multiline)
        truncated = utils.text_preview("x" * 50, 10)
        self.assertEqual(truncated, "x" * 7 + "...")
        self.assertEqual(len(truncated), 10)
        self.assertEqual(utils.text_preview("x" * 50, 2), "xx")

    def test_find_repo_root_prefers_ctx_without_git(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            nested = root / "a" / "b" / "c"
            nested.mkdir(parents=True)
            create_ctx_repo(root, features={"alpha": feature_docs()})

            self.assertEqual(utils.find_repo_root(str(nested)), root.resolve())

    def test_append_event_noops_without_ctx(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            utils.append_event(root, {"type": "noop"})
            self.assertFalse((root / ".ctx").exists())

    def test_resolve_active_feature_prefers_payload_and_env(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            create_ctx_repo(
                root,
                features={
                    "done-feature": feature_docs(),
                    "env-feature": feature_docs(),
                    "active-feature": feature_docs(),
                },
                manifest={
                    "version": 1,
                    "specs": [
                        {"id": "active-feature", "order": 2, "status": "active"},
                        {"id": "done-feature", "order": 1, "status": "done"},
                    ],
                },
            )

            with mock.patch.dict(os.environ, {"CTX_ACTIVE_FEATURE": "env-feature"}, clear=False):
                resolved = utils.resolve_active_feature(root, {"feature": "done-feature"})
                self.assertEqual(resolved.feature, "done-feature")
                self.assertEqual(resolved.source, "payload_feature")

                resolved = utils.resolve_active_feature(root, {})
                self.assertEqual(resolved.feature, "env-feature")
                self.assertEqual(resolved.source, "env_feature")

    def test_resolve_active_feature_warns_for_multiple_active(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            create_ctx_repo(
                root,
                features={
                    "alpha": feature_docs(),
                    "beta": feature_docs(),
                },
                manifest={
                    "version": 1,
                    "specs": [
                        {"id": "beta", "order": 4, "status": "active"},
                        {"id": "alpha", "order": 1, "status": "active"},
                    ],
                },
            )

            resolved = utils.resolve_active_feature(root, {})
            self.assertEqual(resolved.feature, "alpha")
            self.assertEqual(resolved.source, "manifest_active")
            self.assertEqual(resolved.warning, "multiple_active_features:alpha,beta")

    def test_resolve_active_feature_uses_history_then_earliest_planned(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            create_ctx_repo(
                root,
                features={
                    "history-feature": feature_docs(),
                    "planned-earliest": feature_docs(),
                    "planned-later": feature_docs(),
                    "done-feature": feature_docs(),
                    "folded-feature": feature_docs(),
                    "deferred-feature": feature_docs(),
                },
                manifest={
                    "version": 1,
                    "specs": [
                        {"id": "done-feature", "order": 0, "status": "done"},
                        {"id": "planned-later", "order": 4, "status": "planned"},
                        {"id": "folded-feature", "order": 2, "status": "folded"},
                        {"id": "planned-earliest", "order": 1, "status": "planned"},
                        {"id": "deferred-feature", "order": 3, "status": "deferred"},
                    ],
                },
                history=[{"feature": "history-feature", "type": "codex_user_prompt_submit"}],
            )

            # The default fixture TASKS.md carries a [~] task, so the history
            # feature resolves through the doing-task tier.
            resolved = utils.resolve_active_feature(root, {})
            self.assertEqual(resolved.feature, "history-feature")
            self.assertEqual(resolved.source, "history_doing")

            # Without an in-progress task it still wins via the plain history tier.
            write_file(
                root / ".ctx" / "specs" / "history-feature" / "TASKS.md",
                "# Feature Implementation Plan\n\n## Next\n- [ ] T01 Pending\n",
            )
            resolved = utils.resolve_active_feature(root, {})
            self.assertEqual(resolved.feature, "history-feature")
            self.assertEqual(resolved.source, "history")

            write_file(root / ".ctx" / "history" / "events.jsonl", "")
            resolved = utils.resolve_active_feature(root, {})
            self.assertEqual(resolved.feature, "planned-earliest")
            self.assertEqual(resolved.source, "manifest_planned")

    def test_resolve_active_feature_without_manifest_or_ctx(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            create_ctx_repo(root, features={"alpha": feature_docs()})

            resolved = utils.resolve_active_feature(root, {})
            self.assertIsNone(resolved.feature)
            self.assertEqual(resolved.source, "none")

            empty_root = root / "no-ctx"
            empty_root.mkdir()
            resolved = utils.resolve_active_feature(empty_root, {})
            self.assertIsNone(resolved.feature)
            self.assertEqual(resolved.source, "no_ctx")


class HookScriptTest(unittest.TestCase):
    def run_script(
        self,
        script_name: str,
        repo_root: Path,
        payload: object,
        *,
        cwd: Path | None = None,
        env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        script_path = HOOKS_DIR / script_name
        # Drop inherited CTX_* vars so the developer's environment cannot
        # skew preview limits or feature resolution inside the subprocess.
        merged_env = {key: value for key, value in os.environ.items() if not key.startswith("CTX_")}
        if env:
            merged_env.update(env)
        return subprocess.run(
            [sys.executable, str(script_path)],
            input=json.dumps(payload) if not isinstance(payload, str) else payload,
            text=True,
            capture_output=True,
            cwd=str(cwd or repo_root),
            env=merged_env,
            check=False,
        )

    def active_alpha_repo(self, root: Path) -> None:
        create_ctx_repo(
            root,
            manifest={
                "version": 1,
                "specs": [{"id": "alpha", "order": 1, "status": "active"}],
            },
            features={"alpha": feature_docs()},
        )

    def test_session_start_launcher_noops_without_ctx_and_without_git(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            copy_hook_assets(root)
            workdir = root / "nested" / "dir"
            workdir.mkdir(parents=True)

            command = json.loads((root / ".codex" / "hooks.json").read_text(encoding="utf-8"))["hooks"]["SessionStart"][0]["hooks"][0]["command"]
            result = subprocess.run(
                ["sh", "-c", command],
                input=json.dumps({"cwd": str(workdir), "hook_event_name": "SessionStart"}),
                text=True,
                capture_output=True,
                cwd=str(workdir),
                check=False,
            )

            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout.strip(), "")
            self.assertFalse((root / ".ctx").exists())

    def test_session_start_launcher_runs_without_git_when_ctx_exists(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            copy_hook_assets(root)
            create_ctx_repo(
                root,
                manifest={
                    "version": 1,
                    "specs": [{"id": "alpha", "order": 1, "status": "active"}],
                },
                features={
                    "alpha": feature_docs(
                        reviews="# Review Log\n\n## IR-2026-04-29-01 Implementation Review\n"
                    )
                },
                include_codebase=True,
            )
            workdir = root / "nested" / "dir"
            workdir.mkdir(parents=True)

            command = json.loads((root / ".codex" / "hooks.json").read_text(encoding="utf-8"))["hooks"]["SessionStart"][0]["hooks"][0]["command"]
            result = subprocess.run(
                ["sh", "-c", command],
                input=json.dumps(
                    {
                        "cwd": str(workdir),
                        "hook_event_name": "SessionStart",
                        "source": "startup",
                    }
                ),
                text=True,
                capture_output=True,
                cwd=str(workdir),
                check=False,
            )

            self.assertEqual(result.returncode, 0)
            payload = json.loads(result.stdout)
            context = payload["hookSpecificOutput"]["additionalContext"]
            self.assertIn("Latest .ctx feature: alpha", context)
            self.assertIn("Latest review entry: IR-2026-04-29-01 Implementation Review", context)
            self.assertIn("Current TASKS.md focus:", context)
            self.assertIn("Shadow navigation:", context)
            self.assertIn("Read source files before implementation or final review", context)
            event = last_event(root)
            self.assertEqual(event["feature"], "alpha")
            self.assertEqual(event["feature_resolution_source"], "manifest_active")

    def test_session_start_emits_warning_for_multiple_active_manifest_specs(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            create_ctx_repo(
                root,
                manifest={
                    "version": 1,
                    "specs": [
                        {"id": "beta", "order": 2, "status": "active"},
                        {"id": "alpha", "order": 1, "status": "active"},
                    ],
                },
                features={"alpha": feature_docs(), "beta": feature_docs()},
            )

            result = self.run_script(
                "ctx_session_start.py",
                root,
                {
                    "cwd": str(root),
                    "hook_event_name": "SessionStart",
                    "source": "startup",
                },
            )

            self.assertEqual(result.returncode, 0)
            payload = json.loads(result.stdout)
            self.assertNotIn("Shadow navigation:", payload["hookSpecificOutput"]["additionalContext"])
            event = last_event(root)
            self.assertEqual(event["feature"], "alpha")
            self.assertEqual(event["feature_warning"], "multiple_active_features:alpha,beta")

    def test_prompt_hook_records_bounded_preview_and_hashes(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            prompt = "step one\n\nstep two with detail\n" + ("filler " * 40)
            result = self.run_script(
                "ctx_user_prompt_submit.py",
                root,
                {
                    "cwd": str(root),
                    "hook_event_name": "UserPromptSubmit",
                    "prompt": prompt,
                    "session_id": "session-1",
                    "turn_id": "turn-1",
                },
            )

            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertEqual(event["feature"], "alpha")
            self.assertEqual(event["feature_resolution_source"], "manifest_active")
            # Default cap (2000) keeps the full prompt, newlines included.
            self.assertEqual(event["prompt_preview"], prompt.strip())
            self.assertIn("\n\n", event["prompt_preview"])
            self.assertEqual(event["prompt_sha256"], utils.text_sha256(prompt))
            self.assertEqual(event["prompt_char_count"], len(prompt))
            self.assertNotIn("prompt", event)

    def test_prompt_hook_preview_respects_env_cap_and_disable(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            prompt = "word " * 100
            payload = {
                "cwd": str(root),
                "hook_event_name": "UserPromptSubmit",
                "prompt": prompt,
                "session_id": "session-1",
                "turn_id": "turn-1",
            }

            result = self.run_script(
                "ctx_user_prompt_submit.py",
                root,
                payload,
                env={"CTX_HISTORY_PROMPT_PREVIEW_CHARS": "50"},
            )
            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertLessEqual(len(event["prompt_preview"]), 50)
            self.assertTrue(event["prompt_preview"].endswith("..."))

            payload["turn_id"] = "turn-2"
            result = self.run_script(
                "ctx_user_prompt_submit.py",
                root,
                payload,
                env={"CTX_HISTORY_PROMPT_PREVIEW_CHARS": "0"},
            )
            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertIsNone(event["prompt_preview"])
            self.assertEqual(event["prompt_sha256"], utils.text_sha256(prompt))
            self.assertEqual(event["prompt_char_count"], len(prompt))

    def test_post_tool_use_handles_non_string_fields(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            create_ctx_repo(
                root,
                manifest={
                    "version": 1,
                    "specs": [{"id": "alpha", "order": 1, "status": "planned"}],
                },
                features={"alpha": feature_docs()},
            )
            result = self.run_script(
                "ctx_post_tool_use.py",
                root,
                {
                    "cwd": str(root),
                    "hook_event_name": "PostToolUse",
                    "tool_name": "Bash",
                    "tool_input": {"command": 42},
                    "tool_response": {"ok": True},
                },
            )

            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertEqual(event["feature"], "alpha")
            self.assertEqual(event["command_preview"], "")
            self.assertEqual(event["command_sha256"], utils.text_sha256(""))
            self.assertIsNone(event["tool_file_path"])
            self.assertEqual(event["tool_input_preview"], json.dumps({"command": 42}, sort_keys=True))
            self.assertEqual(event["tool_response_kind"], "dict")
            self.assertGreater(event["tool_response_char_count"], 0)
            self.assertEqual(event["tool_response_preview"], json.dumps({"ok": True}, sort_keys=True))
            self.assertNotIn("tool_response", event)

    def test_post_tool_use_records_edit_tool_metadata(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            result = self.run_script(
                "ctx_post_tool_use.py",
                root,
                {
                    "cwd": str(root),
                    "hook_event_name": "PostToolUse",
                    "tool_name": "Edit",
                    "tool_use_id": "call-1",
                    "tool_input": {
                        "file_path": "internal/ui/app.js",
                        "old_string": "a",
                        "new_string": "b",
                    },
                    "tool_response": "ok",
                },
            )

            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertEqual(event["tool_name"], "Edit")
            self.assertEqual(event["tool_file_path"], "internal/ui/app.js")
            self.assertEqual(event["tool_input_preview"], "internal/ui/app.js")
            self.assertEqual(event["command_preview"], "")
            self.assertEqual(event["tool_response_kind"], "string")
            self.assertEqual(event["tool_response_preview"], "ok")

    def test_post_tool_use_response_preview_disable_and_truncation(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            payload = {
                "cwd": str(root),
                "hook_event_name": "PostToolUse",
                "tool_name": "Bash",
                "tool_use_id": "call-1",
                "tool_input": {"command": "go test ./..."},
                "tool_response": "y" * 5000,
            }

            result = self.run_script(
                "ctx_post_tool_use.py",
                root,
                payload,
                env={"CTX_HISTORY_TOOL_RESPONSE_PREVIEW_CHARS": "0"},
            )
            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertIsNone(event["tool_response_preview"])
            self.assertEqual(event["tool_response_char_count"], 5000)

            payload["tool_use_id"] = "call-2"
            result = self.run_script("ctx_post_tool_use.py", root, payload)
            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertEqual(len(event["tool_response_preview"]), 500)
            self.assertTrue(event["tool_response_preview"].endswith("..."))
            self.assertEqual(event["command_preview"], "go test ./...")
            self.assertEqual(event["tool_input_preview"], "go test ./...")

    def test_post_tool_use_duplicate_events_are_deduped(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            payload = {
                "cwd": str(root),
                "hook_event_name": "PostToolUse",
                "tool_name": "Bash",
                "session_id": "session-1",
                "turn_id": "turn-1",
                "tool_use_id": "call-1",
                "tool_input": {"command": "ls"},
                "tool_response": "ok",
            }

            first = self.run_script("ctx_post_tool_use.py", root, payload)
            second = self.run_script("ctx_post_tool_use.py", root, payload)
            self.assertEqual(first.returncode, 0)
            self.assertEqual(second.returncode, 0)
            lines = (root / ".ctx" / "history" / "events.jsonl").read_text(encoding="utf-8").splitlines()
            self.assertEqual(len([line for line in lines if line.strip()]), 1)

    def test_missing_feature_docs_do_not_break_hashing(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            create_ctx_repo(
                root,
                manifest={
                    "version": 1,
                    "specs": [{"id": "alpha", "order": 1, "status": "active"}],
                },
                features={"alpha": {"TASKS.md": "# Feature Implementation Plan\n"}},
            )
            result = self.run_script(
                "ctx_user_prompt_submit.py",
                root,
                {
                    "cwd": str(root),
                    "hook_event_name": "UserPromptSubmit",
                    "prompt": "hello",
                },
            )

            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertEqual(event["feature"], "alpha")
            self.assertIsNone(event["requirements_hash"])
            self.assertIsNone(event["design_hash"])
            self.assertIsNone(event["reviews_hash"])

    def test_malformed_stdin_is_fail_open(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            result = subprocess.run(
                [sys.executable, str(HOOKS_DIR / "ctx_stop.py")],
                input="{not-json",
                text=True,
                capture_output=True,
                cwd=str(root),
                check=False,
            )

            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertEqual(event["feature"], "alpha")
            self.assertEqual(event["feature_resolution_source"], "manifest_active")

    def test_stop_hook_records_bounded_assistant_preview(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            message = "Done.\n\n- updated the parser\n- added tests"
            result = self.run_script(
                "ctx_stop.py",
                root,
                {
                    "cwd": str(root),
                    "hook_event_name": "Stop",
                    "session_id": "session-1",
                    "turn_id": "turn-1",
                    "last_assistant_message": message,
                },
            )

            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertEqual(event["assistant_message_preview"], message)
            self.assertEqual(event["assistant_message_sha256"], utils.text_sha256(message))
            self.assertEqual(event["assistant_message_char_count"], len(message))
            # The raw payload key must never be copied wholesale.
            self.assertNotIn("last_assistant_message", event)

    def test_stop_hook_preview_disabled_keeps_hash_only(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            root = Path(tmpdir)
            self.active_alpha_repo(root)
            message = "secret output"
            result = self.run_script(
                "ctx_stop.py",
                root,
                {
                    "cwd": str(root),
                    "hook_event_name": "Stop",
                    "session_id": "session-1",
                    "turn_id": "turn-1",
                    "last_assistant_message": message,
                },
                env={"CTX_HISTORY_ASSISTANT_PREVIEW_CHARS": "0"},
            )

            self.assertEqual(result.returncode, 0)
            event = last_event(root)
            self.assertIsNone(event["assistant_message_preview"])
            self.assertEqual(event["assistant_message_sha256"], utils.text_sha256(message))
            self.assertEqual(event["assistant_message_char_count"], len(message))
            self.assertNotIn("last_assistant_message", event)


@unittest.skipUnless(INIT_CODEX_REPO.is_file(), "init_codex_repo.py only available in the skill repo")
class MergeHooksJsonTest(unittest.TestCase):
    def load_installer(self):
        spec = importlib.util.spec_from_file_location("init_codex_repo", INIT_CODEX_REPO)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def test_merge_migrates_managed_handler_to_new_matcher_group(self) -> None:
        installer = self.load_installer()
        template = json.loads(SOURCE_HOOKS_JSON.read_text(encoding="utf-8"))

        # Simulate an old install where the PostToolUse handler lived under a
        # "Bash" matcher group.
        legacy = json.loads(json.dumps(template))
        legacy["hooks"]["PostToolUse"][0]["matcher"] = "Bash"

        with tempfile.TemporaryDirectory() as tmpdir:
            target = Path(tmpdir) / "hooks.json"
            target.write_text(json.dumps(legacy, indent=2) + "\n", encoding="utf-8")

            updated = installer.merge_hooks_json(target, SOURCE_HOOKS_JSON)
            self.assertTrue(updated)

            merged = json.loads(target.read_text(encoding="utf-8"))
            groups = merged["hooks"]["PostToolUse"]
            self.assertEqual(len(groups), 1)
            self.assertNotIn("matcher", groups[0])
            handlers = groups[0]["hooks"]
            self.assertEqual(len(handlers), 1)
            self.assertIn("ctx_post_tool_use.py", handlers[0]["command"])

            # Second merge is a no-op.
            self.assertFalse(installer.merge_hooks_json(target, SOURCE_HOOKS_JSON))

    def test_merge_preserves_unmanaged_handlers_in_other_groups(self) -> None:
        installer = self.load_installer()
        template = json.loads(SOURCE_HOOKS_JSON.read_text(encoding="utf-8"))

        legacy = json.loads(json.dumps(template))
        legacy["hooks"]["PostToolUse"][0]["matcher"] = "Bash"
        legacy["hooks"]["PostToolUse"][0]["hooks"].append(
            {"type": "command", "command": "echo custom-user-hook"}
        )

        with tempfile.TemporaryDirectory() as tmpdir:
            target = Path(tmpdir) / "hooks.json"
            target.write_text(json.dumps(legacy, indent=2) + "\n", encoding="utf-8")

            installer.merge_hooks_json(target, SOURCE_HOOKS_JSON)

            merged = json.loads(target.read_text(encoding="utf-8"))
            groups = merged["hooks"]["PostToolUse"]
            self.assertEqual(len(groups), 2)
            by_matcher = {group.get("matcher"): group for group in groups}
            self.assertIn("Bash", by_matcher)
            bash_commands = [handler["command"] for handler in by_matcher["Bash"]["hooks"]]
            self.assertEqual(bash_commands, ["echo custom-user-hook"])
            managed_commands = [handler["command"] for handler in by_matcher[None]["hooks"]]
            self.assertEqual(len(managed_commands), 1)
            self.assertIn("ctx_post_tool_use.py", managed_commands[0])


if __name__ == "__main__":
    unittest.main()
