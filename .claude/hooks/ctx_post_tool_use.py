#!/usr/bin/env python3
from __future__ import annotations

from ctx_hook_utils import (
    append_event,
    claude_source_metadata,
    compact_preview,
    doc_hashes,
    env_int,
    find_repo_root,
    load_hook_input,
    resolve_active_feature,
    response_metrics,
    run_hook,
    session_id,
    stable_json,
    text_preview,
    text_sha256,
    transcript_path,
    utc_timestamp,
)


def main() -> int:
    payload = load_hook_input()
    repo_root = find_repo_root(payload.get("cwd"))
    feature, source = resolve_active_feature(repo_root, payload)
    tool_input = payload.get("tool_input") if isinstance(payload.get("tool_input"), dict) else {}
    input_text = stable_json(tool_input)
    input_limit = env_int("CTX_HISTORY_TOOL_INPUT_PREVIEW_CHARS", 240)
    response_limit = env_int("CTX_HISTORY_TOOL_RESPONSE_PREVIEW_CHARS", 500)
    command = tool_input.get("command") if isinstance(tool_input.get("command"), str) else ""
    event = {
        "timestamp": utc_timestamp(),
        "type": "claude_post_tool_use",
        **claude_source_metadata(),
        "session_id": session_id(payload),
        "hook_event_name": payload.get("hook_event_name") or "PostToolUse",
        "transcript_path": transcript_path(payload),
        "tool_name": payload.get("tool_name"),
        "tool_input_sha256": text_sha256(input_text),
        "command_preview": compact_preview(command, input_limit) if command else None,
        "tool_input_preview": text_preview(input_text, input_limit),
        "cwd": payload.get("cwd"),
        "feature": feature,
        "feature_resolution_source": source,
        **response_metrics(payload.get("tool_response"), response_limit),
        **doc_hashes(repo_root, feature),
    }
    append_event(repo_root, event)
    return 0


if __name__ == "__main__":
    raise SystemExit(run_hook(main))
