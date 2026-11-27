#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/_helper.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Delete container image versions of a GHCR package via the GitHub REST API.

This is a self-contained replacement for the dataaxiom/ghcr-cleanup-action
GitHub Action: it lists the package versions with the "gh" CLI and deletes the
ones whose tags match --delete-tag / --delete-tag-glob and/or that are untagged.

It requires the "gh" CLI authenticated with a token that has "packages: write"
on the owning organization (the standard GITHUB_TOKEN with
"permissions: packages: write" is sufficient).

Options:
  --owner VALUE        organization that owns the package (env: OWNER, default: \$GITHUB_REPOSITORY_OWNER)
  --package VALUE      container package name           (env: PACKAGE, default: repo name)
  --delete-tag VALUE   delete versions carrying this exact tag       (env: DELETE_TAG)
  --delete-tag-glob V  delete versions carrying a tag matching glob V (env: DELETE_TAG_GLOB)
  --delete-untagged    also delete versions that have no tags         (env: DELETE_UNTAGGED=true)
  --dry-run            print what would be deleted without deleting   (env: DRY_RUN=true)
  --help               Show this help and exit

At least one of --delete-tag, --delete-tag-glob, or --delete-untagged is required.
EOF
}

main() {
  local owner="${OWNER:-${GITHUB_REPOSITORY_OWNER:-}}"
  local package="${PACKAGE:-}"
  local delete_tag="${DELETE_TAG:-}"
  local delete_glob="${DELETE_TAG_GLOB:-}"
  local delete_untagged="${DELETE_UNTAGGED:-false}"
  local dry_run="${DRY_RUN:-false}"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --owner)           owner="$2"; shift 2 ;;
      --package)         package="$2"; shift 2 ;;
      --delete-tag)      delete_tag="$2"; shift 2 ;;
      --delete-tag-glob) delete_glob="$2"; shift 2 ;;
      --delete-untagged) delete_untagged="true"; shift ;;
      --dry-run)         dry_run="true"; shift ;;
      --help)            usage; exit 0 ;;
      *) printf "Unknown option: %s\n" "$1" >&2; usage; exit 1 ;;
    esac
  done

  # Default the package name to the repository name (owner/repo -> repo).
  if [[ -z "$package" && -n "${GITHUB_REPOSITORY:-}" ]]; then
    package="${GITHUB_REPOSITORY##*/}"
  fi

  [[ -n "$owner" ]]   || die "missing --owner (or GITHUB_REPOSITORY_OWNER)"
  [[ -n "$package" ]] || die "missing --package (or GITHUB_REPOSITORY)"
  command -v gh >/dev/null 2>&1 || die 'this script requires the "gh" CLI'

  if [[ -z "$delete_tag" && -z "$delete_glob" && "$delete_untagged" != "true" ]]; then
    die "nothing to do: pass --delete-tag, --delete-tag-glob, and/or --delete-untagged"
  fi

  local versions
  versions="$(list_versions "$owner" "$package")"

  local ids
  ids="$(select_version_ids "$versions" "$delete_tag" "$delete_glob" "$delete_untagged")"

  if [[ -z "$ids" ]]; then
    log_info "No matching package versions to delete in ${owner}/${package}"
    return 0
  fi

  delete_versions "$owner" "$package" "$dry_run" "$ids"
}

# list_versions prints the full JSON array of container versions for the package.
list_versions() {
  local owner="$1" package="$2"
  gh api --paginate "/orgs/${owner}/packages/container/${package}/versions?per_page=100" \
    --jq '.[] | {id, tags: (.metadata.container.tags // [])}' \
    | jq -s '.'
}

# select_version_ids filters the versions JSON by the selection criteria and
# prints the IDs to delete, one per line. Matching is done in jq: a version is
# selected if it is untagged (and --delete-untagged is set), carries the exact
# --delete-tag, or carries a tag matching the --delete-tag-glob shell glob
# (translated to a jq regex: '*' -> '.*', '?' -> '.').
select_version_ids() {
  local versions="$1" tag="$2" glob="$3" untagged="$4"
  local regex=""
  if [[ -n "$glob" ]]; then
    regex="$(glob_to_regex "$glob")"
  fi

  printf '%s' "$versions" | jq -r \
    --arg tag "$tag" \
    --arg regex "$regex" \
    --argjson untagged "$([[ "$untagged" == "true" ]] && echo true || echo false)" \
    '.[]
     | select(
         ($untagged and (.tags | length) == 0)
         or ($tag   != "" and (.tags | index($tag)) != null)
         or ($regex != "" and (.tags | any(test("^" + $regex + "$"))))
       )
     | .id'
}

# glob_to_regex converts a simple shell glob (supporting '*' and '?') into a
# regular expression body. All other regex metacharacters are escaped so the
# resulting pattern matches the glob literally.
glob_to_regex() {
  local glob="$1" out="" ch i
  for ((i = 0; i < ${#glob}; i++)); do
    ch="${glob:i:1}"
    case "$ch" in
      '*') out+='.*' ;;
      '?') out+='.' ;;
      '.'|'+'|'('|')'|'['|']'|'{'|'}'|'^'|'$'|'|') out+="\\${ch}" ;;
      $'\\') out+="\\\\" ;;
      *) out+="$ch" ;;
    esac
  done
  printf '%s' "$out"
}

# delete_versions deletes each given version ID (or logs it on dry-run).
delete_versions() {
  local owner="$1" package="$2" dry_run="$3" ids="$4"
  local count id
  count="$(printf '%s\n' "$ids" | grep -c . || true)"
  log_info "Deleting ${count} package version(s) of ${owner}/${package}"
  while IFS= read -r id; do
    [[ -n "$id" ]] || continue
    if [[ "$dry_run" == "true" ]]; then
      log_info "[dry-run] would delete version ${id}"
      continue
    fi
    log_info "Deleting version ${id}"
    _exec gh api -X DELETE "/orgs/${owner}/packages/container/${package}/versions/${id}"
  done <<< "$ids"
}

main "$@"
