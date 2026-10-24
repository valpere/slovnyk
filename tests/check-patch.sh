#!/bin/sh
# Check active patches; historical evidence is preserved byte-for-byte.
set -eu
cd "$(dirname "$0")/.."
if [ "$#" -gt 1 ]; then
    echo 'usage: check-patch.sh [base-commit]' >&2
    exit 2
fi
if [ "$#" -eq 1 ]; then
    case "$1" in
        ''|0000000000000000000000000000000000000000)
            # Initial branch push or workflow_dispatch: check the HEAD commit.
            git show --format= --check HEAD -- . ':(exclude)docs/_assessments/**'
            ;;
        *)
            base=$(git rev-parse --verify --end-of-options "$1^{commit}")
            git diff --check "$base" HEAD -- . ':(exclude)docs/_assessments/**'
            ;;
    esac
fi
# Check both staged and unstaged work, including when no CI base was provided.
git diff --cached --check -- . ':(exclude)docs/_assessments/**'
git diff --check -- . ':(exclude)docs/_assessments/**'
