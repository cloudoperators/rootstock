#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

NEXT_VERSION=$(DEBUG=false hack/update_changelog.sh --get-version)
VERSION="${NEXT_VERSION#v}"

if [[ -z "$VERSION" ]] || ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9][a-zA-Z0-9.]*)?$ ]]; then
  echo "error: could not determine next version (got: '${NEXT_VERSION}')" >&2
  exit 1
fi

case "$(uname)" in
  Darwin) sed -i ''  "s|^version:.*|version: ${VERSION}|" VERSION.yaml ;;
  *)      sed -i     "s|^version:.*|version: ${VERSION}|" VERSION.yaml ;;
esac
