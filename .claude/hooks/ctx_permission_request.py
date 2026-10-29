#!/usr/bin/env python3
from __future__ import annotations

import os
import shutil

from ctx_hook_utils import non_empty_string, run_hook


def main() -> int:
    binary = non_empty_string(os.getenv("CTX_BIN")) or shutil.which("ctx")
    if not binary:
        return 0
    mode = non_empty_string(os.getenv("CTX_WORKBENCH_CLAUDE_HOOK_MODE")) or "broker"
    os.execvpe(
        binary,
        [binary, "workbench", "hook", "claude", "permission-request", "--mode", mode],
        os.environ,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(run_hook(main))
