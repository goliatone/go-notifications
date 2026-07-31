#!/usr/bin/env python3
from __future__ import annotations

import gzip
import hashlib
import json
import os
import shutil
import subprocess
import sys
import time
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def warn(message: str) -> None:
    try:
        print(f"ctx hook: {message}", file=sys.stderr)
    except Exception:
        pass


def load_hook_input() -> dict[str, Any]:
    try:
        raw = sys.stdin.read()
    except Exception:
        return {}
    if not raw.strip():
        return {}
    try:
        payload = json.loads(raw)
    except Exception:
        warn("invalid hook payload JSON")
        return {}
    return payload if isinstance(payload, dict) else {}


def run_hook(main):
    try:
        return int(main() or 0)
    except Exception as exc:
        warn(str(exc))
        return 0


def utc_timestamp() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def non_empty_string(value: object) -> str | None:
    if not isinstance(value, str):
        return None
    value = value.strip()
    return value or None


def text_sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def stable_json(value: object) -> str:
    try:
        return json.dumps(value, separators=(",", ":"), sort_keys=True)
    except TypeError:
        return repr(value)


def text_preview(value: str, limit: int) -> str | None:
    if limit <= 0:
        return None
    normalized = value.replace("\r\n", "\n").replace("\r", "\n").strip()
    if len(normalized) <= limit:
        return normalized
    if limit <= 3:
        return normalized[:limit]
    return normalized[: limit - 3] + "..."


def compact_preview(value: str, limit: int = 240) -> str | None:
    compact = " ".join(value.split())
    return text_preview(compact, limit)


def env_int(name: str, default: int) -> int:
    value = non_empty_string(os.getenv(name))
    if value is None:
        return default
    try:
        parsed = int(value)
    except ValueError:
        return default
    return max(parsed, 0)


def _resolve_start_path(cwd: str | None) -> Path:
    candidate = Path(cwd or os.getcwd()).expanduser()
    try:
        return candidate.resolve()
    except Exception:
        return candidate.absolute()


def find_repo_root(cwd: str | None) -> Path:
    start = _resolve_start_path(cwd)
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            cwd=start,
            check=True,
            capture_output=True,
            text=True,
        )
        if result.stdout.strip():
            return Path(result.stdout.strip())
    except Exception:
        pass
    for candidate in (start, *start.parents):
        if (candidate / ".ctx").is_dir() or (candidate / ".git").exists() or (candidate / ".claude").is_dir():
            return candidate
    return start


def ctx_dir(repo_root: Path) -> Path:
    return repo_root / ".ctx"


def has_ctx_repo(repo_root: Path) -> bool:
    return ctx_dir(repo_root).is_dir()


def history_file(repo_root: Path) -> Path:
    configured = non_empty_string(os.getenv("CTX_HISTORY_DIR"))
    if configured:
        path = Path(configured).expanduser()
        return (path if path.is_absolute() else repo_root / path) / "events.jsonl"
    return ctx_dir(repo_root) / "history" / "events.jsonl"


def history_archive_dir(repo_root: Path) -> Path:
    return history_file(repo_root).parent / "archive"


def history_state_file(repo_root: Path) -> Path:
    return history_file(repo_root).parent / "state.json"


def specs_dir(repo_root: Path) -> Path:
    return ctx_dir(repo_root) / "specs"


def feature_exists(repo_root: Path, feature: str | None) -> bool:
    return bool(feature) and (specs_dir(repo_root) / str(feature)).is_dir()


