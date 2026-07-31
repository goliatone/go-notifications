#!/usr/bin/env python3
from __future__ import annotations

import json
import os

from ctx_hook_utils import (
    append_event,
    codex_source_metadata,
    command_preview,
    doc_hashes,
    env_int,
    find_repo_root,
    load_hook_input,
    resolve_active_feature,
    run_hook,
    text_preview,
    text_sha256,
    utc_timestamp,
)

# Checked in order for the generic input preview; first non-empty string wins.
TOOL_INPUT_PREVIEW_KEYS = (
    "command",
    "file_path",
    "path",
    "pattern",
    "query",
    "url",
    "patch",
    "content",
    "prompt",
)


def tool_event_logging_enabled() -> bool:
    value = os.getenv("CTX_HISTORY_TOOL_EVENTS", "1").strip().lower()
    return value not in {"0", "false", "no", "off"}


def tool_file_path(tool_input: dict) -> str | None:
    for key in ("file_path", "path"):
        value = tool_input.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return None


def tool_input_preview_source(tool_input: dict) -> str:
    for key in TOOL_INPUT_PREVIEW_KEYS:
        value = tool_input.get(key)
        if isinstance(value, str) and value.strip():
            return value
    if tool_input:
        try:
            return json.dumps(tool_input, sort_keys=True)
        except TypeError:
            return repr(tool_input)
    return ""


def response_metrics(value: object, preview_limit: int) -> dict[str, object]:
    if value is None:
        return {
            "tool_response_kind": "null",
            "tool_response_char_count": 0,
            "tool_response_preview": None,
        }
    if isinstance(value, str):
        kind = "string"
        encoded = value
    else:
        kind = type(value).__name__
        try:
            encoded = json.dumps(value, sort_keys=True)
        except TypeError:
            encoded = repr(value)
    return {
        "tool_response_kind": kind,
        "tool_response_char_count": len(encoded),
        "tool_response_preview": text_preview(encoded, preview_limit),
    }


def main() -> int:
    if not tool_event_logging_enabled():
        return 0

    payload = load_hook_input()
    repo_root = find_repo_root(payload.get("cwd"))
    resolution = resolve_active_feature(repo_root, payload)
    feature = resolution.feature
    tool_input = payload.get("tool_input") if isinstance(payload.get("tool_input"), dict) else {}
    command = tool_input.get("command") if isinstance(tool_input.get("command"), str) else ""
    input_limit = env_int("CTX_HISTORY_TOOL_INPUT_PREVIEW_CHARS", 240)
    response_limit = env_int("CTX_HISTORY_TOOL_RESPONSE_PREVIEW_CHARS", 500)
    metrics = response_metrics(payload.get("tool_response"), response_limit)

    event = {
        "timestamp": utc_timestamp(),
        "type": "codex_post_tool_use",
        **codex_source_metadata(),
        "session_id": payload.get("session_id"),
        "turn_id": payload.get("turn_id"),
        "tool_use_id": payload.get("tool_use_id"),
        "hook_event_name": payload.get("hook_event_name"),
        "tool_name": payload.get("tool_name"),
        "cwd": payload.get("cwd"),
        "feature": feature,
        "feature_resolution_source": resolution.source,
        "command_preview": command_preview(command, input_limit) if input_limit > 0 else None,
        "command_sha256": text_sha256(command),
        "tool_file_path": tool_file_path(tool_input),
        "tool_input_preview": text_preview(tool_input_preview_source(tool_input), input_limit),
        **metrics,
        **doc_hashes(repo_root, feature),
    }
    if resolution.warning:
        event["feature_warning"] = resolution.warning
    append_event(repo_root, event)
    return 0


if __name__ == "__main__":
    raise SystemExit(run_hook(main))
