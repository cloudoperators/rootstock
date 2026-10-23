#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

HC="\033[1m"
FMAG="\033[35m"
GREEN="\033[1;32m"
NC="\033[0m"

log_debug() { if [ "${DEBUG:-false}" = "true" ]; then printf "${HC}${FMAG}[.]${NC} %s\n" "$*" >&2; fi; }
log_info()  { printf "${GREEN}[+]${NC} %s\n" "$*" >&2; }

usage() {
  cat <<'EOF'
Usage: automerge_check.sh [OPTIONS]

Check whether a pull request is eligible for automerge and write the result
to GITHUB_OUTPUT (or stdout with --print).

Options:
  --pr <number>       PR number to check (overrides PR_NUMBER env var)
  --print             Write outputs to stdout instead of GITHUB_OUTPUT
  -h, --help          Show this help message

Environment variables:
  PR_NUMBER           PR number to check (required unless --pr is used)
  GITHUB_OUTPUT       Path to the GitHub Actions output file (required in CI)
  PRINT               Set to "true" to redirect outputs to stdout (default: false)
  TARGET_BRANCH       Branch PRs must target (default: main)
  BLOCKED_LABELS      Comma-separated labels that block merging
                      (default: invalid,WIP,wip,do-not-merge,DO-NOT-MERGE)
  DEBUG               Set to "true" to print eligibility diagnostics (default: false)
  GITHUB_REPOSITORY   Repository in owner/name format
                      (default: cloudoperators/rootstock)

Examples:
  # CI (GitHub Actions)
  PR_NUMBER=42 automerge_check.sh

  # Local dev — outputs printed to terminal
  automerge_check.sh --pr 42 --print
  PRINT=true PR_NUMBER=42 automerge_check.sh
EOF
}

main() {
  parse_inputs "$@"

  if [ -z "${PR_NUMBER:-}" ]; then
    log_debug "PR_NUMBER is empty - aborting"
    echo "eligible=false" >> "$GITHUB_OUTPUT"
    return
  fi

  log_info "Checking PR #${PR_NUMBER} for automerge eligibility"
  fetch_pr "$PR_NUMBER"
  fetch_reviews "$PR_NUMBER"

  if check_eligibility; then
    log_info "PR #${PR_NUMBER} is eligible for automerge"
    emit_outputs
  else
    log_info "PR #${PR_NUMBER} is not eligible for automerge"
    echo "eligible=false" >> "$GITHUB_OUTPUT"
  fi
}


parse_inputs() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help)           usage; exit 0 ;;
      --print)             GITHUB_OUTPUT=/dev/stdout; shift ;;
      --pr)                PR_NUMBER="$2"; shift 2 ;;
      --pr=*)              PR_NUMBER="${1#*=}"; shift ;;
      *) echo "error: unknown flag: $1" >&2; echo >&2; usage >&2; exit 1 ;;
    esac
  done

  [[ "${PRINT:-false}" == "true" ]] && GITHUB_OUTPUT=/dev/stdout

  TARGET_BRANCH="${TARGET_BRANCH:-main}"
  BLOCKED_LABELS="${BLOCKED_LABELS:-"invalid,WIP,wip,do-not-merge,DO-NOT-MERGE"}"
  DEBUG="${DEBUG:-false}"
  GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-cloudoperators/rootstock}"
}

# ---------------------------------------------------------------------------
# fetch_pr <pr_number>: populates globals PR and PR_BODY
# ---------------------------------------------------------------------------
fetch_pr() {
  local pr_number="$1"
  PR=$(gh pr view "$pr_number" --repo "$GITHUB_REPOSITORY" \
    --json number,title,headRefName,baseRefName,baseRefOid,mergeStateStatus,url,isDraft,labels,reviewRequests)
  PR_BODY=$(gh pr view "$pr_number" --repo "$GITHUB_REPOSITORY" \
    --json body | jq -r '.body // ""')
}

# ---------------------------------------------------------------------------
# fetch_reviews <pr_number>: populates global REVIEWS as [{login, state}],
# keeping only the latest review per reviewer
# ---------------------------------------------------------------------------
fetch_reviews() {
  local pr_number="$1"
  REVIEWS=$(gh api "repos/${GITHUB_REPOSITORY}/pulls/${pr_number}/reviews" \
    --paginate \
    | jq -s 'add | map(select(.state != "COMMENTED" and .state != "PENDING")) | group_by(.user.login) | map(max_by(.submitted_at)) | map({login: .user.login, state})'
  )
}

