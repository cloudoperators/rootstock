#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/_helper.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Post a Go code coverage report as a pull request comment, comparing the current
coverage against the baseline coverage of the target branch.

This is a self-contained replacement for the fgrosse/go-coverage-report GitHub
Action: it installs the go-coverage-report CLI via "go install", downloads the
baseline coverage artifact of the latest successful baseline-workflow run on the
target branch via the "gh" CLI, builds the list of changed Go files from the
pull request, renders the Markdown report, removes any coverage comments it
posted on previous runs, and posts the report as a fresh PR comment.

In GitHub Actions it relies on the current run having already uploaded its own
coverage artifact (COVERAGE_ARTIFACT_NAME) and on the standard GitHub Actions
environment variables plus a GH_TOKEN with "pull-requests: write" and
"actions: read" permissions.

When run locally (outside GitHub Actions) it detects the absence of
GITHUB_ACTIONS and instead: reads the current coverage profile from disk
(--coverage-file, default coverage.out; generate it with "make test-unit"
first), derives the changed Go files from "git diff" against the target branch,
and writes the rendered report to coverage-report.md rather than commenting on a
PR. No --run-id is required in this mode.

Options:
  --repo VALUE               owner/repo slug         (env: GH_REPO/GITHUB_REPOSITORY, default: cloudoperators/rootstock)
  --pr VALUE                 pull request number     (env: PR_NUMBER)
  --run-id VALUE             current workflow run id (env: GITHUB_RUN_ID)
  --root-package VALUE       Go import path prefix    (env: ROOT_PACKAGE, default: github.com/cloudoperators/rootstock)
  --baseline-workflow VALUE  baseline workflow file  (env: BASELINE_WORKFLOW, default: main.yaml)
  --target-branch VALUE      branch to compare with  (env: TARGET_BRANCH, default: main)
  --artifact-name VALUE      coverage artifact name  (env: COVERAGE_ARTIFACT_NAME, default: code-coverage)
  --coverage-file VALUE      coverage file name      (env: COVERAGE_FILE_NAME, default: coverage.out)
  --cli-version VALUE        go-coverage-report ver  (env: CLI_VERSION, default: v1.5.0)
  --skip-comment             render report only; do not comment (env: SKIP_COMMENT=true)
  --help                     Show this help and exit
EOF
}

# Script-global scratch directory. Declared here (not as a local in main) so the
# EXIT trap can still reference it after main has returned, and so the helper
# functions can use it without it going out of scope.
workdir=""

# Hidden HTML marker embedded at the top of every coverage comment. It lets a
# later run reliably find and update its own previous comment regardless of the
# report's visible text.
COMMENT_MARKER="<!-- rootstock-coverage-report -->"

# Filename the rendered report is written to on local (non-CI) runs.
LOCAL_REPORT_FILE="coverage-report.md"

