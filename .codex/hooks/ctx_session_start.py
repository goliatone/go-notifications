#!/usr/bin/env python3
from __future__ import annotations

import json

from ctx_hook_utils import (
    append_event,
    codex_source_metadata,
    doc_hashes,
    find_repo_root,
    has_ctx_repo,
    latest_review_heading,
    load_hook_input,
    resolve_active_feature,
    run_hook,
    shadow_guidance_lines,
    specs_dir,
    task_summary,
    utc_timestamp,
)


def main() -> int:
    payload = load_hook_input()
    repo_root = find_repo_root(payload.get("cwd"))
    if not has_ctx_repo(repo_root):
        return 0

    resolution = resolve_active_feature(repo_root, payload)
    feature = resolution.feature
    hashes = doc_hashes(repo_root, feature)
    start_source = payload.get("source") if isinstance(payload.get("source"), str) else None

    event = {
        "timestamp": utc_timestamp(),
        "type": "codex_session_start",
        **codex_source_metadata(),
        "session_id": payload.get("session_id"),
        "hook_event_name": payload.get("hook_event_name"),
        "start_source": start_source,
        "cwd": payload.get("cwd"),
        "feature": feature,
        "feature_resolution_source": resolution.source,
        **hashes,
    }
    if resolution.warning:
        event["feature_warning"] = resolution.warning
    append_event(repo_root, event)

    if not feature:
        return 0

    feature_path = specs_dir(repo_root) / feature
    lines = [f"Latest .ctx feature: {feature}"]

    review = latest_review_heading(feature_path)
    if review:
        lines.append(f"Latest review entry: {review}")

    tasks = task_summary(feature_path)
    if tasks:
        lines.append("Current TASKS.md focus:")
        lines.extend(tasks)

    guidance = shadow_guidance_lines(repo_root)
    if guidance:
        lines.extend(guidance)

    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": "SessionStart",
                    "additionalContext": "\n".join(lines),
                }
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(run_hook(main))
