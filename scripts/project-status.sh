#!/bin/sh
# Read-only project overview for starting a work session or checking a release:
# repository sync, version sources, live site, installed binary and open work.
set -u
root=${AGY_STATUS_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}
site_url=${AGY_SITE_URL:-https://agy-swap.aklkbqx.com}
tool_repo=aklkbqx/agy-swap
site_repo=aklkbqx/agy-swap-site
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
: > "$tmp/drift"
flag() { printf '  - %s\n' "$1" >> "$tmp/drift"; }

issues_jq='.[] | "#\(.number) \(([.labels[].name | select(startswith("priority:"))] | first // "priority:-") | sub("priority:"; "")) \(([.labels[].name | select(startswith("status:"))] | first // "status:-") | sub("status:"; "")) [\(([.labels[].name | select(startswith("epic:"))] | first // "epic:-") | sub("epic:"; ""))] \(.title)"'

# Network reads run in parallel; each writes its own file.
gh release view -R "$tool_repo" --json tagName,publishedAt --jq '"\(.tagName) \(.publishedAt)"' > "$tmp/release" 2>/dev/null &
gh issue list -R "$tool_repo" --state open --limit 100 --json number,title,labels --jq "$issues_jq" > "$tmp/tool-issues" 2>/dev/null &
gh issue list -R "$site_repo" --state open --limit 100 --json number,title,labels --jq "$issues_jq" > "$tmp/site-issues" 2>/dev/null &
gh pr list -R "$tool_repo" --state open --json number --jq length > "$tmp/tool-prs" 2>/dev/null &
gh pr list -R "$site_repo" --state open --json number --jq length > "$tmp/site-prs" 2>/dev/null &
curl -s -o /dev/null -w '%{http_code}' --max-time 8 "$site_url/healthz" > "$tmp/health" 2>/dev/null &
curl -s --max-time 8 "$site_url/" > "$tmp/home" 2>/dev/null &
for repo in "$root" "$root/site"; do
  git -C "$repo" fetch -q origin 2>/dev/null &
done
wait

repo_state() { # NAME DIR
  name=$1
  dir=$2
  if [ ! -d "$dir/.git" ]; then
    printf '%-9s missing checkout\n' "$name"
    flag "$name: checkout missing"
    return
  fi
  branch=$(git -C "$dir" branch --show-current)
  head=$(git -C "$dir" rev-parse --short HEAD)
  dirty=$(git -C "$dir" status --porcelain | wc -l | tr -d ' ')
  counts=$(git -C "$dir" rev-list --left-right --count 'HEAD...@{u}' 2>/dev/null) || counts=""
  ahead=${counts%%[[:space:]]*}
  behind=${counts##*[[:space:]]}
  printf '%-9s %s@%s  uncommitted=%s ahead=%s behind=%s\n' "$name" "$branch" "$head" "$dirty" "${ahead:-?}" "${behind:-?}"
  [ "$dirty" = 0 ] || flag "$name: $dirty uncommitted file(s)"
  if [ -z "$counts" ]; then
    flag "$name: branch $branch has no upstream"
  else
    [ "$ahead" = 0 ] || flag "$name: $ahead unpushed commit(s)"
    [ "$behind" = 0 ] || flag "$name: $behind commit(s) behind origin"
  fi
  prs=$(cat "$tmp/$name-prs" 2>/dev/null)
  [ "${prs:-0}" = 0 ] || flag "$name: $prs open PRs"
}

repo_state tool "$root"
repo_state site "$root/site"

make_version=$(sed -n 's/^VERSION ?= *//p' "$root/Makefile" | head -1)
site_version=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$root/site/package.json" 2>/dev/null | head -1)
latest_tag=$(git -C "$root" tag --list 'v*' --sort=-v:refname | head -1)
release=$(cut -d' ' -f1 < "$tmp/release")
released_at=$(cut -d' ' -f2 < "$tmp/release")
health=$(cat "$tmp/health")
live=$(sed -n 's/.*"softwareVersion": *"\([^"]*\)".*/\1/p' "$tmp/home" | head -1)
installed_path=$(command -v agy-swap 2>/dev/null)
installed=$(agy-swap --version 2>/dev/null | awk '{print $2}')

printf 'versions  VERSION %s · tag %s · release %s (%s) · site package %s\n' "$make_version" "${latest_tag:--}" "${release:--}" "${released_at:--}" "${site_version:--}"
printf 'live      %s health %s · %s\n' "$site_url" "${health:-unreachable}" "${live:-unknown}"
printf 'installed %s (%s)\n' "${installed:-none}" "${installed_path:-not on PATH}"

[ "v$make_version" = "$latest_tag" ] || flag "VERSION $make_version is not tagged yet (latest tag ${latest_tag:-none})"
[ "$site_version" = "$make_version" ] || flag "site package ${site_version:-?} differs from VERSION $make_version"
[ "$health" = 200 ] || flag "live site health ${health:-unreachable}"
if [ -z "$release" ]; then
  flag "GitHub unreachable or gh not authenticated"
else
  [ "$latest_tag" = "$release" ] || flag "tag $latest_tag has no GitHub Release (latest $release)"
  [ "$live" = "$release" ] || flag "live site ${live:-unknown}, latest release $release"
  [ "$installed" = "$release" ] || flag "installed ${installed:-none}, latest release $release"
fi

tool_open=$(grep -c . "$tmp/tool-issues")
site_open=$(grep -c . "$tmp/site-issues")
printf '\nopen work (tool %s · site %s)\n' "$tool_open" "$site_open"
sed 's/^/  tool /' "$tmp/tool-issues"
sed 's/^/  site /' "$tmp/site-issues"

if [ -s "$tmp/drift" ]; then
  printf '\nDrift:\n'
  cat "$tmp/drift"
else
  printf '\nDrift: none\n'
fi
