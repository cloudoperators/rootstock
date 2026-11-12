#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

HC="\033[1m"
FMAG="\033[35m"
GREEN="\033[1;32m"
YELLOW="\033[1;33m"
RED="\033[1;31m"
NC="\033[0m"

log_debug() { if [ "${DEBUG:-false}" = "true" ]; then printf "${HC}${FMAG}[.]${NC} %s\n" "$*" >&2; fi; }
log_info()  { printf "${GREEN}[+]${NC} %s\n" "$*" >&2; }
log_warn()  { printf "${YELLOW}[!]${NC} %s\n" "$*" >&2; }
log_error() { printf "${RED}[error]${NC} %s\n" "$*" >&2; }

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Validates that the Helm chart version is bumped in lockstep with chart changes.

Compares the working tree against the merge base of BASE_SHA and HEAD_SHA and
enforces that:
  * if any file under the chart folder (excluding Chart.yaml) changed, the
    '.version' field in Chart.yaml must have changed too;
  * if no such file changed, the '.version' field must not have changed.

Options:
  --base-sha VALUE   Commit to diff against (env: BASE_SHA)
  --head-sha VALUE   HEAD commit to diff to (env: HEAD_SHA, default: HEAD)
  --help             Show this help and exit
EOF
}

main() {
  local base_sha="${BASE_SHA:-}"
  local head_sha="${HEAD_SHA:-HEAD}"
  local chart_dir="k8s_configuration/chart"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --base-sha)  base_sha="$2";  shift 2 ;;
      --head-sha)  head_sha="$2";  shift 2 ;;
      --help)      usage; exit 0 ;;
      *)           log_error "Unknown option: $1"; usage; exit 1 ;;
    esac
  done

  if [[ -z "$base_sha" ]]; then
    log_error "base sha is required (--base-sha or BASE_SHA)"
    exit 1
  fi

  local chart_file="${chart_dir}/Chart.yaml"

  validate_version_bump "$base_sha" "$head_sha" "$chart_dir" "$chart_file"
}

# chart_changes prints the files changed under the chart folder, excluding
# Chart.yaml itself, between the merge base of base_sha..head_sha and head_sha.
chart_changes() {
  local base_sha="$1" head_sha="$2" chart_dir="$3" chart_file="$4"
  git diff --name-only "${base_sha}"..."${head_sha}" -- "${chart_dir}" ":(exclude)${chart_file}"
}

# version_changed returns 0 if the '.version' field in Chart.yaml changed.
version_changed() {
  local base_sha="$1" head_sha="$2" chart_file="$3"
  git diff "${base_sha}"..."${head_sha}" -- "${chart_file}" | grep -qE '^[-+]version:'
}

# chart_change_refs prints a 'file:line' reference for every changed hunk in
# the chart folder (excluding Chart.yaml). The line number is the first line of
# the hunk on the new (head_sha) side of the diff.
chart_change_refs() {
  local base_sha="$1" head_sha="$2" chart_dir="$3" chart_file="$4"
  git diff --unified=0 "${base_sha}"..."${head_sha}" -- "${chart_dir}" ":(exclude)${chart_file}" \
    | awk '
        /^\+\+\+ b\// { file = substr($0, 7); next }
        /^@@/ {
          # hunk header: @@ -old +new @@ ; extract the new-side start line.
          split($3, a, ",")
          line = a[1]
          sub(/^\+/, "", line)
          if (file != "") print file ":" line
        }
      '
}

validate_version_bump() {
  local base_sha="$1" head_sha="$2" chart_dir="$3" chart_file="$4"

  local changes
  changes=$(chart_changes "$base_sha" "$head_sha" "$chart_dir" "$chart_file")

  if [[ -n "$changes" ]]; then
    log_info "Detected changes in ${chart_dir} (excluding Chart.yaml):"
    log_info "${changes}"
    if ! version_changed "$base_sha" "$head_sha" "$chart_file"; then
      log_error "$(printf "${RED}%s %s${NC}" \
          "Chart contents changed but '.version' in ${chart_file} was not updated." \
          "Bump the chart version to the correct semantic version."
      )"
      log_error "Changed locations:"
      local ref
      while IFS= read -r ref; do
        [[ -n "$ref" ]] && log_error "  ${ref}"
      done < <(chart_change_refs "$base_sha" "$head_sha" "$chart_dir" "$chart_file")
      exit 1
    fi
    log_info "Chart version was updated as expected."
  else
    log_info "No changes in ${chart_dir} (excluding Chart.yaml)."
    if version_changed "$base_sha" "$head_sha" "$chart_file"; then
      log_error "'.version' in ${chart_file} changed but no chart contents changed. Revert the version bump."
      exit 1
    fi
    log_info "Chart version unchanged as expected."
  fi
}

main "$@"
