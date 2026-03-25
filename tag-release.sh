#!/usr/bin/env bash
# Tags all Go modules with the given version and pushes tags.
# Usage: ./tag-release.sh v0.2.0

set -euo pipefail

VERSION="${1:?Usage: $0 <version> (e.g. v0.2.0)}"

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-.*)?$ ]]; then
  echo "Error: version must match vX.Y.Z (got: $VERSION)" >&2
  exit 1
fi

MODULES=(core kurrentdb otel)

for mod in "${MODULES[@]}"; do
  tag="${mod}/${VERSION}"
  echo "Tagging ${tag}"
  git tag "${tag}"
done

echo ""
echo "Pushing tags..."
git push origin "${MODULES[@]/%//${VERSION}}"

echo ""
echo "Done. Tags pushed:"
for mod in "${MODULES[@]}"; do
  echo "  ${mod}/${VERSION}"
done
