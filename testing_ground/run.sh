#!/usr/bin/env bash

# SiteCheck Testing Ground — Visual Test Orchestrator
#   ./testing_ground/run.sh setup   — Prime DB.
#   ./testing_ground/run.sh         — Do a test run.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GROUND="$ROOT/testing_ground"
WORK="$GROUND/work"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

TEST_HTTP_PORT=19976
TEST_REMOTE_PORT=19977
TEST_DEAD_PORT=19978

_pid_file() { echo "$WORK/.pid.$1"; }

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

setup_ground() {
    log_info "=== $1 ==="
    for port in "$TEST_HTTP_PORT" "$TEST_REMOTE_PORT" "$TEST_DEAD_PORT"; do
        local pid
        pid=$(lsof -ti ":$port" 2>/dev/null || true)
        [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
    done
    mkdir -p "$WORK/data" "$WORK/output"
}

start_test_servers() {
    python3 "$GROUND/servers/test_http_server.py" --port "$TEST_HTTP_PORT" &
    echo "$!" > "$(_pid_file http)"
    for i in $(seq 1 30); do
        if curl -s "http://127.0.0.1:$TEST_HTTP_PORT/ok" >/dev/null 2>&1; then
            log_info "Test HTTP server ready on :$TEST_HTTP_PORT"
            return 0
        fi
        sleep 0.1
    done
    log_error "Test HTTP server failed to start"
    return 1
}

start_remote_outpost() {
    local resources="$1" port="${2:-$TEST_REMOTE_PORT}" token="${3:-test-remote-token}" name="${4:-remote_outpost}"
    SITECHECK_TOKEN="$token" \
    SITECHECK_RESOURCES_DIR="$resources" \
    SITECHECK_WORKERS="2" \
    SITECHECK_LISTEN=":$port" \
    SITECHECK_DEFAULT_TIMEOUT="10" \
        "$ROOT/scoutpost" &
    echo "$!" > "$(_pid_file "$name")"
    for i in $(seq 1 50); do
        if curl -s -H "Authorization: Bearer $token" "http://127.0.0.1:$port/" >/dev/null 2>&1; then
            log_info "Remote outpost '$name' ready on :$port"
            return 0
        fi
        sleep 0.1
    done
    log_error "Remote outpost '$name' failed to start"
    return 1
}

stop_test_servers() {
    for name in http remote_outpost doomed; do
        local pf="$(_pid_file "$name")"
        if [[ -f "$pf" ]]; then
            local pid=$(cat "$pf")
            kill -0 "$pid" 2>/dev/null && { kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; }
            rm -f "$pf"
        fi
    done
    for port in "$TEST_HTTP_PORT" "$TEST_REMOTE_PORT" "$TEST_DEAD_PORT"; do
        local pid=$(lsof -ti ":$port" 2>/dev/null || true)
        [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
    done
}

build_binaries() {
    log_info "Building binaries..."
    ( cd "$ROOT" && go build -o sitecheck ./cmd/sitecheck && go build -o scoutpost ./cmd/scoutpost )
    log_info "Binaries ready."
}

_run_sc() {
    local config="$1" resources_dir="$2"
    (
        cd "$ROOT"
        set -a; source "$config"; set +a
        SITECHECK_RESOURCES_DIR="$resources_dir" \
        TEST_HTTP_PORT="$TEST_HTTP_PORT" \
        TEST_REMOTE_PORT="$TEST_REMOTE_PORT" \
        TEST_DEAD_PORT="$TEST_DEAD_PORT" \
            ./sitecheck 2>&1 || true
    )
}

scenario_setup() {
    setup_ground "Setup — All Outposts Up"
    build_binaries
    start_test_servers
    start_remote_outpost "$GROUND/resources/doomed"  "$TEST_DEAD_PORT"  "doomed-token"      "doomed"
    start_remote_outpost "$GROUND/resources/survivor"
    log_info "Running: all outposts up, all resources..."
    _run_sc "$GROUND/configs/multi.env" "$GROUND/resources/local"
    stop_test_servers
    log_info "Output in $WORK/output/ — DB primed for test runs."
}

scenario_test() {
    setup_ground "Test — All Behaviors"
    if [[ ! -f "$WORK/data/sitecheck.db" ]]; then
        log_error "No database found — run './testing_ground/run.sh setup' first."
        exit 1
    fi
    build_binaries
    start_test_servers
    start_remote_outpost "$GROUND/resources/survivor"
    log_info "Running: mixed local + survivor remote + doomed down..."
    _run_sc "$GROUND/configs/multi.env" "$GROUND/resources/local"
    stop_test_servers
    log_info "Output left in $WORK/output/ for inspection."
}

main() {
    local scenario="${1:-test}"
    echo ""
    echo "============================================"
    echo "  SiteCheck Testing Ground"
    echo "============================================"
    echo ""
    case "$scenario" in
        setup) scenario_setup ;;
        test)  scenario_test ;;
        *)
            log_error "Unknown scenario: $scenario"
            echo "Usage: $0 [setup|test]"
            exit 1 ;;
    esac
}

main "$@"
