#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

changelog_start_commit=4251bdc5a5693fa817ae6859bfab31a2be4a97bd

commit_msg="${COMMIT_MSG:-}"
base_commit="${BASE_COMMIT:-}"
debug="${DEBUG:-false}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --commit-msg)  commit_msg="${2:-}";  shift 2 ;;
    --base-commit) base_commit="${2:-}"; shift 2 ;;
    --debug)       debug=true;           shift ;;
    *) echo "error: unknown parameter: $1" >&2; exit 1 ;;
  esac
done

if [[ "$commit_msg" != "" ]]; then
  if [[ "$base_commit" == "" ]]; then
    base_commit=$(git rev-parse HEAD)
  fi
else
  if [[ "$base_commit" == "" ]]; then
    if [ "$(git rev-parse --abbrev-ref HEAD)" = "main" ]; then
      base_commit=$(git rev-parse HEAD)
    else
      base_commit=$(git merge-base HEAD origin/main 2>/dev/null) || {
        echo "error: no common ancestor with origin/main found; ensure the branch is based on main" >&2
        exit 1
      }
    fi
  fi
fi

cliff_args=(--output CHANGELOG.md --bump)
if [[ "$commit_msg" != "" ]]; then
  cliff_args+=(--with-commit "$(printf '%s' "${commit_msg}")")
fi
cliff_args+=("${changelog_start_commit}..${base_commit}")

$debug && echo "git cliff ${cliff_args[*]}"
git cliff "${cliff_args[@]}"