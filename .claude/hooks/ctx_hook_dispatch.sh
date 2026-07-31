#!/bin/sh

set +e

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" 2>/dev/null && pwd)
script_name=$1

if [ -z "$script_dir" ] || [ -z "$script_name" ]; then
    exit 0
fi

script_path="$script_dir/$script_name"
if [ ! -f "$script_path" ]; then
    exit 0
fi

if ! command -v python3 >/dev/null 2>&1; then
    echo "ctx hook: python3 not found for $script_name" >&2
    exit 0
fi

exec python3 "$script_path"
