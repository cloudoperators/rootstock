#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
#
# helm_release.sh — package the rootstock Helm chart and publish it to an OCI
# registry (GitHub Packages / GHCR by default).
#
# The registry push requires an authenticated helm client; run
# `helm registry login ghcr.io` beforehand. Changed-file validation is assumed
# to happen in helm-release.yaml.
set -euo pipefail

HC="\033[1m"
FMAG="\033[35m"
GREEN="\033[1;32m"
RED="\033[1;31m"
NC="\033[0m"

log_debug() { if [ "${DEBUG:-false}" = "true" ]; then printf "${HC}${FMAG}[.]${NC} %s\n" "$*" >&2; fi; }
log_info()  { printf "${GREEN}[+]${NC} %s\n" "$*" >&2; }
log_error() { printf "${RED}[error]${NC} %s\n" "$*" >&2; }
die()       { log_error "$*"; exit 1; }

# --- Static configuration -----------------------------------------------------
repository="cloudoperators/rootstock"
chart_name="rootstock"

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Package the rootstock Helm chart and publish it to an OCI registry, matching
the publish step of .github/workflows/helm-release.yaml.

Options:
  --registry VALUE     OCI registry host (env: REGISTRY, default: ghcr.io)
  --chart-folder VALUE Path to the chart directory
                       (env: CHART_FOLDER, default: k8s_configuration/chart)
  --version VALUE      Override the chart version
                       (env: VERSION, default: .version from Chart.yaml)
  --out-dir VALUE      Directory for the packaged .tgz (env: OUT_DIR, default: ./tmp)
  --dry-run            Package but do not push (env: DRY_RUN, default: false)
  --plain-http         Use plain HTTP for registry push/pull, for insecure
                       registries such as a local zot (env: PLAIN_HTTP, default: false)
  --force              Push even if the chart version already exists in the
                       registry (skip the overwrite guard) (env: FORCE, default: false)
  --debug              Verbose debug logging (env: DEBUG, default: false)
  --help               Show this help and exit

Publishes chart '${chart_name}' under repository '${repository}'.
Assumes 'helm registry login' has already happened.
EOF
}

main() {
  local registry="${REGISTRY:-ghcr.io}"
  local chart_folder="${CHART_FOLDER:-k8s_configuration/chart}"
  local version="${VERSION:-}"
  local out_dir="${OUT_DIR:-./tmp}"
  local dry_run="${DRY_RUN:-false}"
  local plain_http="${PLAIN_HTTP:-false}"
  local force="${FORCE:-false}"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --registry)     registry="$2";     shift 2 ;;
      --chart-folder) chart_folder="$2"; shift 2 ;;
      --version)      version="$2";      shift 2 ;;
      --out-dir)      out_dir="$2";      shift 2 ;;
      --dry-run)      dry_run=true;      shift ;;
      --plain-http)   plain_http=true;   shift ;;
      --force)        force=true;        shift ;;
      --debug)        DEBUG=true;        shift ;;
      --help)         usage; exit 0 ;;
      *)              printf "Unknown option: %s\n" "$1" >&2; usage; exit 1 ;;
    esac
  done

  preflight "$chart_folder"
  version="$(resolve_version "$version" "$chart_folder")"

  local oci_path="oci://${registry}/${repository}/charts/"
  log_info "registry:   ${registry}"
  log_info "repository: ${repository}"
  log_info "chart:      ${chart_name} (folder ${chart_folder})"
  log_info "version:    ${version}"
  log_info "oci path:   ${oci_path}"

  if [[ "$force" == true ]]; then
    log_debug "force enabled; skipping version-exists guard"
  elif version_exists "$oci_path" "$chart_name" "$version" "$plain_http"; then
    die "chart ${chart_name}:${version} already exists in ${oci_path}. Please increment the chart version."
  fi

  package_and_push "$chart_folder" "$version" "$out_dir" "$oci_path" "$dry_run" "$plain_http"
}

preflight() {
  local chart_folder="$1"
  local missing=()
  local t
  for t in helm yq; do
    command -v "$t" >/dev/null 2>&1 || missing+=("$t")
  done
  ((${#missing[@]})) && die "missing required tools: ${missing[*]}"
  [[ -d "$chart_folder" ]] || die "chart folder not found: ${chart_folder}"
  [[ -f "${chart_folder}/Chart.yaml" ]] || die "Chart.yaml not found in ${chart_folder}"
  log_debug "preflight ok"
}

# Flag override wins; otherwise read .version from Chart.yaml.
resolve_version() {
  local version="$1" chart_folder="$2"
  if [[ -z "$version" ]]; then
    version="$(yq '.version' "${chart_folder}/Chart.yaml")" \
      || die "failed to read .version from ${chart_folder}/Chart.yaml"
  fi
  [[ -n "$version" && "$version" != "null" ]] || die "could not resolve a chart version"
  printf '%s' "$version"
}

# True when the chart version already exists in the registry (helm pull succeeds).
version_exists() {
  local oci_path="$1" chart_name="$2" version="$3" plain_http="${4:-false}"
  local pulldir rc=0
  local http_args=()
  [[ "$plain_http" == true ]] && http_args=(--plain-http)
  pulldir="$(mktemp -d "${TMPDIR:-/tmp}/helm-release.XXXXXX")"
  if helm pull "${oci_path}${chart_name}" --version "${version}" "${http_args[@]}" -d "$pulldir" >/dev/null 2>&1; then
    log_debug "existing chart found at ${oci_path}${chart_name}:${version}"
  else
    log_debug "no existing chart at ${oci_path}${chart_name}:${version}"
    rc=1
  fi
  rm -rf "$pulldir"
  return "$rc"
}

package_and_push() {
  local chart_folder="$1" version="$2" out_dir="$3" oci_path="$4" dry_run="$5" plain_http="${6:-false}"
  local http_args=()
  [[ "$plain_http" == true ]] && http_args=(--plain-http)
  mkdir -p "$out_dir"

  helm package "$chart_folder" -d "$out_dir" --version "$version" \
    || die "helm package failed"

  # helm package writes a deterministic '<chart>-<version>.tgz'.
  local pkg="${out_dir}/${chart_name}-${version}.tgz"
  [[ -f "$pkg" ]] || die "packaged chart not found at ${pkg}"
  log_info "packaged $(basename "$pkg")"

  if [[ "$dry_run" == true ]]; then
    log_info "[dry-run] would 'helm push ${pkg} ${oci_path} ${http_args[*]}'"
    return
  fi

  helm push "$pkg" "$oci_path" "${http_args[@]}" \
    || die "helm push to ${oci_path} failed"
  log_info "pushed ${pkg} to ${oci_path}"
}

main "$@"
