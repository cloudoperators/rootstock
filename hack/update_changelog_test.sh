#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# Local test suite for hack/update_changelog.sh.
#
# Each test builds a throwaway git repository as a fixture, runs the real
# changelog script (hack/update_changelog.sh) against it using the repo's real
# cliff.toml, and asserts the generated CHANGELOG.md / bumped version.
#
# Requirements: git, git-cliff (same tooling the CI uses).
#
# Usage:
#   hack/update_changelog_test.sh          # run all tests
#   DEBUG=true hack/update_changelog_test.sh   # print generated changelogs
#
# Exit code is non-zero if any test fails.
set -uo pipefail

# ---------------------------------------------------------------------------
# Locations / tooling
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
UPDATE_SCRIPT="${SCRIPT_DIR}/update_changelog.sh"
CLIFF_CONFIG="${REPO_ROOT}/cliff.toml"
DEBUG="${DEBUG:-false}"

if ! command -v git-cliff >/dev/null 2>&1; then
  echo "error: git-cliff is required to run this test suite" >&2
  echo "       install it, e.g.: brew install git-cliff" >&2
  exit 1
fi

# ===========================================================================
# Test Run
# ===========================================================================
main() {
  echo "Running changelog generation test suite"
  echo "  script : ${UPDATE_SCRIPT}"
  echo "  config : ${CLIFF_CONFIG}"
  echo "  cliff  : $(git-cliff --version)"
  echo

  test_pr_feat_fix_breaking
  test_tag_on_head
  test_tag_one_behind
  test_tag_two_behind
  test_new_commit_relabels_previous_unreleased

  echo
  if [[ "${TESTS_FAILED}" -eq 0 ]]; then
    echo "${GREEN}All ${TESTS_RUN} tests passed.${RESET}"
    exit 0
  else
    echo "${RED}${TESTS_FAILED} of ${TESTS_RUN} tests failed.${RESET}"
    exit 1
  fi

}

# ---------------------------------------------------------------------------
# Test harness
# ---------------------------------------------------------------------------
TESTS_RUN=0
TESTS_FAILED=0
CURRENT_TEST=""
FIXTURE_DIR=""

GREEN=$'\033[0;32m'; RED=$'\033[0;31m'; YELLOW=$'\033[1;33m'; RESET=$'\033[0m'

cleanup() { [[ -n "${FIXTURE_DIR}" && -d "${FIXTURE_DIR}" ]] && rm -rf "${FIXTURE_DIR}"; }
trap cleanup EXIT

start_test() {
  CURRENT_TEST="$1"
  TESTS_RUN=$((TESTS_RUN + 1))
  echo "${YELLOW}• ${CURRENT_TEST}${RESET}"
}

fail() {
  TESTS_FAILED=$((TESTS_FAILED + 1))
  echo "  ${RED}FAIL${RESET}: $1"
}

pass() { echo "  ${GREEN}PASS${RESET}: $1"; }

assert_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    pass "$msg"
  else
    fail "$msg"
    echo "    expected to contain: $needle"
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" msg="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    pass "$msg"
  else
    fail "$msg"
    echo "    expected NOT to contain: $needle"
  fi
}

assert_eq() {
  local actual="$1" expected="$2" msg="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$msg"
  else
    fail "$msg"
    echo "    expected: '$expected'"
    echo "    actual:   '$actual'"
  fi
}

# ---------------------------------------------------------------------------
# Fixture helpers
# ---------------------------------------------------------------------------

# Create a fresh fixture repo with a root commit and a baseline-feature commit
# that is tagged with the given baseline tag. The baseline commit is used as the
# changelog start commit (its parent "^" is the root commit, mirroring the real
# repo where the start commit has a parent).
#
# Sets globals: FIXTURE_DIR, START_COMMIT
new_fixture() {
  local baseline_tag="$1"
  cleanup
  FIXTURE_DIR="$(mktemp -d)"
  (
    cd "${FIXTURE_DIR}" || exit 1
    git init -q -b main
    # Isolate from the user's global git config (signing etc.).
    git config commit.gpgsign false
    git config tag.gpgsign false
    git config user.email "test@example.com"
    git config user.name "Test User"
    # Deterministic timestamps so the output is stable across machines.
    export GIT_AUTHOR_DATE="2024-01-01T00:00:00"
    export GIT_COMMITTER_DATE="2024-01-01T00:00:00"
    git commit -q --allow-empty -m "chore: initialize repository"
    git commit -q --allow-empty -m "feat: baseline feature"
    git tag "${baseline_tag}"
  )
  # git-cliff resolves cliff.toml from the current directory; give the fixture
  # the repo's real configuration so the test exercises the production config.
  cp "${CLIFF_CONFIG}" "${FIXTURE_DIR}/cliff.toml"
  START_COMMIT="$(cd "${FIXTURE_DIR}" && git rev-parse HEAD)"
}

