#!/usr/bin/env bash
# scripts/sync-wiki.sh
#
# Mirrors docs/wiki/ in the main repo into the GitHub wiki (a separate
# git repository at git@github.com:<owner>/<repo>.wiki.git).
#
# Prereqs:
#   - The wiki must already exist. GitHub creates the underlying git
#     repo only after the first page is created via the web UI:
#     https://github.com/shaiknoorullah/agenthive/wiki → "Create the first page".
#   - Push access to the repo (SSH keys or gh CLI auth).
#
# Usage:
#   ./scripts/sync-wiki.sh
#
# What it does:
#   1. Clones the wiki into a temp dir.
#   2. Deletes every *.md in the wiki working tree.
#   3. Copies every *.md from docs/wiki/ (except this directory's own
#      README.md, which is repo-internal).
#   4. Commits with a stable subject ("docs: sync wiki from main").
#   5. Pushes. If there are no changes, exits 0.
#
# Idempotent. Safe to run repeatedly.

set -euo pipefail

REPO_OWNER="shaiknoorullah"
REPO_NAME="agenthive"
WIKI_URL="git@github.com:${REPO_OWNER}/${REPO_NAME}.wiki.git"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"
SRC_DIR="${REPO_ROOT}/docs/wiki"

if [[ ! -d "${SRC_DIR}" ]]; then
  echo "error: ${SRC_DIR} does not exist" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d -t agenthive-wiki-XXXXXX)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "→ cloning ${WIKI_URL} into ${TMP_DIR}"
if ! git clone --quiet "${WIKI_URL}" "${TMP_DIR}/wiki" 2>/dev/null; then
  cat >&2 <<'MSG'
error: could not clone the wiki repository.

The most common cause: the wiki has never been initialized. GitHub only
materializes the underlying git repo after the first page is created
through the web UI. Go to

  https://github.com/shaiknoorullah/agenthive/wiki

and click "Create the first page". Title and body can be anything; this
script will overwrite them on the next run.

Other possible causes: missing SSH key, missing push permission, or the
repo URL is wrong (see WIKI_URL at the top of this script).
MSG
  exit 2
fi

cd "${TMP_DIR}/wiki"

echo "→ clearing existing wiki pages"
git rm --quiet -f -- '*.md' 2>/dev/null || true

echo "→ copying docs/wiki/*.md into the wiki"
# Copy every .md EXCEPT README.md (which is repo-internal explanation,
# not a wiki page).
for src in "${SRC_DIR}"/*.md; do
  base="$(basename "${src}")"
  if [[ "${base}" == "README.md" ]]; then
    continue
  fi
  cp "${src}" "./${base}"
done

git add -A

if git diff --cached --quiet; then
  echo "→ wiki already up to date — nothing to push"
  exit 0
fi

echo "→ committing"
git -c user.name="${GIT_AUTHOR_NAME:-agenthive wiki sync}" \
    -c user.email="${GIT_AUTHOR_EMAIL:-wiki-sync@agenthive.local}" \
    commit --quiet -m "docs: sync wiki from main"

echo "→ pushing"
git push --quiet

echo "✓ wiki updated"