def read_text(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except Exception:
        return ""


def resolve_active_feature(repo_root: Path, payload: dict[str, Any] | None = None) -> tuple[str | None, str | None]:
    payload = payload if isinstance(payload, dict) else {}
    explicit = non_empty_string(payload.get("feature"))
    if feature_exists(repo_root, explicit):
        return explicit, "payload_feature"
    env_feature = non_empty_string(os.getenv("CTX_ACTIVE_FEATURE"))
    if feature_exists(repo_root, env_feature):
        return env_feature, "env_feature"
    state = history_state_file(repo_root)
    if state.is_file():
        try:
            latest = non_empty_string(json.loads(state.read_text(encoding="utf-8")).get("latest_feature"))
        except Exception:
            latest = None
        if feature_exists(repo_root, latest):
            return latest, "history_state"
    manifest = ctx_dir(repo_root) / "manifest.json"
    try:
        specs = json.loads(manifest.read_text(encoding="utf-8")).get("specs", [])
    except Exception:
        specs = []
    candidates = []
    for spec in specs if isinstance(specs, list) else []:
        if not isinstance(spec, dict):
            continue
        feature = non_empty_string(spec.get("id"))
        status = non_empty_string(spec.get("status"))
        if status in {"active", "planned"} and feature_exists(repo_root, feature):
            order = spec.get("order") if isinstance(spec.get("order"), int) else sys.maxsize
            candidates.append((0 if status == "active" else 1, order, feature))
    if candidates:
        candidates.sort()
        return candidates[0][2], "manifest"
    return None, None


def file_sha256(path: Path) -> str | None:
    try:
        if not path.is_file():
            return None
        return hashlib.sha256(path.read_bytes()).hexdigest()
    except Exception:
        return None


def doc_hashes(repo_root: Path, feature: str | None) -> dict[str, str | None]:
    if not feature:
        return {}
    root = specs_dir(repo_root) / feature
    return {
        "requirements_hash_after": file_sha256(root / "REQUIREMENTS.md"),
        "design_hash_after": file_sha256(root / "DESIGN.md"),
        "tasks_hash_after": file_sha256(root / "TASKS.md"),
        "reviews_hash_after": file_sha256(root / "REVIEWS.md"),
    }


def rotate_history_if_needed(repo_root: Path) -> None:
    max_bytes = env_int("CTX_HISTORY_MAX_BYTES", 5 * 1024 * 1024)
    target = history_file(repo_root)
    try:
        if max_bytes <= 0 or target.stat().st_size < max_bytes:
            return
    except OSError:
        return
    archive_dir = history_archive_dir(repo_root)
    archive_dir.mkdir(parents=True, exist_ok=True)
    archive = archive_dir / f"events-{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}.jsonl"
    try:
        target.replace(archive)
        target.touch()
        with archive.open("rb") as source, gzip.open(str(archive) + ".gz", "wb") as destination:
            shutil.copyfileobj(source, destination)
        archive.unlink()
    except OSError:
        return


def event_dedupe_key(event: dict[str, Any]) -> str | None:
    fields_by_type = {
        "claude_session_start": ("type", "session_id", "transcript_path", "cwd"),
        "claude_user_prompt_submit": ("type", "session_id", "prompt_sha256"),
        "claude_post_tool_use": ("type", "session_id", "tool_name", "tool_input_sha256"),
        "claude_stop": ("type", "session_id", "assistant_message_sha256"),
    }
    fields = fields_by_type.get(str(event.get("type")))
    if fields is None:
        return None
    values = [event.get(field) for field in fields]
    if any(value in (None, "") for value in values):
        return None
    return json.dumps(values, separators=(",", ":"), sort_keys=True)


def recent_event_dedupe_keys(target: Path, byte_limit: int = 65536) -> set[str]:
    if not target.is_file():
        return set()
    try:
        size = target.stat().st_size
        with target.open("rb") as fh:
            if size > byte_limit:
                fh.seek(size - byte_limit)
                fh.readline()
            lines = fh.read().decode("utf-8", errors="replace").splitlines()
    except OSError:
        return set()
    keys = set()
    for line in lines:
        try:
            payload = json.loads(line)
        except Exception:
            continue
        if isinstance(payload, dict):
            key = event_dedupe_key(payload)
            if key:
                keys.add(key)
    return keys


@contextmanager
def history_file_lock(target: Path):
    lock_dir = target.with_name(f"{target.name}.lock")
    acquired = False
    for _ in range(200):
        try:
            lock_dir.mkdir()
            acquired = True
            break
        except FileExistsError:
            time.sleep(0.025)
        except Exception:
            break
    try:
        yield
    finally:
        if acquired:
            try:
                lock_dir.rmdir()
            except Exception:
                pass


def write_history_state(repo_root: Path, event: dict[str, Any]) -> None:
    feature = event.get("feature") if isinstance(event.get("feature"), str) else None
    if not feature:
        return
    state = {
        "latest_feature": feature,
        "latest_epic": None,
        "latest_action": None,
        "latest_event_type": event.get("type"),
        "latest_at": event.get("timestamp") or utc_timestamp(),
        "latest_review_entry": None,
        "updated_at": utc_timestamp(),
    }
    target = history_state_file(repo_root)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(state, separators=(",", ":"), sort_keys=True) + "\n", encoding="utf-8")


def append_event(repo_root: Path, event: dict[str, Any]) -> None:
    if not has_ctx_repo(repo_root):
        return
    target = history_file(repo_root)
    try:
        target.parent.mkdir(parents=True, exist_ok=True)
        with history_file_lock(target):
            rotate_history_if_needed(repo_root)
            if event_dedupe_key(event) in recent_event_dedupe_keys(target):
                return
            write_history_state(repo_root, event)
            with target.open("a", encoding="utf-8") as fh:
                fh.write(json.dumps(event, separators=(",", ":"), sort_keys=True))
                fh.write("\n")
    except Exception as exc:
        warn(f"failed to append event: {exc}")


def claude_source_metadata() -> dict[str, str]:
    return {"source": "claude", "source_mode": "cli"}


def session_id(payload: dict[str, Any]) -> str | None:
    return non_empty_string(payload.get("session_id")) or non_empty_string(payload.get("sessionId"))


def transcript_path(payload: dict[str, Any]) -> str | None:
    return non_empty_string(payload.get("transcript_path")) or non_empty_string(payload.get("transcriptPath"))


def response_metrics(value: object, preview_limit: int) -> dict[str, object]:
    if value is None:
        return {"tool_response_kind": "null", "tool_response_char_count": 0, "tool_response_preview": None}
    encoded = value if isinstance(value, str) else stable_json(value)
    return {
        "tool_response_kind": "string" if isinstance(value, str) else type(value).__name__,
        "tool_response_char_count": len(encoded),
        "tool_response_preview": text_preview(encoded, preview_limit),
    }


def latest_assistant_text(path: str | None) -> str:
    if not path:
        return ""
    transcript = Path(path).expanduser()
    if not transcript.is_file():
        return ""
    last = ""
    try:
        for line in transcript.read_text(encoding="utf-8").splitlines():
            try:
                payload = json.loads(line)
            except Exception:
                continue
            if not isinstance(payload, dict) or payload.get("type") != "assistant":
                continue
            message = payload.get("message") if isinstance(payload.get("message"), dict) else {}
            content = message.get("content")
            parts = []
            if isinstance(content, str):
                parts.append(content)
            elif isinstance(content, list):
                for block in content:
                    if isinstance(block, dict) and isinstance(block.get("text"), str):
                        parts.append(block["text"])
            if parts:
                last = "\n".join(parts)
    except Exception:
        return ""
    return last