# Add an empty commit with the given message to the fixture.
fixture_commit() {
  (cd "${FIXTURE_DIR}" && \
    GIT_AUTHOR_DATE="2024-06-01T00:00:00" GIT_COMMITTER_DATE="2024-06-01T00:00:00" \
    git commit -q --allow-empty -m "$1")
}

# Run update_changelog.sh inside the fixture.
#   gen_changelog [extra-env ...]  -> writes CHANGELOG.md, echoes its contents
# COMMIT_MSG / BASE_COMMIT are read from the environment by the script.
gen_changelog() {
  (
    cd "${FIXTURE_DIR}" || exit 1
    if ! CHANGELOG_START_COMMIT="${START_COMMIT}" DEBUG="${DEBUG}" \
         bash "${UPDATE_SCRIPT}" >/dev/null; then
      exit 1
    fi
    cat CHANGELOG.md
  )
}

# Return the bumped version the script would use.
gen_version() {
  (
    cd "${FIXTURE_DIR}" || exit 1
    CHANGELOG_START_COMMIT="${START_COMMIT}" \
    DEBUG=false \
      bash "${UPDATE_SCRIPT}" --get-version 2>/dev/null
  )
}

debug_dump() {
  if [[ "${DEBUG}" == "true" ]]; then
    echo "---- generated CHANGELOG.md ----"
    echo "$1"
    echo "--------------------------------"
  fi
}

# A conventional PR-style commit message with footers, as produced by the
# automerge/changelog_version workflow.
pr_commit_msg() {
  # $1 = subject, $2 = number, $3 = author
  printf '%s\n\nPR-Number: %s\nPR-Url: https://github.com/cloudoperators/rootstock/pull/%s\nPR-Author: %s' \
    "$1" "$2" "$2" "$3"
}

# ===========================================================================
# Tests
# ===========================================================================

# ---------------------------------------------------------------------------
# Scenario: new PR to main carrying feat, fix and breaking-change commits.
# The PR content is injected as a virtual commit via COMMIT_MSG (automerge flow).
# ---------------------------------------------------------------------------
test_pr_feat_fix_breaking() {
  start_test "new PR to main with feat, fix, breaking-change"
  new_fixture "v1.0.0"
  local base; base="$(cd "${FIXTURE_DIR}" && git rev-parse HEAD)"

  # feat -> minor, fix -> patch, breaking -> major; breaking wins => 2.0.0
  local feat fix brk
  feat="$(pr_commit_msg 'feat: add new api' 101 alice)"
  fix="$(pr_commit_msg 'fix: correct crash' 102 bob)"
  brk="$(pr_commit_msg 'feat!: remove deprecated flag' 103 carol)"

  # The automerge flow runs once per PR; simulate the three PRs landing in turn.
  # First PR (feat) lands as a real commit on main:
  fixture_commit "$feat"
  # Second PR (fix) lands as a real commit on main:
  fixture_commit "$fix"
  # Third PR (breaking) is the one currently being merged (virtual commit),
  # base is still the current tip of main.
  base="$(cd "${FIXTURE_DIR}" && git rev-parse HEAD)"

  local out ver
  out="$(COMMIT_MSG="$brk" BASE_COMMIT="$base" gen_changelog)"
  ver="$(COMMIT_MSG="$brk" BASE_COMMIT="$base" gen_version)"
  debug_dump "$out"

  assert_eq "$ver" "v2.0.0" "breaking change bumps to major v2.0.0"
  assert_contains "$out" "Add new api"            "feat entry present"
  assert_contains "$out" "Correct crash"          "fix entry present"
  assert_contains "$out" "Remove deprecated flag" "breaking entry present"
  assert_contains "$out" "❗"                      "breaking marker rendered"
  assert_contains "$out" "(2.0.0 - "              "all new entries grouped under 2.0.0"
  assert_contains "$out" "[#103]"                 "PR number of merging PR rendered"
  assert_contains "$out" "[@carol]"               "PR author rendered"
  # Baseline tagged release is preserved and NOT re-versioned.
  assert_contains "$out" "(1.0.0 - "              "baseline 1.0.0 entry preserved"
}

# ---------------------------------------------------------------------------
# Scenario: latest release tag is on HEAD of main (nothing unreleased).
# ---------------------------------------------------------------------------
test_tag_on_head() {
  start_test "latest release tag is on HEAD of main"
  new_fixture "v1.0.0"
  local base; base="$(cd "${FIXTURE_DIR}" && git rev-parse HEAD)"

  local out ver
  out="$(BASE_COMMIT="$base" gen_changelog)"
  ver="$(BASE_COMMIT="$base" gen_version)"
  debug_dump "$out"

  # With no unreleased commits the bumped version stays at the current tag.
  assert_eq "$ver" "v1.0.0" "no unreleased commits -> version stays at v1.0.0"
  assert_contains "$out" "(1.0.0 - " "existing release entry present"
  # No new, higher version section should be introduced.
  assert_not_contains "$out" "(1.0.1 - " "no spurious patch release created"
  assert_not_contains "$out" "unreleased"  "no floating unreleased section"
}

