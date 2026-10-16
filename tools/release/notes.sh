#!/usr/bin/env bash
# Prints one version's section of CHANGELOG.md (without its heading): notes.sh 0.1b12
# It fails if there is none, so a release cannot go out without its notes.
set -euo pipefail
version=${1#v}
root=$(cd "$(dirname "$0")/../.." && pwd)
notes=$(awk -v v="$version" '
	/^## / { on = (index($0, "## " v " ") == 1 || $0 == "## " v); next }
	on' "$root/CHANGELOG.md")
if [ -z "$(printf '%s' "$notes" | tr -d '[:space:]')" ]; then
	echo "CHANGELOG.md has no section for $version" >&2
	exit 1
fi
printf '%s\n' "$notes" | sed -e '/./,$!d'
