#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

# ---------------------------------------------------------------------------
# Consistent changelog generation.
#
# The CHANGELOG is always regenerated in full (never appended to) from a fixed
# start commit up to the current tip of `main`. This guarantees that:
#   * the changelog reflects all commits that are on main,
#   * all commits at the HEAD of main are assumed to enter the next release
#     (we always run with `--bump`, so untagged commits are grouped under the
#     computed next version instead of a floating "[Unreleased]" section),
#   * for a new commit to main, all previous entries that don't have a release
#     tag yet are updated (a full regeneration re-labels every untagged entry
#     under the recomputed next version).
#
# During the automerge flow the squash-merge commit does not exist yet, so its
# message is injected as a virtual commit via COMMIT_MSG / --commit-msg and the
# range endpoint (BASE_COMMIT / --base-commit) is the current tip of main.
# ---------------------------------------------------------------------------

# The commit from which the changelog history starts. Overridable via
# CHANGELOG_START_COMMIT so the script can be exercised against test fixtures.
changelog_start_commit="${CHANGELOG_START_COMMIT:-8585d9f2727bfe5d1bf91074340b20bcc7eecaad}"

commit_msg="${COMMIT_MSG:-}"
base_commit="${BASE_COMMIT:-}"
debug="${DEBUG:-false}"
print_version="false"
# Directory of the target git repository. Defaults to the current working
# directory so existing callers (which `cd` into the repo) keep working.
# Overridable via REPO_DIR or --repo-dir so the script can regenerate the
# changelog for a repository checked out in a different folder.
repo_dir="${REPO_DIR:-$PWD}"
# Path to the git-cliff configuration. Defaults to CLIFF_CONFIG (already honored
# by the original script) and may also be set via --config. When empty, git-cliff
# discovers the config automatically from the working tree.
cliff_config="${CLIFF_CONFIG:-}"

usage() {
  cat <<'EOF'
Usage: update_changelog.sh [OPTIONS]

Regenerate CHANGELOG.md (or print the next bumped version) for a git repository
using git-cliff.

Options:
  --repo-dir PATH    Target git repository directory (env: REPO_DIR,
                     default: current working directory)
  --config PATH      Path to the git-cliff config file, e.g. cliff.toml
                     (env: CLIFF_CONFIG, default: git-cliff auto-discovery)
  --commit-msg MSG   Inject MSG as a virtual commit (env: COMMIT_MSG)
  --base-commit SHA  Range endpoint for history (env: BASE_COMMIT,
                     default: tip of main)
  --get-version      Print the next bumped version instead of writing CHANGELOG.md
  --debug            Print the resolved git-cliff invocation (env: DEBUG=true)
  --help             Show this help and exit
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-dir)    repo_dir="${2:-}";    shift 2 ;;
    --config)      cliff_config="${2:-}"; shift 2 ;;
    --commit-msg)  commit_msg="${2:-}";  shift 2 ;;
    --base-commit) base_commit="${2:-}"; shift 2 ;;
    --debug)       debug=true;           shift ;;
    --get-version) print_version=true;   shift ;;
    --help)        usage; exit 0 ;;
    *) echo "error: unknown parameter: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if [[ -z "$repo_dir" ]]; then
  echo "error: --repo-dir must not be empty" >&2
  exit 1
fi
if [[ ! -d "$repo_dir" ]]; then
  echo "error: repository directory not found: ${repo_dir}" >&2
  exit 1
fi
# Normalize to an absolute path so --output and --repository are unambiguous
# regardless of the caller's current working directory.
repo_dir="$(cd "$repo_dir" && pwd)"

# Determine the range endpoint: the current tip of main.
#
# * On main (release flow) HEAD already is the tip of main.
# * On a PR branch (automerge flow) the tip of main is passed explicitly via
#   BASE_COMMIT (the PR base sha); the new commit is supplied via COMMIT_MSG.
#   As a fallback we resolve origin/main directly so newer main commits are
#   included without including commits unique to the PR branch.
if [[ "$base_commit" == "" ]]; then
  if [ "$(git -C "$repo_dir" rev-parse --abbrev-ref HEAD)" = "main" ]; then
    base_commit=$(git -C "$repo_dir" rev-parse HEAD)
  else
    base_commit=$(git -C "$repo_dir" rev-parse --verify 'origin/main^{commit}' 2>/dev/null) || {
      echo "error: origin/main not found; fetch main or pass --base-commit" >&2
      exit 1
    }
    echo "warning: using origin/main as range endpoint; run 'git fetch' if it may be stale" >&2
  fi
fi

# Always bump: everything not behind a release tag belongs to the next release.
cliff_args=(--bump)

# Point git-cliff at the target repository so history is read from there and,
# together with an absolute --output path below, the script works regardless of
# the caller's current working directory.
cliff_args+=(--repository "${repo_dir}")

# Allow forcing a specific git-cliff config. In privileged CI the changelog is
# generated against an untrusted (fork) checkout, so the config must be pinned
# to a trusted copy rather than auto-discovered from the working tree (a
# fork-controlled cliff.toml can run arbitrary commands via postprocessors).
if [[ -n "${cliff_config}" ]]; then
  cliff_args+=(--config "${cliff_config}")
fi

if $print_version; then
  cliff_args+=(--bumped-version)
else
  # Write CHANGELOG.md inside the target repository (absolute path), not the
  # caller's cwd.
  cliff_args+=(--output "${repo_dir}/CHANGELOG.md")
fi

if [[ "$commit_msg" != "" ]]; then
  cliff_args+=(--with-commit "$(printf '%s' "${commit_msg}")")
fi
cliff_args+=("${changelog_start_commit}^..${base_commit}")

[[ "$debug" == true ]] && echo "git cliff ${cliff_args[*]}"
git cliff "${cliff_args[@]}"
