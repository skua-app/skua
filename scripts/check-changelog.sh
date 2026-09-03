#!/usr/bin/env bash
#
# CHANGELOG consistency check. Run by `make check`.
#
# WHY THIS EXISTS. A commit once anchored a CHANGELOG edit on a released
# version's `## [0.16.0]` heading and consumed it without putting it back. The
# heading vanished and that release's entries were silently folded into
# [Unreleased]. It survived six weeks and every CI run, because nothing looked:
# `make check` had no changelog lint, and the workflow's tag verification only
# fires on a tag push and only asks about that one tag's own section.
#
# The file carries the same version list three times over — as headings, as
# comparison-link definitions at the bottom, and as git tags — and that
# redundancy is what makes the damage detectable. This script cross-checks all
# three, in both directions.
#
# NO NETWORK, EVER. It reads the working tree and the local tag list. It never
# fetches, so it is safe in CI and offline. The consequence is that in a shallow
# clone or a checkout made without tags there is nothing to compare tags
# against; see MODES below.
#
# Usage:  scripts/check-changelog.sh [--table]
#           --table  print the full version matrix, not just the failures.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHANGELOG="$ROOT/CHANGELOG.md"

# Versions below this shipped under the project's private development name and
# predate the public repository, so they deliberately have NO git tag and NO
# comparison-link definition. That is a documented policy — see the "Note on
# early history" at the top of CHANGELOG.md — not a gap, and the checks below
# exempt them rather than flagging seventeen false failures. The exemption
# cannot quietly swallow the whole file: the floor version itself is required
# to have both a heading and a link (check B3), so if the exemption ever starts
# applying to everything, that check fails first.
PUBLIC_HISTORY_FLOOR=0.8.0

TABLE=0
if [ "${1:-}" = "--table" ]; then TABLE=1; fi

if [ ! -f "$CHANGELOG" ]; then
  echo "changelog: FAIL — $CHANGELOG not found" >&2
  exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail_count=0
fail() {
  fail_count=$((fail_count + 1))
  printf 'changelog: FAIL — %s\n' "$1" >&2
}

# Numeric X.Y.Z comparison. `sort -V` is not portable enough to rely on (BSD and
# GNU differ), so sort the two operands on the three numeric fields instead.
ver_ge() {
  [ "$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | head -1)" = "$2" ]
}
sort_desc() { sort -t. -k1,1nr -k2,2nr -k3,3nr; }

# ---------------------------------------------------------------------------
# Extract the three lists.
# ---------------------------------------------------------------------------
# Headings, in the order they appear in the file. `## Comparison links` and any
# other prose heading is not bracketed, so it is not picked up here.
grep -E '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' "$CHANGELOG" |
  sed -E 's/^## \[([0-9]+\.[0-9]+\.[0-9]+)\].*/\1/' > "$TMP/headings-in-order" || true
LC_ALL=C sort -u "$TMP/headings-in-order" > "$TMP/headings"

# Comparison-link definitions at the bottom of the file.
grep -E '^\[[0-9]+\.[0-9]+\.[0-9]+\]:' "$CHANGELOG" |
  sed -E 's/^\[([0-9]+\.[0-9]+\.[0-9]+)\]:.*/\1/' > "$TMP/links-raw" || true
LC_ALL=C sort -u "$TMP/links-raw" > "$TMP/links"

# Tags, from the local repository only.
: > "$TMP/tags"
MODE=no-tags
MODE_NOTE='no tags in this checkout'
if git -C "$ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  git -C "$ROOT" tag --list 'v[0-9]*' 2>/dev/null |
    sed -E 's/^v//' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' > "$TMP/tags-raw" || true
  LC_ALL=C sort -u "$TMP/tags-raw" > "$TMP/tags"
  if [ -s "$TMP/tags" ]; then
    MODE=full
    MODE_NOTE="$(wc -l < "$TMP/tags" | tr -d ' ') tags visible"
  fi
  if [ "$(git -C "$ROOT" rev-parse --is-shallow-repository 2>/dev/null || echo false)" = "true" ]; then
    MODE_NOTE="$MODE_NOTE, shallow clone"
  fi
else
  MODE_NOTE='not a git repository'
fi

n_headings=$(wc -l < "$TMP/headings" | tr -d ' ')
n_links=$(wc -l < "$TMP/links" | tr -d ' ')

