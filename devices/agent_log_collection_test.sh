#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
FIXTURE_DIR="$SCRIPT_DIR/test-fixtures"

run_collection() {
    PATH="$FIXTURE_DIR:$PATH" AGENT_LOGREAD_TEST_MODE="$1" \
        sh "$SCRIPT_DIR/agent.sh" --self-test-log-collection
}

healthy=$(run_collection healthy)
case "$healthy" in
    *"healthy log"*) ;;
    *)
        echo "healthy log collection failed: $healthy" >&2
        exit 1
        ;;
esac

started_at=$(date +%s)
blocked=$(run_collection block)
elapsed=$(( $(date +%s) - started_at ))

if [ "$elapsed" -gt 4 ]; then
    echo "blocked log collection exceeded watchdog: ${elapsed}s" >&2
    exit 1
fi

if [ -n "$blocked" ]; then
    echo "blocked log collection returned unexpected output: $blocked" >&2
    exit 1
fi

echo "agent log collection watchdog passed (${elapsed}s blocked case)"
