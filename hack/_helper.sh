# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
# shellcheck shell=bash
#
# _helper.sh — reusable shell helpers for scripts under hack/.
#
# This file is meant to be *sourced*, not executed. It provides the shared
# logging helpers (log_debug/log_info/log_warn/log_error), a `die` convenience,
# and the command runners `_exec` / `_exec_output`.
#
# Usage (from any hack/ script):
#   source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../_helper.sh"
#
# The guard below makes repeated sourcing a no-op, so scripts that source each
# other (e.g. greenhouse/plugin_control.sh sourcing greenhouse/control.sh) do
# not redefine these functions.

if [ -n "${_ROOTSTOCK_HELPER_SH:-}" ]; then
    return 0
fi
_ROOTSTOCK_HELPER_SH=1

# --- Color constants ----------------------------------------------------------
# Use $'...' so these hold the actual ESC bytes. That way they render correctly
# whether interpolated into a printf *format* string or passed as a printf %s
# *argument* (printf only interprets backslash escapes in the format, not in
# %s arguments).
HC=$'\033[1m'
FMAG=$'\033[35m'
GREEN=$'\033[1;32m'
YELLOW=$'\033[1;33m'
RED=$'\033[1;31m'
NC=$'\033[0m'

# --- Logging helpers ----------------------------------------------------------
# All logging goes to stderr so stdout stays clean for capturable output.
log_debug() { if [ "${DEBUG:-false}" = "true" ]; then printf "${HC}${FMAG}[.]${NC} %s\n" "$*" >&2; fi; }
log_info()  { printf "${GREEN}[+]${NC} %s\n" "$*" >&2; }
log_warn()  { printf "${YELLOW}[!]${NC} %s\n" "$*" >&2; }
log_error() { printf "${RED}[error]${NC} %s\n" "$*" >&2; }
die()       { log_error "$*"; exit 1; }

# --- Command runners ----------------------------------------------------------

# _exec runs the given command and, on failure, prints the failed command in
# red before propagating the non-zero exit status. When DEBUG=true, it also
# logs the command before running it (via log_debug).
#
# Two call styles are supported:
#   - Multiple arguments: the command is executed directly (no shell), so
#     arguments are passed verbatim without word-splitting or glob expansion.
#       _exec kubectl apply -f "$file"
#   - A single argument: the string is run through `bash -c`, enabling shell
#     features such as pipes, redirects, and command substitution.
#       _exec "kubectl get pods | grep rootstock >/dev/null 2>&1"
_exec() {
    local status=0
    log_debug "exec: $*"
    if [[ "$#" -eq 1 ]]; then
        bash -c "$1" || status=$?
    else
        "$@" || status=$?
    fi
    if [[ "${status}" -ne 0 ]]; then
        log_error "command failed (exit ${status}): ${RED}$*${NC}"
        return "${status}"
    fi
}

# _exec_output runs a command via _exec but captures its stdout into the
# variable named by the first argument. Usage: _exec_output VAR cmd [args...]
# As with _exec, a single trailing argument is run through `bash -c`, enabling
# pipes/redirects/command substitution:
#   _exec_output NS "echo '$doc' | yq -r '.metadata.namespace'"
# On failure it prints the failed command (via _exec) and propagates the
# non-zero exit status.
_exec_output() {
    local __var="$1"
    shift

    local __output __status=0
    __output="$(_exec "$@")" || __status=$?

    if [[ "${__status}" -ne 0 ]]; then
        return "${__status}"
    fi

    printf -v "${__var}" '%s' "${__output}"
}
