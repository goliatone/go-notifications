#!/usr/bin/env python3
from __future__ import annotations

from ctx_hook_utils import (
    append_event,
    codex_source_metadata,
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


def main() -> int:
    payload = load_hook_input()
    repo_root = find_repo_root(payload.get("cwd"))
    resolution = resolve_active_feature(repo_root, payload)
    feature = resolution.feature
    last_message = payload.get("last_assistant_message")
    if not isinstance(last_message, str):
        last_message = ""

    event = {
        "timestamp": utc_timestamp(),
        "type": "codex_stop",
        **codex_source_metadata(),
        "session_id": payload.get("session_id"),
        "turn_id": payload.get("turn_id"),
        "hook_event_name": payload.get("hook_event_name"),
        "cwd": payload.get("cwd"),
        "feature": feature,
        "feature_resolution_source": resolution.source,
        "stop_hook_active": bool(payload.get("stop_hook_active")),
        "assistant_message_preview": text_preview(last_message, env_int("CTX_HISTORY_ASSISTANT_PREVIEW_CHARS", 2000)) if last_message else None,
        "assistant_message_sha256": text_sha256(last_message) if last_message else None,
        "assistant_message_char_count": len(last_message),
        **doc_hashes(repo_root, feature),
    }
    if resolution.warning:
        event["feature_warning"] = resolution.warning
    append_event(repo_root, event)
    return 0


if __name__ == "__main__":
    raise SystemExit(run_hook(main))
