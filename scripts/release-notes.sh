#!/usr/bin/env sh
# Print release notes for a tag: the commits since the previous tag as
# `git log --oneline` shows them (abbreviated hashes, newest first), inside a
# code block so the lines render verbatim.
#
# usage: scripts/release-notes.sh <tag>
set -eu

tag="${1:?usage: release-notes.sh <tag>}"
prev="$(git describe --tags --abbrev=0 "${tag}^" 2>/dev/null || true)"

if [ -n "$prev" ]; then
    range="${prev}..${tag}"
    printf '## Changes since %s\n\n' "$prev"
else
    range="$tag"
    printf '## Changes\n\n'
fi

printf '```\n'
git log --oneline --no-decorate "$range"
printf '```\n'
