#!/usr/bin/env python3
from __future__ import annotations

from ctx_hook_utils import (
    append_event,
    claude_source_metadata,
    doc_hashes,
    find_repo_root,
    load_hook_input,
    resolve_active_feature,
    run_hook,
    session_id,
    transcript_path,
    utc_timestamp,
)


def main() -> int:
    payload = load_hook_input()
    repo_root = find_repo_root(payload.get("cwd"))
    feature, source = resolve_active_feature(repo_root, payload)
    event = {
        "timestamp": utc_timestamp(),
        "type": "claude_session_start",
        **claude_source_metadata(),
        "session_id": session_id(payload),
        "hook_event_name": payload.get("hook_event_name") or "SessionStart",
        "transcript_path": transcript_path(payload),
        "cwd": payload.get("cwd"),
        "feature": feature,
        "feature_resolution_source": source,
        **doc_hashes(repo_root, feature),
    }
    append_event(repo_root, event)
    return 0


if __name__ == "__main__":
    raise SystemExit(run_hook(main))