# ---------------------------------------------------------------------------
# has_codeowner_approved: returns 0 if a codeowner's latest review is APPROVED
# Reads individual @users and @org/teams from .github/CODEOWNERS
# ---------------------------------------------------------------------------
has_codeowner_approved() {
  local codeowners_raw teams approved_logins login team org slug

  codeowners_raw=$(gh api "repos/${GITHUB_REPOSITORY}/contents/.github/CODEOWNERS" \
    --jq '.content' | base64 -d)
  teams=$(printf '%s' "$codeowners_raw" \
    | grep -v '^#' | grep -v '^[[:space:]]*$' \
    | grep -oE '@[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+' | sed 's/@//')
  allowed_approvers=$(printf '%s' "$codeowners_raw" \
    | grep -v '^#' | grep -v '^[[:space:]]*$' \
    | sed 's/@[a-zA-Z0-9_.-]*\/[a-zA-Z0-9_.-]*//g' \
    | grep -oE '@[a-zA-Z0-9_.-]+' | sed 's/@//')

  approved_logins=$(echo "$REVIEWS" \
    | jq -r '[.[] | select(.state == "APPROVED") | .login] | .[]')

  for login in $approved_logins; do
    if printf '%s\n' "$allowed_approvers" | grep -qx "$login"; then
      return 0
    fi
    for team in $teams; do
      org="${team%%/*}"
      slug="${team##*/}"
      if gh api "orgs/${org}/teams/${slug}/memberships/${login}" \
          --silent >/dev/null 2>&1; then
        return 0
      fi
    done
  done
  return 1
}

# ---------------------------------------------------------------------------
# check_eligibility: evaluates all conditions and returns 0 (eligible) or 1 (not)
# ---------------------------------------------------------------------------
check_eligibility() {
  local base state draft blocked_label pending_reviews changes_requested codeowner_approved eligible

  base=$(echo "$PR" | jq -r .baseRefName)
  state=$(echo "$PR" | jq -r .mergeStateStatus)
  draft=$(echo "$PR" | jq -r .isDraft)
  blocked_label=$(echo "$PR" | jq -r --arg bl "$BLOCKED_LABELS" '
    ($bl | split(",")) as $list | .labels[].name | select(IN($list[]))
  ' | head -1)
  pending_reviews=$(echo "$PR" | jq -r '
    [.reviewRequests[] | select(.login != null) | .login | select(test("copilot"; "i") | not)] | length'
  )
  changes_requested=$(echo "$REVIEWS" \
    | jq '[.[] | select(.state == "CHANGES_REQUESTED")] | length')

  if has_codeowner_approved; then codeowner_approved="true"
  else                            codeowner_approved="false"
  fi

  eligible="true"

  if [ "$base" != "$TARGET_BRANCH" ] \
    || [ "$state" != "CLEAN" ] \
    || [ "$changes_requested" -gt 0 ] \
    || [ "$codeowner_approved" != "true" ] \
    || [ "$pending_reviews" -gt 0 ] \
    || [ "$draft" = "true" ] \
    || [ -n "$blocked_label" ]; then
    eligible="false"
  fi

  debug_eligibility "$eligible" "$base" "$state" "$changes_requested" \
    "$codeowner_approved" "$pending_reviews" "$draft" "$blocked_label"

  if [[ "$eligible" == "true" ]]; then
    return 0
  else
    return 1
  fi
}

# ---------------------------------------------------------------------------
# debug_eligibility <eligible> <base> <state> <changes_requested>
#                   <codeowner_approved> <pending_reviews> <draft> <blocked_label>
# Prints all eligibility conditions; no-ops when DEBUG != true
# ---------------------------------------------------------------------------
debug_eligibility() {
  local eligible="$1" base="$2" state="$3" changes_requested="$4" \
        codeowner_approved="$5" pending_reviews="$6" draft="$7" blocked_label="$8"

  log_debug "eligible=$eligible"
  log_debug "pr_number=$(echo "$PR" | jq -r .number)"
  if [ "$eligible" = "true" ]; then
    log_debug "pr_title=$(echo "$PR" | jq -r .title)"
    log_debug "pr_url=$(echo "$PR" | jq -r .url)"
    log_debug "head_ref=$(echo "$PR" | jq -r .headRefName)"
    log_debug "base_sha=$(echo "$PR" | jq -r .baseRefOid)"
  fi
  log_debug "base=$base ?= $TARGET_BRANCH"
  log_debug "merge_state_status=$state ?= NOT CLEAN"
  log_debug "changes_requested=$changes_requested ?= 0"
  log_debug "codeowner_approved=$codeowner_approved ?= true"
  log_debug "pending_reviews=$pending_reviews ?= 0"
  log_debug "is_draft=$draft ?= true"
  log_debug "blocked_label=${blocked_label:-<none>}"
  log_debug "pr_body=$PR_BODY"
}

# ---------------------------------------------------------------------------
# emit_outputs: writes eligible=true and all PR fields to GITHUB_OUTPUT
# ---------------------------------------------------------------------------
emit_outputs() {
  {
    echo "eligible=true"
    echo "pr_number=$(echo "$PR" | jq -r .number)"
    echo "pr_title=$(echo "$PR" | jq -r .title)"
    echo "pr_url=$(echo "$PR" | jq -r .url)"
    echo "head_ref=$(echo "$PR" | jq -r .headRefName)"
    echo "base_sha=$(echo "$PR" | jq -r .baseRefOid)"
    printf 'pr_body<<__BODY__\n'
    printf '%s\n' "$PR_BODY"
    printf '__BODY__\n'
  } >> "$GITHUB_OUTPUT"
}

main "$@"