main() {
  local repo="${GH_REPO:-${GITHUB_REPOSITORY:-cloudoperators/rootstock}}"
  local pr_number="${PR_NUMBER:-}"
  local run_id="${GITHUB_RUN_ID:-}"
  local root_package="${ROOT_PACKAGE:-github.com/cloudoperators/rootstock}"
  local baseline_workflow="${BASELINE_WORKFLOW:-main.yaml}"
  local target_branch="${TARGET_BRANCH:-main}"
  local artifact_name="${COVERAGE_ARTIFACT_NAME:-code-coverage}"
  local coverage_file="${COVERAGE_FILE_NAME:-coverage.out}"
  local cli_version="${CLI_VERSION:-v1.5.0}"
  local skip_comment="${SKIP_COMMENT:-false}"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo)              repo="$2"; shift 2 ;;
      --pr)                pr_number="$2"; shift 2 ;;
      --run-id)            run_id="$2"; shift 2 ;;
      --root-package)      root_package="$2"; shift 2 ;;
      --baseline-workflow) baseline_workflow="$2"; shift 2 ;;
      --target-branch)     target_branch="$2"; shift 2 ;;
      --artifact-name)     artifact_name="$2"; shift 2 ;;
      --coverage-file)     coverage_file="$2"; shift 2 ;;
      --cli-version)       cli_version="$2"; shift 2 ;;
      --skip-comment)      skip_comment="true"; shift ;;
      --help)              usage; exit 0 ;;
      *) printf "Unknown option: %s\n" "$1" >&2; usage; exit 1 ;;
    esac
  done

  # Determine whether to use the on-disk coverage profile (local runs) or to
  # download it from the current workflow run's artifact (CI runs). Locally
  # there is no workflow run to download from, so the profile must already
  # exist on disk; generate it with "make test-unit" first.
  local use_local_coverage="false"
  local report_file=""
  if [[ -z "${GITHUB_ACTIONS:-}" ]]; then
    [[ -f "$coverage_file" ]] || die "${coverage_file} not found - run 'make test-unit' first to generate it"
    use_local_coverage="true"
    # Local runs have no PR to comment on; write the rendered report to
    # LOCAL_REPORT_FILE instead of commenting.
    skip_comment="true"
    report_file="$LOCAL_REPORT_FILE"
  fi

  [[ -n "$repo" ]] || die "missing --repo (or GH_REPO/GITHUB_REPOSITORY)"
  # A workflow run id is only needed when the current coverage is downloaded
  # from that run's artifact (CI). Local runs use the on-disk profile instead.
  if [[ "$use_local_coverage" != "true" ]]; then
    [[ -n "$run_id" ]] || die "missing --run-id (or GITHUB_RUN_ID)"
  fi
  command -v gh >/dev/null 2>&1 || die 'this script requires the "gh" CLI'
  command -v go >/dev/null 2>&1 || die 'this script requires the "go" toolchain'

  export GH_REPO="$repo"

  workdir="$(mktemp -d)"
  trap 'if [[ -n "${workdir:-}" && -d "${workdir}" ]]; then rm -rf "${workdir}"; fi' EXIT

  install_cli "$cli_version"

  local new_cov="$workdir/new-coverage.out"
  local old_cov="$workdir/old-coverage.out"
  local changed_files="$workdir/changed-files.json"
  local report="$workdir/coverage-comment.md"

  if [[ "$use_local_coverage" == "true" ]]; then
    log_info "Using local coverage profile '${coverage_file}'"
    cp "$coverage_file" "$new_cov"
  else
    download_current_coverage "$run_id" "$artifact_name" "$coverage_file" "$new_cov"
  fi

  local baseline_run_id="" baseline_sha="" baseline_run_url=""
  if find_baseline_run "$baseline_workflow" "$target_branch"; then
    if download_baseline_coverage "$baseline_run_id" "$artifact_name" "$coverage_file" "$old_cov"; then
      log_info "Using baseline run ${baseline_run_id} (commit ${baseline_sha:0:7})"
    else
      log_warn "Baseline artifact could not be downloaded; reporting current coverage only"
      : > "$old_cov"
      baseline_run_id="" baseline_sha="" baseline_run_url=""
    fi
  else
    log_warn "No successful baseline run found on ${target_branch}; reporting current coverage only"
    : > "$old_cov"
  fi

  build_changed_files "$pr_number" "$target_branch" "$changed_files"

  render_report "$root_package" "$old_cov" "$new_cov" "$changed_files" \
    "$baseline_sha" "$baseline_run_id" "$baseline_run_url" "$report"

  if [[ ! -s "$report" ]]; then
    log_info "No coverage report generated (no changed Go files); nothing to comment"
    if [[ -n "$report_file" ]]; then
      : > "$report_file"
      log_info "Wrote empty coverage report to ${report_file}"
    fi
    return 0
  fi

  prepend_marker "$report"

  if [[ -n "$report_file" ]]; then
    cp "$report" "$report_file"
    log_info "Wrote coverage report to ${report_file}"
  fi

  if [[ "$skip_comment" == "true" ]]; then
    log_info "SKIP_COMMENT=true; rendered report but not commenting"
    if [[ -z "$report_file" ]]; then
      cat "$report"
    fi
    return 0
  fi

  [[ -n "$pr_number" ]] || die "missing --pr (or PR_NUMBER) but a comment was requested"
  post_comment "$repo" "$pr_number" "$report"
}

# install_cli installs the pinned go-coverage-report CLI into $(go env GOPATH)/bin
# and makes sure that directory is on PATH for the rest of the script.
install_cli() {
  local version="$1"
  local gobin
  gobin="$(go env GOPATH)/bin"
  if [[ ! -x "$gobin/go-coverage-report" ]]; then
    log_info "Installing go-coverage-report@${version}"
    _exec go install "github.com/fgrosse/go-coverage-report/cmd/go-coverage-report@${version}"
  fi
  case ":$PATH:" in
    *":$gobin:"*) ;;
    *) export PATH="$gobin:$PATH" ;;
  esac
}

# download_current_coverage fetches the coverage artifact that the current run
# uploaded and writes the coverage file to the given destination path.
download_current_coverage() {
  local run_id="$1" artifact_name="$2" coverage_file="$3" dest="$4"
  local dir="$workdir/current"
  log_info "Downloading current coverage artifact '${artifact_name}' from run ${run_id}"
  _exec gh run download "$run_id" --name="$artifact_name" --dir="$dir"
  [[ -f "$dir/$coverage_file" ]] || die "artifact '${artifact_name}' does not contain '${coverage_file}'"
  mv "$dir/$coverage_file" "$dest"
}

# find_baseline_run locates the latest successful run of the baseline workflow on
# the target branch (for the "push" event) and sets baseline_run_id/sha/url.
# Returns non-zero if no such run exists.
find_baseline_run() {
  local workflow="$1" branch="$2"
  local json
  if ! json="$(gh run list \
      --workflow="$workflow" \
      --branch="$branch" \
      --event=push \
      --status=success \
      --limit=1 \
      --json=databaseId,headSha,url)"; then
    return 1
  fi
  baseline_run_id="$(printf '%s' "$json" | jq -r '.[0].databaseId // empty')"
  baseline_sha="$(printf '%s' "$json" | jq -r '.[0].headSha // empty')"
  baseline_run_url="$(printf '%s' "$json" | jq -r '.[0].url // empty')"
  [[ -n "$baseline_run_id" ]]
}

