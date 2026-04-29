#!/usr/bin/env bash
set -euo pipefail

# Prepares a release by updating submodule go.mod dependencies and creating the root tag.
# After running, push manually: git push origin dev --tags

usage() {
  echo "Usage: $0 <version>"
  echo "  version: semver tag, e.g. v0.5.0"
  exit 1
}

if [ $# -ne 1 ]; then
  usage
fi

VERSION="$1"

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Error: version must match vX.Y.Z (got: $VERSION)"
  exit 1
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"

echo "Updating submodule go.mod files to ${VERSION}..."
sed -i "s|github.com/meshcore-go/meshcore-go v.*|github.com/meshcore-go/meshcore-go ${VERSION}|" \
  "${REPO_ROOT}/companion/transport/go.mod" \
  "${REPO_ROOT}/hardware/transport/go.mod"

echo "Committing dependency update..."
git add \
  "${REPO_ROOT}/companion/transport/go.mod" \
  "${REPO_ROOT}/hardware/transport/go.mod"
git commit -m "release: update submodule dependencies to ${VERSION}"

echo ""
echo "Done. Next steps:"
echo "  1. git push origin dev"
echo "  2. Merge dev into main (PR or fast-forward)"
echo "  3. git checkout main && git tag ${VERSION} && git push origin main --tags"
