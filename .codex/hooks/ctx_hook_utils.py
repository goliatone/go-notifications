#!/usr/bin/env python3
from __future__ import annotations

import gzip
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import time
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class FeatureResolution:
    feature: str | None
    source: str | None = None
    warning: str | None = None


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


def utc_timestamp() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def non_empty_string(value: object) -> str | None:
    if not isinstance(value, str):
        return None
    normalized = value.strip()
    return normalized or None


def safe_read_text(path: Path) -> str | None:
    try:
        return path.read_text(encoding="utf-8")
    except FileNotFoundError:
        return None
    except Exception:
        warn(f"unable to read file: {path}")
        return None


def read_lines(path: Path) -> list[str]:
    content = safe_read_text(path)
    if content is None:
        return []
    return content.splitlines()


def file_sha256(path: Path) -> str | None:
    try:
        if not path.is_file():
            return None
        digest = hashlib.sha256()
        digest.update(path.read_bytes())
        return digest.hexdigest()
    except Exception:
        warn(f"unable to hash file: {path}")
        return None


def text_sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def compact_preview(value: str, limit: int = 240) -> str:
    compact = " ".join(value.split())
    if len(compact) <= limit:
        return compact
    return compact[: limit - 3] + "..."


def command_preview(command: str, limit: int = 240) -> str:
    return compact_preview(command, limit=limit)


def text_preview(value: str, limit: int) -> str | None:
    """Bounded multi-line preview. Returns None when limit disables capture (<= 0)."""
    if limit <= 0:
        return None
    normalized = value.replace("\r\n", "\n").replace("\r", "\n").strip()
    if len(normalized) <= limit:
        return normalized
    if limit <= 3:
        return normalized[:limit]
    return normalized[: limit - 3] + "..."