# Say which mode this run is in, always and up front. A check that quietly
# degrades to asserting less is worse than no check, so the reduced mode has to
# announce itself rather than print the same "OK" as a full run.
if [ "$MODE" = full ]; then
  printf 'changelog: mode=full (%s) — headings, links and tags cross-checked\n' "$MODE_NOTE"
else
  printf 'changelog: mode=no-tags (%s) — tag cross-checks SKIPPED; heading<->link checks still run\n' "$MODE_NOTE"
fi

# ---------------------------------------------------------------------------
# A. Structure of the file itself.
# ---------------------------------------------------------------------------
unreleased_headings=$(grep -c '^## \[Unreleased\]' "$CHANGELOG" || true)
if [ "$unreleased_headings" -ne 1 ]; then
  fail "expected exactly one '## [Unreleased]' heading, found $unreleased_headings. Every changelog edit anchors on it; without it the next edit anchors on a released version instead."
fi
if ! grep -qE '^\[Unreleased\]:' "$CHANGELOG"; then
  fail "no '[Unreleased]:' comparison-link definition at the bottom of the file."
fi

# Duplicate headings: two '## [X.Y.Z]' sections for one version.
LC_ALL=C sort "$TMP/headings-in-order" | uniq -d > "$TMP/dupes"
while read -r v; do
  [ -n "$v" ] || continue
  fail "version $v has more than one '## [$v]' heading."
done < "$TMP/dupes"

# Descending order. A heading reinserted in the wrong place is a sign that an
# edit went in against the wrong anchor, which is exactly the failure mode here.
sort_desc < "$TMP/headings-in-order" > "$TMP/headings-sorted"
if ! cmp -s "$TMP/headings-in-order" "$TMP/headings-sorted"; then
  fail "version headings are not in descending order. Expected: $(tr '\n' ' ' < "$TMP/headings-sorted")"
fi

# Each released heading carries a date, and has a body. An empty section means
# its entries went somewhere else — which is what the June damage looked like
# from the other side. [Unreleased] is legitimately empty right after a release.
awk '
  /^## \[[0-9]+\.[0-9]+\.[0-9]+\]/ {
    if (cur != "" && !seen) print "empty:" cur
    cur = $0
    sub(/^## \[/, "", cur); sub(/\].*/, "", cur)
    # The separator is an em dash, deliberately NOT written out in the pattern.
    # Under a C locale awk matches byte by byte, so a literal em dash inside a
    # bracket expression decomposes into its three UTF-8 bytes as three separate
    # single-byte alternatives: one byte matches, the pattern then wants
    # whitespace and finds the second byte of the dash instead, and every dated
    # heading in the file reports as undated. Testing the separator only as "one
    # or more non-space characters" is byte-safe and behaves the same in any
    # locale, and still rejects a heading with no separator or no date.
    if ($0 !~ /\][[:space:]]+[^[:space:]]+[[:space:]]+[0-9]{4}-[0-9]{2}-[0-9]{2}[[:space:]]*$/) print "undated:" cur
    seen = 0
    next
  }
  /^## / { if (cur != "" && !seen) print "empty:" cur; cur = ""; seen = 0; next }
  { if (cur != "" && $0 ~ /[^[:space:]]/) seen = 1 }
  END { if (cur != "" && !seen) print "empty:" cur }
' "$CHANGELOG" > "$TMP/section-problems" || true
while IFS=: read -r kind version; do
  [ -n "${version:-}" ] || continue
  case "$kind" in
    empty) fail "section '## [$version]' has no entries under it." ;;
    undated) fail "heading '## [$version]' has no '— YYYY-MM-DD' date." ;;
  esac
done < "$TMP/section-problems"

# ---------------------------------------------------------------------------
# B. Headings <-> link definitions. These run in BOTH modes, and they are what
#    still catches the original damage in a tagless checkout: the tag is not
#    visible, but the '[0.16.0]:' link definition at the bottom of the file is,
#    and it points at a heading that is no longer there.
# ---------------------------------------------------------------------------
LC_ALL=C comm -23 "$TMP/links" "$TMP/headings" > "$TMP/links-no-heading"
while read -r v; do
  [ -n "$v" ] || continue
  fail "link definition '[$v]:' exists but there is no '## [$v]' heading. A released section was deleted or renamed — restore the heading."
