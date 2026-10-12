#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

NEXT_VERSION=$(git cliff --bumped-version)
VERSION="${NEXT_VERSION#v}"

case "$(uname)" in
  Darwin) sed -i ''  "s/^version:.*/version: ${VERSION}/" VERSION.yaml ;;
  *)      sed -i     "s/^version:.*/version: ${VERSION}/" VERSION.yaml ;;
esac