def codex_source_mode() -> str:
    originator = non_empty_string(os.getenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE"))
    if originator and "vscode" in originator.lower():
        return "vscode"

    term_program = non_empty_string(os.getenv("TERM_PROGRAM"))
    if term_program and term_program.lower() == "vscode":
        return "vscode"

    for key in ("VSCODE_IPC_HOOK", "VSCODE_PID", "VSCODE_CWD", "VSCODE_INJECTION", "VSCODE_CLI"):
        if non_empty_string(os.getenv(key)):
            return "vscode"

    return "cli"


def codex_source_metadata() -> dict[str, str | None]:
    return {
        "source": "codex",
        "source_mode": codex_source_mode(),
        "source_originator": non_empty_string(os.getenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE")),
        "source_term_program": non_empty_string(os.getenv("TERM_PROGRAM")),
    }


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
        root = result.stdout.strip()
        if root:
            return Path(root)
    except Exception:
        pass

    for candidate in (start, *start.parents):
        if (candidate / ".ctx").is_dir():
            return candidate
        if (candidate / ".git").exists():
            return candidate
        if (candidate / ".codex").is_dir():
            return candidate
    return start


def ctx_dir(repo_root: Path) -> Path:
    return repo_root / ".ctx"


def has_ctx_repo(repo_root: Path) -> bool:
    return ctx_dir(repo_root).is_dir()


def history_dir(repo_root: Path) -> Path:
    configured = non_empty_string(os.getenv("CTX_HISTORY_DIR"))
    if configured:
        path = Path(configured).expanduser()
        return path if path.is_absolute() else repo_root / path
    return ctx_dir(repo_root) / "history"


def history_file(repo_root: Path) -> Path:
    return history_dir(repo_root) / "events.jsonl"


def history_archive_dir(repo_root: Path) -> Path:
    configured = non_empty_string(os.getenv("CTX_HISTORY_ARCHIVE_DIR"))
    if configured:
        path = Path(configured).expanduser()
        return path if path.is_absolute() else repo_root / path
    return history_dir(repo_root) / "archive"


def history_state_file(repo_root: Path) -> Path:
    configured = non_empty_string(os.getenv("CTX_HISTORY_STATE_FILE"))
    if configured:
        path = Path(configured).expanduser()
        return path if path.is_absolute() else repo_root / path
    return history_dir(repo_root) / "state.json"


def manifest_file(repo_root: Path) -> Path:
    return ctx_dir(repo_root) / "manifest.json"


def specs_dir(repo_root: Path) -> Path:
    return ctx_dir(repo_root) / "specs"


def feature_dir(repo_root: Path, feature: str) -> Path:
    return specs_dir(repo_root) / feature


def feature_exists(repo_root: Path, feature: str | None) -> bool:
    return bool(feature) and feature_dir(repo_root, feature).is_dir()


def env_int(name: str, default: int) -> int:
    value = non_empty_string(os.getenv(name))
    if value is None:
        return default
    try:
        parsed = int(value)
    except ValueError:
        return default
    return max(parsed, 0)


def prune_history_archives(repo_root: Path) -> None:
    max_archives = env_int("CTX_HISTORY_MAX_ARCHIVES", 20)
    if max_archives <= 0:
        return

    archive_dir = history_archive_dir(repo_root)
    if not archive_dir.is_dir():
        return

    archives = sorted(
        path
        for path in archive_dir.iterdir()
        if path.is_file()
        and path.name.startswith("events-")
        and (path.name.endswith(".jsonl") or path.name.endswith(".jsonl.gz"))
    )
    for path in archives[: max(0, len(archives) - max_archives)]:
        try:
            path.unlink()
        except OSError:
            pass


def rotate_history_if_needed(repo_root: Path) -> None:
    max_bytes = env_int("CTX_HISTORY_MAX_BYTES", 5 * 1024 * 1024)
    if max_bytes <= 0:
        return

    target = history_file(repo_root)
    try:
        size = target.stat().st_size
    except OSError:
        return
    if size <= 0 or size < max_bytes:
        return

    archive_dir = history_archive_dir(repo_root)
    archive_dir.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    archive = archive_dir / f"events-{stamp}.jsonl"
    if archive.exists() or archive.with_suffix(archive.suffix + ".gz").exists():
        archive = archive_dir / f"events-{stamp}-{os.getpid()}.jsonl"

    try:
        target.replace(archive)
        target.touch()
        compressed = archive.with_suffix(archive.suffix + ".gz")
        with archive.open("rb") as source, gzip.open(compressed, "wb") as destination:
            shutil.copyfileobj(source, destination)
        archive.unlink()
    except OSError:
        return

    prune_history_archives(repo_root)


def latest_review_heading(feature_path: Path) -> str | None:
    reviews_file = feature_path / "REVIEWS.md"
    if not reviews_file.is_file():
        return None

    for line in read_lines(reviews_file):
        if line.startswith("## "):
            return line[3:].strip()
    return None


def read_spec_epic(repo_root: Path, feature: str | None) -> str | None:
    if not feature:
        return None

    requirements_file = feature_dir(repo_root, feature) / "REQUIREMENTS.md"
    content = safe_read_text(requirements_file)
    if content is None or not content.startswith("---"):
        return None

    lines = content.splitlines()
    if not lines or lines[0].strip() != "---":
        return None

    for line in lines[1:]:
        if line.strip() == "---":
            return None
        key, sep, value = line.partition(":")
        if sep and key.strip() == "epic":
            normalized = value.strip().strip("\"'")
            return normalized or None
    return None


def write_history_state(repo_root: Path, event: dict[str, Any]) -> None:
    feature = event.get("feature")
    if not isinstance(feature, str):
        feature = None

    epic = event.get("epic")
    if not isinstance(epic, str):
        epic = None
    if not epic:
        epic = read_spec_epic(repo_root, feature)

    if not feature and not epic:
        return

    latest_at = non_empty_string(event.get("timestamp")) or utc_timestamp()
    state = {
        "latest_feature": feature,
        "latest_epic": epic,
        "latest_action": event.get("action") if isinstance(event.get("action"), str) else None,
        "latest_event_type": event.get("type") if isinstance(event.get("type"), str) else None,
        "latest_at": latest_at,
        "latest_review_entry": None,
        "updated_at": utc_timestamp(),
    }

    if feature:
        review = latest_review_heading(feature_dir(repo_root, feature))
        if review:
            state["latest_review_entry"] = review

    target = history_state_file(repo_root)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_name(f".{target.name}.{os.getpid()}.tmp")
    try:
        tmp.write_text(json.dumps(state, separators=(",", ":"), sort_keys=True) + "\n", encoding="utf-8")
        tmp.replace(target)
    except OSError:
        try:
            tmp.unlink()
        except OSError:
            pass


def event_dedupe_key(event: dict[str, Any]) -> str | None:
    event_type = event.get("type")
    fields_by_type = {
        "codex_session_start": ("type", "session_id", "hook_event_name", "start_source", "cwd"),
        "codex_user_prompt_submit": ("type", "session_id", "turn_id", "prompt_sha256"),
        "codex_post_tool_use": ("type", "session_id", "turn_id", "tool_use_id", "command_sha256"),
        "codex_stop": ("type", "session_id", "turn_id"),
    }
    fields = fields_by_type.get(event_type)
    if fields is None:
        return None

    values: list[Any] = []
    for field in fields:
        value = event.get(field)
        if value is None or value == "":
            return None
        values.append(value)
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

    keys: set[str] = set()
    for line in lines:
        if not line.strip():
            continue
        try:
            payload = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(payload, dict):
            continue
        key = event_dedupe_key(payload)
        if key is not None:
            keys.add(key)
    return keys


def is_recent_duplicate_event(target: Path, event: dict[str, Any]) -> bool:
    key = event_dedupe_key(event)
    if key is None:
        return False
    return key in recent_event_dedupe_keys(target)


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
        except Exception as exc:
            warn(f"failed to acquire history lock: {exc}")
            break
    if not acquired:
        warn(f"history lock unavailable; appending without lock: {lock_dir}")
    try:
        yield
    finally:
        if acquired:
            try:
                lock_dir.rmdir()
            except Exception as exc:
                warn(f"failed to release history lock: {exc}")


def append_event(repo_root: Path, event: dict[str, Any]) -> None:
    if not has_ctx_repo(repo_root):
        return

    target = history_file(repo_root)
    try:
        target.parent.mkdir(parents=True, exist_ok=True)
        with history_file_lock(target):
            rotate_history_if_needed(repo_root)
            if is_recent_duplicate_event(target, event):
                return
            write_history_state(repo_root, event)
            with target.open("a", encoding="utf-8") as fh:
                fh.write(json.dumps(event, separators=(",", ":"), sort_keys=True))
                fh.write("\n")
    except Exception as exc:
        warn(f"failed to append event: {exc}")


def load_json_file(path: Path) -> dict[str, Any]:
    content = safe_read_text(path)
    if content is None:
        return {}
    try:
        payload = json.loads(content)
    except Exception:
        warn(f"unable to parse JSON file: {path}")
        return {}
    return payload if isinstance(payload, dict) else {}


def manifest_specs(repo_root: Path) -> list[dict[str, Any]]:
    payload = load_json_file(manifest_file(repo_root))
    specs = payload.get("specs")
    if not isinstance(specs, list):
        return []
    return [spec for spec in specs if isinstance(spec, dict)]


def manifest_order(spec: dict[str, Any]) -> tuple[int, str]:
    order = spec.get("order")
    if isinstance(order, int):
        normalized_order = order
    else:
        normalized_order = sys.maxsize
    feature_id = non_empty_string(spec.get("id")) or ""
    return normalized_order, feature_id


def latest_feature_from_state(repo_root: Path) -> str | None:
    target = history_state_file(repo_root)
    if not target.is_file():
        return None
    try:
        payload = json.loads(target.read_text(encoding="utf-8"))
    except Exception:
        return None
    feature = non_empty_string(payload.get("latest_feature"))
    if feature_exists(repo_root, feature):
        return feature
    return None


def latest_feature_from_history(repo_root: Path) -> str | None:
    feature_from_state = latest_feature_from_state(repo_root)
    if feature_from_state:
        return feature_from_state

    if not has_ctx_repo(repo_root):
        return None

    target = history_file(repo_root)
    if not target.is_file():
        return None

    try:
        size = target.stat().st_size
        with target.open("rb") as fh:
            if size > 65536:
                fh.seek(size - 65536)
                fh.readline()
            lines = fh.read().decode("utf-8", errors="replace").splitlines()
    except OSError:
        return None

    for line in reversed(lines):
        if not line.strip():
            continue
        try:
            payload = json.loads(line)
        except json.JSONDecodeError:
            continue
        feature = non_empty_string(payload.get("feature"))
        if feature_exists(repo_root, feature):
            return feature
    return None


TASK_IN_PROGRESS_RE = re.compile(r"^\s*-\s+\[~\]\s+")


def task_has_in_progress(repo_root: Path, feature: str | None) -> bool:
    if not feature_exists(repo_root, feature):
        return False

    tasks_file = feature_dir(repo_root, feature or "") / "TASKS.md"
    for line in read_lines(tasks_file):
        if TASK_IN_PROGRESS_RE.match(line):
            return True
    return False


def resolve_active_feature(repo_root: Path, payload: dict[str, Any] | None = None) -> FeatureResolution:
    if not has_ctx_repo(repo_root):
        return FeatureResolution(feature=None, source="no_ctx")

    payload = payload if isinstance(payload, dict) else {}

    explicit_feature = non_empty_string(payload.get("feature"))
    if feature_exists(repo_root, explicit_feature):
        return FeatureResolution(feature=explicit_feature, source="payload_feature")

    env_feature = non_empty_string(os.getenv("CTX_ACTIVE_FEATURE"))
    if feature_exists(repo_root, env_feature):
        return FeatureResolution(feature=env_feature, source="env_feature")

    specs = manifest_specs(repo_root)
    active_specs = sorted(
        [
            spec
            for spec in specs
            if non_empty_string(spec.get("status")) == "active"
            and feature_exists(repo_root, non_empty_string(spec.get("id")))
        ],
        key=manifest_order,
    )
    active_feature_ids = {
        feature_id
        for feature_id in (non_empty_string(spec.get("id")) for spec in active_specs)
        if feature_id
    }
    active_warning = None
    if len(active_specs) > 1:
        active_ids = ",".join(non_empty_string(spec.get("id")) or "" for spec in active_specs)
        active_warning = f"multiple_active_features:{active_ids}"

    history_feature = latest_feature_from_history(repo_root)
    if len(active_specs) == 1:
        return FeatureResolution(
            feature=non_empty_string(active_specs[0].get("id")),
            source="manifest_active",
        )

    if (
        history_feature
        and task_has_in_progress(repo_root, history_feature)
        and (not active_feature_ids or history_feature in active_feature_ids)
    ):
        return FeatureResolution(
            feature=history_feature,
            source="history_doing",
            warning=active_warning,
        )

    if active_specs:
        return FeatureResolution(
            feature=non_empty_string(active_specs[0].get("id")),
            source="manifest_active",
            warning=active_warning,
        )

    if history_feature:
        return FeatureResolution(feature=history_feature, source="history")

    planned_specs = sorted(
        [
            spec
            for spec in specs
            if non_empty_string(spec.get("status")) == "planned"
            and feature_exists(repo_root, non_empty_string(spec.get("id")))
        ],
        key=manifest_order,
    )
    if planned_specs:
        return FeatureResolution(
            feature=non_empty_string(planned_specs[0].get("id")),
            source="manifest_planned",
        )

    return FeatureResolution(feature=None, source="none")


def task_summary(feature_path: Path) -> list[str]:
    tasks_file = feature_path / "TASKS.md"
    if not tasks_file.is_file():
        return []

    current_section = ""
    summary: list[str] = []
    for line in read_lines(tasks_file):
        if line.startswith("## "):
            current_section = line[3:].strip()
            continue
        if current_section in {"In Progress", "Next"} and line.startswith("- ["):
            summary.append(line.strip())
        if len(summary) >= 4:
            break
    return summary


def doc_hashes(repo_root: Path, feature: str | None) -> dict[str, str | None]:
    if not feature_exists(repo_root, feature):
        return {
            "requirements_hash": None,
            "design_hash": None,
            "tasks_hash": None,
            "reviews_hash": None,
        }

    current_feature_dir = feature_dir(repo_root, feature or "")
    return {
        "requirements_hash": file_sha256(current_feature_dir / "REQUIREMENTS.md"),
        "design_hash": file_sha256(current_feature_dir / "DESIGN.md"),
        "tasks_hash": file_sha256(current_feature_dir / "TASKS.md"),
        "reviews_hash": file_sha256(current_feature_dir / "REVIEWS.md"),
    }


def git_head(repo_root: Path) -> str | None:
    try:
        result = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=repo_root,
            check=True,
            capture_output=True,
            text=True,
        )
        head = result.stdout.strip()
        return head or None
    except Exception:
        return None


def ctx_cli_binary_name() -> str:
    return (
        non_empty_string(os.getenv("CTX_CODEX_CLI_BIN"))
        or non_empty_string(os.getenv("CTX_CLI_BIN"))
        or "ctx"
    )


def command_exists(command: str) -> bool:
    if not command:
        return False

    if "/" in command:
        path = Path(command).expanduser()
        return path.is_file() and os.access(path, os.X_OK)

    return shutil.which(command) is not None


def shadow_overview_file(repo_root: Path) -> Path:
    return ctx_dir(repo_root) / "codebase" / "OVERVIEW.md"


def shadow_guidance_lines(repo_root: Path) -> list[str]:
    if not shadow_overview_file(repo_root).is_file():
        return []

    lines = [
        "Shadow navigation:",
        "- Start with .ctx/codebase/OVERVIEW.md",
        "- Then relevant .ctx/codebase/modules/*.md, files/*.md, and symbols/*.md",
        "- Read source files before implementation or final review",
    ]

    ctx_cli = ctx_cli_binary_name()
    if command_exists(ctx_cli):
        lines.append(
            f"- Use `{ctx_cli} shadow check`, `refresh`, `watch`, or `print-config` when shadow CLI workflows are needed"
        )

    return lines


def run_hook(main: Any) -> int:
    try:
        return int(main() or 0)
    except BrokenPipeError:
        return 0
    except Exception as exc:
        warn(f"hook failed: {exc}")
        return 0
