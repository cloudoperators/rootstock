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
pull request, renders the Markdown report, and creates or updates the PR comment.

It must run inside GitHub Actions: it relies on the current run having already
uploaded its own coverage artifact (COVERAGE_ARTIFACT_NAME) and on the standard
GitHub Actions environment variables plus a GH_TOKEN with "pull-requests: write"
and "actions: read" permissions.

Options:
  --repo VALUE               owner/repo slug         (env: GH_REPO, default: \$GITHUB_REPOSITORY)
  --pr VALUE                 pull request number     (env: PR_NUMBER)
  --run-id VALUE             current workflow run id (env: GITHUB_RUN_ID)
  --root-package VALUE       Go import path prefix    (env: ROOT_PACKAGE)
  --baseline-workflow VALUE  baseline workflow file  (env: BASELINE_WORKFLOW, default: ci.yaml)
  --target-branch VALUE      branch to compare with  (env: TARGET_BRANCH, default: main)
  --artifact-name VALUE      coverage artifact name  (env: COVERAGE_ARTIFACT_NAME, default: code-coverage)
  --coverage-file VALUE      coverage file name      (env: COVERAGE_FILE_NAME, default: coverage.out)
  --cli-version VALUE        go-coverage-report ver  (env: CLI_VERSION, default: v1.5.0)
  --skip-comment             render report only; do not comment (env: SKIP_COMMENT=true)
  --help                     Show this help and exit
EOF
}

main() {
  local repo="${GH_REPO:-${GITHUB_REPOSITORY:-}}"
  local pr_number="${PR_NUMBER:-}"
  local run_id="${GITHUB_RUN_ID:-}"
  local root_package="${ROOT_PACKAGE:-}"
  local baseline_workflow="${BASELINE_WORKFLOW:-ci.yaml}"
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

  [[ -n "$repo" ]]   || die "missing --repo (or GH_REPO/GITHUB_REPOSITORY)"
  [[ -n "$run_id" ]] || die "missing --run-id (or GITHUB_RUN_ID)"
  command -v gh >/dev/null 2>&1 || die 'this script requires the "gh" CLI'
  command -v go >/dev/null 2>&1 || die 'this script requires the "go" toolchain'

  export GH_REPO="$repo"

  local workdir
  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' EXIT

  install_cli "$cli_version"

  local new_cov="$workdir/new-coverage.out"
  local old_cov="$workdir/old-coverage.out"
  local changed_files="$workdir/changed-files.json"
  local report="$workdir/coverage-comment.md"

  download_current_coverage "$run_id" "$artifact_name" "$coverage_file" "$new_cov"

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

  build_changed_files "$pr_number" "$changed_files"

  render_report "$root_package" "$old_cov" "$new_cov" "$changed_files" \
    "$baseline_sha" "$baseline_run_id" "$baseline_run_url" "$report"

  if [[ ! -s "$report" ]]; then
    log_info "No coverage report generated (no changed Go files); nothing to comment"
    return 0
  fi

  if [[ "$skip_comment" == "true" ]]; then
    log_info "SKIP_COMMENT=true; rendered report but not commenting"
    cat "$report"
    return 0
  fi

  [[ -n "$pr_number" ]] || die "missing --pr (or PR_NUMBER) but a comment was requested"
  upsert_comment "$repo" "$pr_number" "$report"
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

# build_changed_files writes the list of changed *.go files of the pull request
# (excluding vendored files) as a JSON string array, which is the format the
# go-coverage-report CLI expects. Without a PR number, an empty array is written.
build_changed_files() {
  local pr_number="$1" dest="$2"
  if [[ -z "$pr_number" ]]; then
    printf '[]' > "$dest"
    return 0
  fi
  log_info "Collecting changed Go files of PR #${pr_number}"
  gh pr view "$pr_number" --json files \
    --jq '[.files[].path | select(endswith(".go")) | select(startswith("vendor/") | not)]' \
    > "$dest"
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

# upsert_comment creates a new coverage comment on the pull request, replacing any
# previous coverage comment posted by github-actions[bot] (identified by the
# "Coverage Δ" marker the report always contains).
upsert_comment() {
  local repo="$1" pr_number="$2" report="$3"
  local comment_id
  comment_id="$(gh api "repos/${repo}/issues/${pr_number}/comments" \
    --jq '.[] | select(.user.login=="github-actions[bot]" and (.body | test("Coverage Δ"))) | .id' \
    | head -n 1)"
  if [[ -n "$comment_id" ]]; then
    log_info "Replacing previous coverage comment ${comment_id}"
    _exec gh api -X DELETE "repos/${repo}/issues/comments/${comment_id}"
  else
    log_info "Creating new coverage comment"
  fi
  _exec gh pr comment "$pr_number" --body-file="$report"
}

main "$@"
