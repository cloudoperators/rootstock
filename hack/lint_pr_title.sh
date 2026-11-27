#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/_helper.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Validate that a pull request title follows the Conventional Commits format
(https://www.conventionalcommits.org):

    type(optional scope)!: subject

This is a self-contained replacement for the amannn/action-semantic-pull-request
GitHub Action, which is not a verified Marketplace creator.

The title is rejected unless:
  * the type is one of the allowed types (see --types), and
  * the subject (text after the colon) is non-empty.

A scope in parentheses and a trailing "!" (breaking change) are optional.

Options:
  --title VALUE   PR title to validate                  (env: PR_TITLE)
  --types VALUE   allowed types, whitespace-separated    (env: ALLOWED_TYPES, default: "fix feat docs chore")
  --help          Show this help and exit
EOF
}

main() {
  local title="${PR_TITLE:-}"
  local types="${ALLOWED_TYPES:-fix feat docs chore}"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --title) title="$2"; shift 2 ;;
      --types) types="$2"; shift 2 ;;
      --help)  usage; exit 0 ;;
      *) printf "Unknown option: %s\n" "$1" >&2; usage; exit 1 ;;
    esac
  done

  [[ -n "$title" ]] || die "missing --title (or PR_TITLE)"

  # Build an alternation of the allowed types (e.g. "fix|feat|docs|chore").
  local type_alt=""
  local t
  for t in $types; do
    type_alt+="${type_alt:+|}${t}"
  done
  [[ -n "$type_alt" ]] || die "no allowed types configured"

  # Conventional Commits header:
  #   <type>(<optional scope>)<optional !>: <subject>
  # The subject must contain at least one non-space character.
  local regex="^(${type_alt})(\([^)]+\))?!?: +.*[^[:space:]].*$"

  if [[ "$title" =~ $regex ]]; then
    log_info "PR title is valid: \"${title}\""
    return 0
  fi

  log_error "Invalid PR title: \"${title}\""
  log_error "It must follow the Conventional Commits format: type(optional scope): subject"
  log_error "Allowed types: ${types// /, }"
  log_error "Examples: \"feat: add probe endpoint\", \"fix(redfish): handle 401\", \"chore!: drop go 1.26\""
  return 1
}

main "$@"
