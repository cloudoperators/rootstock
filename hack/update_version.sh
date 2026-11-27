#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

# Resolve the sibling script relative to THIS script's location, not the current
# working directory. In privileged CI the cwd is an untrusted fork checkout, so
# a relative `hack/update_changelog.sh` would execute fork-controlled code.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Target git repository whose VERSION.yaml / Chart.yaml should be updated.
# Defaults to the current working directory so existing callers keep working.
# Overridable via REPO_DIR or --repo-dir so the version can be written into a
# repository checked out in a different folder (e.g. the base_sha checkout in
# the automerge flow), while the tooling itself runs from a trusted location.
repo_dir="${REPO_DIR:-$PWD}"
# Path to the git-cliff config, forwarded to update_changelog.sh (env: CLIFF_CONFIG).
cliff_config="${CLIFF_CONFIG:-}"
# Range endpoint for history, forwarded to update_changelog.sh (env: BASE_COMMIT).
base_commit="${BASE_COMMIT:-}"

usage() {
  cat <<'EOF'
Usage: update_version.sh [OPTIONS]

Compute the next release version via update_changelog.sh and write it into
VERSION.yaml and the Helm chart's Chart.yaml of the target repository.

Options:
  --repo-dir PATH    Target git repository directory (env: REPO_DIR,
                     default: current working directory)
  --config PATH      Path to the git-cliff config file (env: CLIFF_CONFIG)
  --base-commit SHA  Range endpoint for history (env: BASE_COMMIT)
  --help             Show this help and exit
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-dir)    repo_dir="${2:-}";     shift 2 ;;
    --config)      cliff_config="${2:-}"; shift 2 ;;
    --base-commit) base_commit="${2:-}";  shift 2 ;;
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
repo_dir="$(cd "$repo_dir" && pwd)"

# Compute the next version using the trusted sibling script, pointed at the
# target repository. Pass --config / --base-commit through only when set so the
# changelog script's own defaults still apply otherwise.
changelog_args=(--repo-dir "$repo_dir" --get-version)
[[ -n "$cliff_config" ]] && changelog_args+=(--config "$cliff_config")
[[ -n "$base_commit" ]] && changelog_args+=(--base-commit "$base_commit")

next_version=$(DEBUG=false "${script_dir}/update_changelog.sh" "${changelog_args[@]}")
version="${next_version#v}"

if [[ -z "$version" ]] || ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9][a-zA-Z0-9.]*)?$ ]]; then
  echo "error: could not determine next version (got: '${next_version}')" >&2
  exit 1
fi

version_file="${repo_dir}/VERSION.yaml"
chart_file="${repo_dir}/k8s_configuration/chart/Chart.yaml"

case "$(uname)" in
  Darwin) SED_INPLACE=(-i '') ;;
  *)      SED_INPLACE=(-i) ;;
esac

sed "${SED_INPLACE[@]}" "s|^version:.*|version: ${version}|" "$version_file"
sed "${SED_INPLACE[@]}" "s|^version:.*|version: ${version}|" "$chart_file"