done < "$TMP/links-no-heading"

# Orphaned heading: a version at or above the public-history floor with no link.
: > "$TMP/orphans"
while read -r v; do
  ver_ge "$v" "$PUBLIC_HISTORY_FLOOR" || continue
  LC_ALL=C grep -qx "$v" "$TMP/links" || echo "$v" >> "$TMP/orphans"
done < "$TMP/headings"
if [ -s "$TMP/orphans" ]; then
  while read -r v; do
    fail "heading '## [$v]' has no '[$v]:' comparison-link definition at the bottom of the file."
  done < "$TMP/orphans"
fi

# B3 — the floor itself must be fully present, so the pre-0.8.0 exemption above
# can never widen into "nothing is checked".
if ! LC_ALL=C grep -qx "$PUBLIC_HISTORY_FLOOR" "$TMP/headings"; then
  fail "no '## [$PUBLIC_HISTORY_FLOOR]' heading. That version is the floor the pre-public-history exemption is anchored on."
fi
if ! LC_ALL=C grep -qx "$PUBLIC_HISTORY_FLOOR" "$TMP/links"; then
  fail "no '[$PUBLIC_HISTORY_FLOOR]:' link definition. That version is the floor the pre-public-history exemption is anchored on."
fi

# The Unreleased link must compare from the newest released version.
newest_heading="$(head -1 "$TMP/headings-sorted" || true)"
unreleased_from="$(sed -nE 's|^\[Unreleased\]: .*/compare/v([0-9]+\.[0-9]+\.[0-9]+)\.\.\.HEAD[[:space:]]*$|\1|p' "$CHANGELOG" | head -1)"
if [ -n "$newest_heading" ] && [ -n "$unreleased_from" ] && [ "$unreleased_from" != "$newest_heading" ]; then
  fail "[Unreleased] compares from v$unreleased_from but the newest version heading is $newest_heading. Bump the link."
fi

# ---------------------------------------------------------------------------
# C. Tags. Full mode only.
# ---------------------------------------------------------------------------
# Deliberately ONE-WAY: every tag must have a heading and a link, but a heading
# without a tag is fine. The release commit lands the section before the tag is
# pushed, so requiring the tag would fail every release PR.
if [ "$MODE" = full ]; then
  LC_ALL=C comm -23 "$TMP/tags" "$TMP/headings" > "$TMP/tags-no-heading"
  if [ -s "$TMP/tags-no-heading" ]; then
    while read -r v; do
      fail "tag v$v is released but has no '## [$v]' heading in CHANGELOG.md."
    done < "$TMP/tags-no-heading"
  fi
  LC_ALL=C comm -23 "$TMP/tags" "$TMP/links" > "$TMP/tags-no-link"
  if [ -s "$TMP/tags-no-link" ]; then
    while read -r v; do
      fail "tag v$v is released but has no '[$v]:' comparison-link definition."
    done < "$TMP/tags-no-link"
  fi
fi

# ---------------------------------------------------------------------------
# Report.
# ---------------------------------------------------------------------------
print_table() {
  printf '%-10s %-8s %-6s %-5s\n' VERSION HEADING LINK TAG
  while read -r v; do
    h=yes
    LC_ALL=C grep -qx "$v" "$TMP/links" && l=yes || l='-'
    if [ "$MODE" = full ]; then
      LC_ALL=C grep -qx "$v" "$TMP/tags" && t=yes || t='-'
    else
      t='?'
    fi
    printf '%-10s %-8s %-6s %-5s\n' "$v" "$h" "$l" "$t"
  done < "$TMP/headings-sorted"
}

if [ "$TABLE" -eq 1 ]; then print_table; fi

if [ "$fail_count" -gt 0 ]; then
  echo "changelog: $fail_count problem(s) found." >&2
  [ "$TABLE" -eq 1 ] || { echo "changelog: run 'scripts/check-changelog.sh --table' for the full version matrix." >&2; }
  exit 1
fi

exempt=0
while read -r v; do ver_ge "$v" "$PUBLIC_HISTORY_FLOOR" || exempt=$((exempt + 1)); done < "$TMP/headings"
printf 'changelog: OK — %s version headings, %s link definitions (%s pre-%s versions exempt by policy)\n' \
  "$n_headings" "$n_links" "$exempt" "$PUBLIC_HISTORY_FLOOR"
