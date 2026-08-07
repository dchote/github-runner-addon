#!/usr/bin/env bash
# Bump the addon version across every source that must stay in sync.
#
# Usage:
#   ./scripts/bump-version.sh              # patch (default)
#   ./scripts/bump-version.sh patch|minor|major
#   ./scripts/bump-version.sh 0.4.0        # exact version
#
# Updates:
#   github_runner/config.yaml
#   github_runner/internal/runtime/runtime.go (DefaultVersion)
#   github_runner/api/openapi.yaml (info.version)
#   github_runner/Dockerfile (ARG BUILD_VERSION default)
#   github_runner/CHANGELOG.md (prepend ## <version> if missing)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG="$ROOT/github_runner/config.yaml"
RUNTIME="$ROOT/github_runner/internal/runtime/runtime.go"
OPENAPI="$ROOT/github_runner/api/openapi.yaml"
DOCKERFILE="$ROOT/github_runner/Dockerfile"
CHANGELOG="$ROOT/github_runner/CHANGELOG.md"

current="$(sed -nE 's/^version:[[:space:]]*"([^"]+)".*/\1/p' "$CONFIG" | head -1)"
if [[ -z "$current" ]]; then
  echo "error: could not read version from $CONFIG" >&2
  exit 1
fi

bump="${1:-patch}"
if [[ "$bump" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-].*)?$ ]]; then
  next="$bump"
else
  IFS=. read -r major minor patch <<<"$current"
  patch="${patch%%[!0-9]*}"
  case "$bump" in
    patch) next="${major}.${minor}.$((patch + 1))" ;;
    minor) next="${major}.$((minor + 1)).0" ;;
    major) next="$((major + 1)).0.0" ;;
    *)
      echo "usage: $0 [patch|minor|major|X.Y.Z]" >&2
      exit 1
      ;;
  esac
fi

if [[ "$next" == "$current" ]]; then
  echo "version already $current"
  exit 0
fi

echo "bumping $current → $next"

# macOS/BSD and GNU sed both accept -i.bak; clean up backup.
sed_inplace() {
  local file="$1"
  shift
  sed -i.bak "$@" "$file"
  rm -f "${file}.bak"
}

# Replace only the first "  version: …" line (OpenAPI info.version).
replace_first_indented_version() {
  local file="$1" ver="$2"
  awk -v ver="$ver" '
    !done && /^  version: / { print "  version: " ver; done=1; next }
    { print }
  ' "$file" >"${file}.tmp"
  mv "${file}.tmp" "$file"
}

sed_inplace "$CONFIG" "s/^version:[[:space:]]*\".*\"/version: \"$next\"/"
sed_inplace "$RUNTIME" "s/DefaultVersion[[:space:]]*=[[:space:]]*\"[^\"]*\"/DefaultVersion  = \"$next\"/"
replace_first_indented_version "$OPENAPI" "$next"
sed_inplace "$DOCKERFILE" "s/^ARG BUILD_VERSION=.*/ARG BUILD_VERSION=$next/"

if ! grep -qE "^##[[:space:]]+${next//./\\.}([[:space:]]|$)" "$CHANGELOG"; then
  tmp="$(mktemp)"
  {
    echo "# Changelog"
    echo
    echo "## $next"
    echo
    echo "- _(add release notes)_"
    echo
    # Drop the leading "# Changelog" heading from the existing file.
    tail -n +2 "$CHANGELOG" | sed -e '1{/^$/d;}'
  } >"$tmp"
  mv "$tmp" "$CHANGELOG"
fi

echo "updated sources to $next"
echo "next: edit CHANGELOG.md, commit, tag $next, push, gh release create"
