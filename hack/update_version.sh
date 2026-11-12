#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

next_version=$(DEBUG=false hack/update_changelog.sh --get-version)
version="${next_version#v}"

if [[ -z "$version" ]] || ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9][a-zA-Z0-9.]*)?$ ]]; then
  echo "error: could not determine next version (got: '${next_version}')" >&2
  exit 1
fi

chart_file="k8s_configuration/chart/Chart.yaml"

case "$(uname)" in
  Darwin) SED_INPLACE=(-i '') ;;
  *)      SED_INPLACE=(-i) ;;
esac

sed "${SED_INPLACE[@]}" "s|^version:.*|version: ${version}|" VERSION.yaml
sed "${SED_INPLACE[@]}" "s|^appVersion:.*|appVersion: \"${version}\"|" "$chart_file"