# ---------------------------------------------------------------------------
# Scenario: latest release tag is 1 commit behind HEAD of main.
# ---------------------------------------------------------------------------
test_tag_one_behind() {
  start_test "latest release tag is 1 commit behind HEAD of main"
  new_fixture "v1.0.0"
  fixture_commit "$(pr_commit_msg 'fix: a single bugfix' 201 dave)"
  local base; base="$(cd "${FIXTURE_DIR}" && git rev-parse HEAD)"

  local out ver
  out="$(BASE_COMMIT="$base" gen_changelog)"
  ver="$(BASE_COMMIT="$base" gen_version)"
  debug_dump "$out"

  assert_eq "$ver" "v1.0.1" "single fix one commit ahead -> patch bump v1.0.1"
  assert_contains "$out" "(1.0.1 - " "unreleased commit grouped under next version"
  assert_contains "$out" "A single bugfix" "the single commit appears"
  assert_contains "$out" "(1.0.0 - " "baseline release preserved"
}

# ---------------------------------------------------------------------------
# Scenario: latest release tag is 2 commits behind HEAD of main.
# Verifies that BOTH unreleased commits are grouped under the same recomputed
# next version (consistency: previous untagged entries are re-labeled).
# ---------------------------------------------------------------------------
test_tag_two_behind() {
  start_test "latest release tag is 2 commits behind HEAD of main"
  new_fixture "v1.0.0"
  # First unreleased commit is only a fix (would be a patch on its own)...
  fixture_commit "$(pr_commit_msg 'fix: first unreleased fix' 301 erin)"
  # ...second unreleased commit is a feat, which must pull BOTH into a minor.
  fixture_commit "$(pr_commit_msg 'feat: second unreleased feature' 302 frank)"
  local base; base="$(cd "${FIXTURE_DIR}" && git rev-parse HEAD)"

  local out ver
  out="$(BASE_COMMIT="$base" gen_changelog)"
  ver="$(BASE_COMMIT="$base" gen_version)"
  debug_dump "$out"

  assert_eq "$ver" "v1.1.0" "fix + feat two commits ahead -> minor bump v1.1.0"
  assert_contains "$out" "First unreleased fix"     "first unreleased commit present"
  assert_contains "$out" "Second unreleased feature" "second unreleased commit present"
  # Consistency: the earlier fix must be re-grouped under 1.1.0, not 1.0.1.
  assert_not_contains "$out" "(1.0.1 - " "earlier fix re-labeled to the minor release, not left at patch"

  # Count how many entries are grouped under 1.1.0 — both must be there.
  local count
  count="$(printf '%s\n' "$out" | grep -c '(1.1.0 - ' || true)"
  if [[ "$count" -ge 2 ]]; then
    pass "both unreleased commits grouped under the same next version (1.1.0)"
  else
    fail "both unreleased commits grouped under the same next version (1.1.0)"
    echo "    found $count entries under 1.1.0, expected >= 2"
  fi
  assert_contains "$out" "(1.0.0 - " "baseline release preserved"
}

# ---------------------------------------------------------------------------
# Extra consistency check: adding a new commit re-labels previously untagged
# entries (simulates a new PR landing after an unreleased commit already exists).
# ---------------------------------------------------------------------------
test_new_commit_relabels_previous_unreleased() {
  start_test "new commit re-labels previously unreleased entries"
  new_fixture "v1.0.0"
  # An unreleased fix already on main -> would be v1.0.1 on its own.
  fixture_commit "$(pr_commit_msg 'fix: pre-existing unreleased fix' 401 gina)"
  local base; base="$(cd "${FIXTURE_DIR}" && git rev-parse HEAD)"

  local before; before="$(BASE_COMMIT="$base" gen_version)"
  assert_eq "$before" "v1.0.1" "before new PR: version is patch v1.0.1"

  # A new feat PR is now merging (virtual commit) -> should pull everything to minor.
  local feat; feat="$(pr_commit_msg 'feat: incoming feature pr' 402 hank)"
  local out after
  out="$(COMMIT_MSG="$feat" BASE_COMMIT="$base" gen_changelog)"
  after="$(COMMIT_MSG="$feat" BASE_COMMIT="$base" gen_version)"
  debug_dump "$out"

  assert_eq "$after" "v1.1.0" "after feat PR: version re-computed to minor v1.1.0"
  assert_contains "$out" "Pre-existing unreleased fix" "previous unreleased commit still present"
  assert_not_contains "$out" "(1.0.1 - " "previous unreleased fix re-labeled from 1.0.1 to 1.1.0"
  assert_contains "$out" "(1.1.0 - " "entries grouped under new minor version"
}

main "${@}"
