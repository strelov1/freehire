#!/usr/bin/env bash
# Cuts one changelog release: a git tag plus a GitHub Release, and nothing else.
#
# The notes are the commit subjects that touched this component since its previous
# tag. There is no asset, and nothing here deploys — the host still ships from
# freehire-ops' release.sh. The release exists so a stretch of work has a name.
#
# Two components, same shape as career-ops' manifest (the root package and web)
# and as this repo's extension-v* tags:
#   backend  everything except web/, extension/ and design-system/
#   web      web/ and design-system/ (the UI primitives the site is built from)
# A commit that touches both is listed in both. extension/ keeps the releases
# extension.yml already cuts from package.json; it is not a third component here.
#
# release-please, which career-ops uses, only keeps conventional commits. This
# repository's subjects are sentences ("Classify inbox mail with Jev before the
# LLM"), and those are dropped on the floor, so the notes are collected here.
#
# The summary is not committed back to the tree. A pull request opened with the
# workflow token never gets the checks main requires — .github/workflows/contributors.yml
# is the write-up of that trap — and a changelog file that cannot be merged goes
# stale the day it lands. The tag is the git record; the release body is the summary.
#
# The first release for a component, when it has no tag yet, covers the last 30
# days rather than the whole history, and is numbered 0.1.0 (or 1.0.0 when the
# dispatch asks for a major). Later runs bump from that tag. A run with nothing
# new exits 0, so a quiet week is not an empty release.
set -euo pipefail

: "${COMPONENT:?COMPONENT must be set}"
: "${BUMP:?BUMP must be set}"
: "${LATEST:?LATEST must be set}"
: "${GITHUB_SHA:?GITHUB_SHA must be set}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"

case "$COMPONENT" in
  backend | web) ;;
  *)
    echo "unknown component: $COMPONENT" >&2
    exit 1
    ;;
esac

case "$BUMP" in
  patch | minor | major) ;;
  *)
    echo "unknown bump: $BUMP" >&2
    exit 1
    ;;
esac

case "$LATEST" in
  true | false) ;;
  *)
    echo "LATEST must be true or false" >&2
    exit 1
    ;;
esac

if [ "${DRY_RUN:-}" != "1" ] && [ "${GITHUB_REF:-}" != "refs/heads/main" ]; then
  echo "releases are cut from main (got ${GITHUB_REF:-unset})" >&2
  exit 1
fi

window='30 days ago'

if [ "$COMPONENT" = "backend" ]; then
  paths=(. ':(exclude)web' ':(exclude)extension' ':(exclude)design-system')
else
  paths=(web design-system)
fi

prev=
while IFS= read -r tag; do
  rest=${tag#"${COMPONENT}-v"}
  if [[ "$rest" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    prev=$tag
    break
  fi
done < <(git tag -l "${COMPONENT}-v*" --sort=-v:refname)

log_file=$(mktemp)
trap 'rm -f "$log_file"' EXIT
if [ -n "$prev" ]; then
  git log --no-merges --format='%h %s' "${prev}..${GITHUB_SHA}" -- "${paths[@]}" >"$log_file"
else
  git log --no-merges --since="$window" --format='%h %s' "${GITHUB_SHA}" -- "${paths[@]}" >"$log_file"
fi

commits=()
while IFS= read -r line; do
  [ -n "$line" ] || continue
  commits+=("$line")
done <"$log_file"

if [ "${#commits[@]}" -eq 0 ]; then
  echo "No ${COMPONENT} commits to release."
  exit 0
fi

bump_version() {
  local version="$1"
  local kind="$2"
  local major minor patch
  IFS=. read -r major minor patch <<<"$version"
  case "$kind" in
    patch) patch=$((patch + 1)) ;;
    minor)
      minor=$((minor + 1))
      patch=0
      ;;
    major)
      major=$((major + 1))
      minor=0
      patch=0
      ;;
  esac
  printf '%s.%s.%s\n' "$major" "$minor" "$patch"
}

if [ -z "$prev" ]; then
  if [ "$BUMP" = "major" ]; then
    version=1.0.0
  else
    version=0.1.0
  fi
else
  version=$(bump_version "${prev#"${COMPONENT}-v"}" "$BUMP")
fi

tag="${COMPONENT}-v${version}"
notes=$(mktemp)
trap 'rm -f "$log_file" "$notes"' EXIT

{
  echo "Changelog only. This tag does not deploy."
  echo
  if [ -n "$prev" ]; then
    echo "Changes since ${prev}."
    printf 'Compare: https://github.com/%s/compare/%s...%s\n' "$GITHUB_REPOSITORY" "$prev" "$tag"
  else
    echo "First summary. Commits from the last 30 days; numbering starts here."
  fi
  echo
  for line in "${commits[@]}"; do
    hash=${line%% *}
    subject=${line#* }
    if [ -z "$subject" ]; then
      continue
    fi
    printf '* %s (%s)\n' "$subject" "$hash"
  done
} >"$notes"

echo "---- ${tag} (${#commits[@]} commits) ----"
cat "$notes"

if [ "${DRY_RUN:-}" = "1" ]; then
  echo "dry run, not creating ${tag}"
  exit 0
fi

: "${GH_TOKEN:?GH_TOKEN must be set}"

if gh release view "$tag" --repo "$GITHUB_REPOSITORY" >/dev/null 2>&1; then
  echo "Release ${tag} already exists."
  exit 0
fi

args=(
  release create "$tag"
  --repo "$GITHUB_REPOSITORY"
  --target "$GITHUB_SHA"
  --title "${COMPONENT} v${version}"
  --notes-file "$notes"
)
if [ "$LATEST" = "true" ]; then
  args+=(--latest)
else
  args+=(--latest=false)
fi

gh "${args[@]}"
echo "Created ${tag}"