# download_baseline_coverage downloads the baseline run's coverage artifact and
# writes the coverage file to the given destination path. Returns non-zero if the
# artifact or file is unavailable (e.g. expired).
download_baseline_coverage() {
  local run_id="$1" artifact_name="$2" coverage_file="$3" dest="$4"
  local dir="$workdir/baseline"
  if ! gh run download "$run_id" --name="$artifact_name" --dir="$dir" 2>/dev/null; then
    return 1
  fi
  [[ -f "$dir/$coverage_file" ]] || return 1
  mv "$dir/$coverage_file" "$dest"
}

# build_changed_files writes the list of changed *.go files (excluding vendored
# files) as a JSON string array, which is the format the go-coverage-report CLI
# expects. With a PR number the list comes from the pull request; otherwise
# (local runs) it is derived from "git diff" against the target branch.
build_changed_files() {
  local pr_number="$1" target_branch="$2" dest="$3"
  if [[ -n "$pr_number" ]]; then
    log_info "Collecting changed Go files of PR #${pr_number}"
    gh pr view "$pr_number" --json files \
      --jq '[.files[].path | select(endswith(".go")) | select(startswith("vendor/") | not)]' \
      > "$dest"
    return 0
  fi
  log_info "Collecting changed Go files from 'git diff' against ${target_branch}"
  {
    git diff --name-only "${target_branch}...HEAD" 2>/dev/null \
      | { grep -E '\.go$' || true; } \
      | { grep -v '^vendor/' || true; } \
      | jq -R -s -c 'split("\n") | map(select(length > 0))'
  } > "$dest"
}

# render_report runs the go-coverage-report CLI to produce the Markdown comment.
render_report() {
  local root_package="$1" old_cov="$2" new_cov="$3" changed_files="$4"
  local baseline_sha="$5" baseline_run_id="$6" baseline_run_url="$7" dest="$8"

  local baseline_flags=()
  if [[ -n "$baseline_sha" ]]; then
    baseline_flags=(
      -baseline-commit="$baseline_sha"
      -baseline-run-id="$baseline_run_id"
      -baseline-run-url="$baseline_run_url"
    )
  fi

  log_info "Rendering coverage report"
  go-coverage-report \
    -root="$root_package" \
    "${baseline_flags[@]}" \
    "$old_cov" \
    "$new_cov" \
    "$changed_files" \
    > "$dest"
}

# prepend_marker inserts the hidden COMMENT_MARKER as the first line of the
# report so that a later run can reliably find and delete its own previous
# comments regardless of the report's visible text.
prepend_marker() {
  local report="$1" tmp
  tmp="$(mktemp)"
  {
    printf '%s\n\n' "$COMMENT_MARKER"
    cat "$report"
  } > "$tmp"
  mv "$tmp" "$report"
}

# post_comment removes every previous coverage comment posted by this script on
# the pull request and then creates a single fresh comment, so each run ends up
# with exactly one coverage comment (bumped to the bottom of the PR thread).
post_comment() {
  local repo="$1" pr_number="$2" report="$3"

  delete_previous_comments "$repo" "$pr_number"

  log_info "Creating new coverage comment"
  _exec gh pr comment "$pr_number" --body-file="$report"
}

# delete_previous_comments deletes all coverage comments previously posted by
# github-actions[bot] on the pull request (identified by the hidden marker, or
# by the "Coverage Δ" table header for comments created before the marker
# existed). Missing comments are ignored.
delete_previous_comments() {
  local repo="$1" pr_number="$2"
  local id
  while IFS= read -r id; do
    [[ -n "$id" ]] || continue
    log_info "Deleting previous coverage comment ${id}"
    _exec gh api --method DELETE "repos/${repo}/issues/comments/${id}"
  done < <(find_existing_comments "$repo" "$pr_number")
}

# find_existing_comments prints the IDs (one per line) of every coverage comment
# posted by github-actions[bot] on the pull request. Comments carrying the
# hidden marker are matched first; comments created before the marker existed
# are matched by the "Coverage Δ" table header (which every report contains).
find_existing_comments() {
  local repo="$1" pr_number="$2"
  # --paginate --slurp returns a JSON array whose elements are the per-page
  # arrays; `add` flattens them into a single list of comments before filtering.
  gh api --paginate --slurp "repos/${repo}/issues/${pr_number}/comments" \
    | jq -r \
      '(add // [])
       | [ .[] | select(.user.login == "github-actions[bot]") ] as $bot
       | ( [ $bot[] | select(.body | contains("'"$COMMENT_MARKER"'")) ]
           + [ $bot[] | select(.body | test("Coverage Δ")) ] )
       | unique_by(.id)
       | .[].id'
}

main "$@"
