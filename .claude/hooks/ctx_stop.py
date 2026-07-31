#!/usr/bin/env python3
from __future__ import annotations

from ctx_hook_utils import (
    append_event,
    claude_source_metadata,
    doc_hashes,
    env_int,
    find_repo_root,
    latest_assistant_text,
    load_hook_input,
    resolve_active_feature,
    run_hook,
    session_id,
    text_preview,
    text_sha256,
    transcript_path,
    utc_timestamp,
)


def main() -> int:
    payload = load_hook_input()
    repo_root = find_repo_root(payload.get("cwd"))
    feature, source = resolve_active_feature(repo_root, payload)
    last_message = latest_assistant_text(transcript_path(payload))
    event = {
        "timestamp": utc_timestamp(),
        "type": "claude_stop",
        **claude_source_metadata(),
        "session_id": session_id(payload),
        "hook_event_name": payload.get("hook_event_name") or "Stop",
        "transcript_path": transcript_path(payload),
        "cwd": payload.get("cwd"),
        "feature": feature,
        "feature_resolution_source": source,
        "assistant_message_preview": text_preview(last_message, env_int("CTX_HISTORY_ASSISTANT_PREVIEW_CHARS", 2000)) if last_message else None,
        "assistant_message_sha256": text_sha256(last_message) if last_message else None,
        "assistant_message_char_count": len(last_message),
        **doc_hashes(repo_root, feature),
    }
    append_event(repo_root, event)
    return 0


if __name__ == "__main__":
    raise SystemExit(run_hook(main))
