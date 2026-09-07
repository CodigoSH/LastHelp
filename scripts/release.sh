#!/bin/bash

# ==========================================
# 🚀 Unified Release Script
# Handles: Stable, Beta and RC releases
#
# Usage: ./scripts/release.sh [stable|beta|rc] [X.Y.Z] ["release title"]
#                                              [--dry-run] [--yes] [--skip-checks]
#
# This is CodigoSH's release script, and the point of it is that it is the same
# one everywhere. It came from Lastboard, which has cut a lot of releases with
# it; what differs between projects is the CONFIGURATION block below and
# nothing else, so a fix made in one project is a file copied to the others.
#
# The title is the one-line summary of the release. It heads the notes — it is
# not a changelog entry — and it is committed as `release: <title>`, which is
# also how the aggregation of a stable knows to leave the beta titles out.
# Passing it as an argument is what keeps it from falling back to "minor updates".
#
# X.Y.Z overrides the computed version. Needed for stable-from-stable, where the
# computed one is the version already tagged.
#
# --dry-run prints the version it would produce, checks every file it would
# rewrite, renders the changelog and lists the git commands — without touching
# a single file. Use it to inspect a release before making it. It also states
# where a real run would stop to ask for confirmation.
# ==========================================

# Fail loudly: an unchecked error here means a tag pushed against a broken tree.
set -euo pipefail

# --- Arguments ------------------------------------------------------------

TYPE=""
TITLE=""
FORCED_VERSION=""
DRY_RUN=0
SKIP_CHECKS=0
ASSUME_YES=0

for arg in "$@"; do
    case "$arg" in
        stable|beta|rc)  TYPE="$arg" ;;
        --dry-run)       DRY_RUN=1 ;;
        --skip-checks)   SKIP_CHECKS=1 ;;
        --yes|-y)        ASSUME_YES=1 ;;
        --*)             echo "❌ Unknown option: $arg"; exit 1 ;;
        # A bare X.Y.Z is the target version, not a title. This is the
        # stable-from-stable case, where the computed version is the one already
        # tagged and the real one has to be stated.
        [0-9]*.[0-9]*.[0-9]*) FORCED_VERSION="$arg" ;;
        *)               TITLE="$arg" ;;
    esac
done

if [[ "$TYPE" != "stable" && "$TYPE" != "beta" && "$TYPE" != "rc" ]]; then
    echo "Usage: ./scripts/release.sh [stable|beta|rc] [X.Y.Z] [\"release title\"] [options]"
    echo "  stable : Promotes current version to stable (removes suffixes)"
    echo "  beta   : Bumps Beta version (creates new patch-Beta.01 if current is stable)"
    echo "  rc     : Bumps RC version"
    echo
    echo "  X.Y.Z          Target version, for when the computed one is not the one you want"
    echo "                 (stable-from-stable: 1.0.0 is already tagged, so pass 1.0.1)"
    echo "  --dry-run      Show what would happen, change nothing"
    echo "  --yes, -y      Don't ask for confirmation (required to run stable without a terminal)"
    echo "  --skip-checks  Skip the test/build preflight (emergencies only)"
    exit 1
fi

# A stable offers itself to everyone on the stable channel, so it asks before
# pushing. Without a terminal that question cannot be answered — fail here
# rather than after two minutes of preflight.
if [ "$TYPE" = "stable" ] && [ "$DRY_RUN" = "0" ] && [ "$ASSUME_YES" = "0" ] && [ ! -t 0 ]; then
    echo "❌ 'stable' asks for confirmation and there is no terminal to ask on."
    echo "   Re-run with --yes to confirm up front."
    exit 1
fi

# --- CONFIGURATION --------------------------------------------------------
#
# The only part that differs between CodigoSH projects. Everything below this
# block is the same file in all of them.

PROJECT="LastHelp"
RELEASE_BRANCH="main"
VERSION_FILE="package.json"

# Where the version is stamped, as "file|match|replacement|label". The binary
# takes its version from -ldflags at build time and the fallback in version.go
# deliberately says "dev" — a binary that cannot say where it came from should
# not claim a version number — so package.json is the only file stamped.
VERSION_STAMPS=(
    "package.json|\"version\": \".*\"|s/\"version\": \".*\"/\"version\": \"\$NEW_VERSION\"/|package.json"
)

# Where the release notes come from. `changelog` takes the [Unreleased] section
# of CHANGELOG.md; `commits` generates them from Conventional Commit subjects.
#
# Hand-written wins: generated notes list what changed in the repository, which
# is not what a reader of a release page wants to know. A hand-written
# RELEASE_NOTES_<tag>.md still beats both.
NOTES_FROM="changelog"

# The preflight, as a list of "label|command". It runs before anything is
# written, because a tag and a push are what cannot be taken back cheaply.
#
# Entries are added as the things they check come to exist: `bun test` and
# `i18n:scan` join this list with the first frontend test and the first
# translated string. A preflight that invokes a script nobody wrote fails every
# release until someone deletes the line, which teaches people to delete lines.
PREFLIGHT=(
    "gofmt|test -z \"\$(gofmt -l ./cmd ./internal)\""
    "go vet|go vet ./..."
    "architecture rules|go test -count=1 ./internal/arch/"
    "bun install|bun install --frozen-lockfile"
    "frontend build|bun run build"
    "go test -race|go test -count=1 -race ./..."
    "typecheck|bun run typecheck"
    "i18n:sync|bun run i18n:sync"
    "i18n:scan|bun run i18n:scan"
)

# --- Everything below is the same in every CodigoSH project ---------------

die() { echo "❌ $1" >&2; exit 1; }

# Runs a command, or prints it when doing a dry run.
run() {
    if [ "$DRY_RUN" = "1" ]; then
        echo "   [dry-run] $*"
    else
        "$@"
    fi
}

if [ "$DRY_RUN" = "1" ]; then
    echo "🧪 DRY RUN — nothing will be written, committed or pushed."
    echo
fi

# --- 0. Preconditions ------------------------------------------------------

# A release must be reproducible from what is published, so the tree has to be
# clean and level with the remote before anything is rewritten.
echo "🔍 Checking the repository..."

[ -f "$VERSION_FILE" ] || die "$VERSION_FILE not found — run this from the project root."
git rev-parse --git-dir >/dev/null 2>&1 || die "Not a git repository."

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
[ "$CURRENT_BRANCH" = "$RELEASE_BRANCH" ] || \
    die "On branch '$CURRENT_BRANCH'. Releases are cut from '$RELEASE_BRANCH'."

if [ -n "$(git status --porcelain)" ]; then
    echo "   Uncommitted changes:"
    git status --short | sed 's/^/     /'
    # A dry run is for inspecting a release before making it, so it reports the
    # dirty tree and carries on; a real run stops.
    [ "$DRY_RUN" = "1" ] || \
        die "Commit or stash them first — a release commit should only carry the version bump."
    echo "   ⚠️  A real run would stop here."
    TREE_DIRTY=1
fi
TREE_DIRTY=${TREE_DIRTY:-0}

git fetch origin "$RELEASE_BRANCH" --quiet 2>/dev/null || \
    echo "   ⚠️  Could not reach origin; skipping the sync check."
if git rev-parse --verify --quiet "origin/$RELEASE_BRANCH" >/dev/null; then
    BEHIND=$(git rev-list --count "HEAD..origin/$RELEASE_BRANCH")
    [ "$BEHIND" = "0" ] || \
        die "Local $RELEASE_BRANCH is $BEHIND commit(s) behind origin. Pull first."
fi

if [ "$TREE_DIRTY" = "0" ]; then
    echo "   ✅ On $RELEASE_BRANCH, clean and up to date."
else
    echo "   ✅ On $RELEASE_BRANCH, up to date with origin."
fi

# --- 1. Preflight ----------------------------------------------------------

# Tag and push happen further down and cannot be taken back cheaply, so the
# suite runs *before* anything is written rather than by hand beforehand.
if [ "$SKIP_CHECKS" = "1" ]; then
    echo "⏭️  Preflight skipped (--skip-checks)."
else
    echo "🧪 Preflight..."
    for entry in "${PREFLIGHT[@]}"; do
        label="${entry%%|*}"
        command="${entry#*|}"
        eval "$command" >/dev/null 2>&1 || die "$label failed. Run it to see why: $command"
        echo "   ✅ $label"
    done

    # The frontend build writes into internal/web/dist, which is not committed,
    # so nothing is expected to change here. A diff means something else did.
    if [ "$TREE_DIRTY" = "0" ] && [ -n "$(git status --porcelain)" ]; then
        echo "   The preflight changed tracked files:"
        git status --short | sed 's/^/     /'
        [ "$DRY_RUN" = "1" ] || die "Commit them first."
        echo "   ⚠️  A real run would stop here."
    fi
fi

# --- 2. Calculate the new version -----------------------------------------

CURRENT_FULL=$(grep -oP '"version": "\K[^"]+' "$VERSION_FILE") || \
    die "Could not read the version from $VERSION_FILE."
echo "ℹ️  Current Version: $CURRENT_FULL"

BASE_VERSION=$(echo "$CURRENT_FULL" | sed -E 's/-(Beta|RC)\.[0-9]+$//')

case $TYPE in
    beta)
        if [[ "$CURRENT_FULL" == *"-Beta."* ]]; then
            LAST_NUM=$(echo "$CURRENT_FULL" | grep -oP 'Beta\.\K[0-9]+')
            NEXT_NUM=$((10#$LAST_NUM + 1))
        elif [[ "$CURRENT_FULL" == *"-RC."* ]]; then
            NEXT_NUM=1
        else
            # From stable, bump the patch first: 1.2.0 -> 1.2.1-Beta.01.
            # Reusing the same base would produce a version that sorts *below*
            # the stable it came from.
            IFS='.' read -r -a PARTS <<< "$BASE_VERSION"
            BASE_VERSION="${PARTS[0]}.${PARTS[1]}.$((PARTS[2] + 1))"
            NEXT_NUM=1
            echo "ℹ️  Bumping patch for new Beta cycle: $BASE_VERSION"
        fi
        NEW_VERSION="$BASE_VERSION-Beta.$(printf "%02d" $NEXT_NUM)"
        ;;

    rc)
        if [[ "$CURRENT_FULL" == *"-RC."* ]]; then
            LAST_NUM=$(echo "$CURRENT_FULL" | grep -oP 'RC\.\K[0-9]+')
            NEXT_NUM=$((10#$LAST_NUM + 1))
        else
            NEXT_NUM=1
        fi
        NEW_VERSION="$BASE_VERSION-RC.$(printf "%02d" $NEXT_NUM)"
        ;;

    stable)
        NEW_VERSION="$BASE_VERSION"
        echo "Current Version: $CURRENT_FULL"
        echo "Proposed Stable: $NEW_VERSION"
        ;;
esac
NEW_TAG="v$NEW_VERSION"

# An explicit X.Y.Z wins over the computed one. Stable-from-stable needs it: the
# computed version is the one already tagged.
if [ -n "$FORCED_VERSION" ]; then
    NEW_VERSION="$FORCED_VERSION"

    # The channel still decides the suffix. Taking the number literally means
    # `release.sh beta 1.0.0` prepares the tag v1.0.0 — a stable tag, cut on the
    # beta channel, from a command that says beta in it. The number given is the
    # base; if it already carries a suffix it is used as written.
    case "$NEW_VERSION" in
        *-Beta.*|*-RC.*) ;;
        *)
            case "$TYPE" in
                beta) NEW_VERSION="$NEW_VERSION-Beta.01" ;;
                rc)   NEW_VERSION="$NEW_VERSION-RC.01" ;;
            esac
            ;;
    esac

    # And the contradiction in the other direction is a mistake, not a default.
    if [ "$TYPE" = stable ]; then
        case "$NEW_VERSION" in
            *-Beta.*|*-RC.*)
                die "Asked for a stable release of $NEW_VERSION. Drop the suffix, or use the matching channel." ;;
        esac
    fi

    NEW_TAG="v$NEW_VERSION"
    echo "ℹ️  Version given on the command line: $NEW_VERSION"
fi

git rev-parse --verify --quiet "refs/tags/$NEW_TAG" >/dev/null && \
    die "Tag $NEW_TAG already exists. Pass the target version, e.g. '$0 $TYPE 1.2.3'."

echo "🚀 Preparing Release: $NEW_TAG ($TYPE)"

if [ "$TYPE" == "stable" ]; then
    # Spelled out rather than "this will push to production": a beta pushes too.
    # What only a stable does is offer itself to every installation on the
    # stable channel.
    echo "   This tags $NEW_TAG and offers the update to every installation on"
    echo "   the stable channel."
    if [ "$DRY_RUN" = "1" ]; then
        echo "   [dry-run] would ask for confirmation here (skip it with --yes)"
    elif [ "$ASSUME_YES" = "1" ]; then
        echo "   Confirmed with --yes."
    else
        read -r -p "   Continue? (y/N) " confirm || confirm=""
        if [[ $confirm != [yY] && $confirm != [yY][eE][sS] ]]; then
            echo "❌ Cancelled"
            exit 1
        fi
    fi
fi

# --- 3. Release title ------------------------------------------------------

# The one-line summary of the release. It heads the notes; it is NOT a changelog
# entry. It is committed as `release: <title>` so that the aggregation of a
# stable leaves every beta's title out of the changes.
if [ -z "$TITLE" ]; then
    if [ "$DRY_RUN" = "1" ]; then
        TITLE="(no title given)"
    elif [ -t 0 ]; then
        read -r -p "   Title for $NEW_TAG: " TITLE || TITLE=""
    fi
    [ -n "$TITLE" ] || die "A release with no title is one nobody can tell apart from the last."
fi

# --- 4. Update the version files -------------------------------------------

# Every rewrite is checked: a silent no-op here leaves the version disagreeing
# between files, which only shows up once someone reports a version they never
# installed.
apply_version() {
    local file=$1 pattern=$2 replacement=$3 label=$4

    [ -f "$file" ] || die "$label: $file not found."
    grep -qE "$pattern" "$file" || \
        die "$label: no line matching /$pattern/ in $file — the file changed shape, update this script."

    if [ "$DRY_RUN" = "1" ]; then
        echo "   [dry-run] would rewrite $label in $file"
        echo "             $(grep -m1 -E "$pattern" "$file" | sed 's/^[[:space:]]*//')"
        return
    fi

    sed -i "$replacement" "$file"
    grep -qF "$NEW_TAG" "$file" || grep -qF "$NEW_VERSION" "$file" || \
        die "$label: the rewrite of $file did not take effect."
    echo "   ✅ $label"
}

echo "📝 Updating version files..."
STAMPED_FILES=()
for stamp in "${VERSION_STAMPS[@]}"; do
    IFS='|' read -r sfile spattern sreplacement slabel <<< "$stamp"

    # Substitution, not `eval echo`.
    #
    # Lastboard calls apply_version directly, so the shell expands $NEW_VERSION
    # inside a double-quoted argument and the escaped quotes survive. Holding
    # the same expressions in an array means expanding them later, and `eval
    # echo` was the obvious way to do it — it is also the wrong one: eval parses
    # the string a second time and eats every double quote, so
    # s/"version": ".*"/... arrives at sed as s/version: .*/... and matches
    # nothing. The failure is silent, because sed is perfectly happy to replace
    # zero occurrences.
    sreplacement=${sreplacement//\$NEW_VERSION/$NEW_VERSION}
    sreplacement=${sreplacement//\$NEW_TAG/$NEW_TAG}

    apply_version "$sfile" "$spattern" "$sreplacement" "$slabel"
    STAMPED_FILES+=("$sfile")
done

# --- 5. The changelog and the notes ----------------------------------------

if [ "$TYPE" == "stable" ]; then
    LAST_TAG=$(git tag -l "v*" --sort=-v:refname | grep -v -E 'Beta|RC' | head -n 1 || true)
    [ -n "$LAST_TAG" ] || LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || true)
else
    LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || true)
fi
# A project's first release has nothing to diff against, and dying here means
# this script cannot cut the release it exists for. The range is then the whole
# history, which is exactly what a first changelog should say.
if [ -n "$LAST_TAG" ]; then
    RANGE="$LAST_TAG..HEAD"
    RANGE_LABEL="$LAST_TAG..HEAD"
else
    RANGE="HEAD"
    RANGE_LABEL="the beginning"
    echo "ℹ️  No previous tag: the changelog covers the whole history."
fi

# Commits that describe the release process rather than the release, and the
# types nobody deciding whether to update needs to read.
HOUSEKEEPING='^release:|^chore\(release\)|^docs\(release\)|^chore: cleanup|^chore: bump|pre-release|^Merge branch|^Merge remote-tracking|^test(\([a-z0-9_ -]+\))?!?:|^ci(\([a-z0-9_ -]+\))?!?:|^style(\([a-z0-9_ -]+\))?!?:|^docs(\([a-z0-9_ -]+\))?!?:|^[a-z]+\(docs\)!?:|^[a-z]+\(ci\)!?:'
CONVENTIONAL_TYPES='feat|fix|refactor|perf|chore|docs|style|build|ci|test|i18n|revert'

clean_subjects() {
    grep -vE "$HOUSEKEEPING" | \
    sed -E "s/^($CONVENTIONAL_TYPES)(\([a-z0-9_ -]+\))?(!)?: *//I" | \
    awk 'NF {$1=toupper(substr($1,1,1))substr($1,2); print "- " $0}' | \
    sort -u
}
subjects() { git log "$RANGE" --pretty=format:"%s"; }

# The [Unreleased] section, for projects whose notes come from the changelog.
unreleased() {
    awk '/^## \[Unreleased\]/ { on = 1; next } /^## \[/ { on = 0 } on { print }' CHANGELOG.md \
        | sed -e '/./,$!d' | awk 'BEGIN{b=0} {lines[NR]=$0} END{for(i=NR;i>0;i--) if(lines[i] ~ /[^[:space:]]/){last=i;break} for(i=1;i<=last;i++) print lines[i]}'
}

NOTES_FILE="RELEASE_NOTES_$NEW_TAG.md"

# A hand-written file always wins: the generated one is a list of commit
# subjects, which is not what a reader wants.
if [ -f "$NOTES_FILE" ]; then
    echo "📄 Using existing release notes: $NOTES_FILE"
    BODY=$(cat "$NOTES_FILE")
elif [ "$NOTES_FROM" = "changelog" ]; then
    BODY_SECTIONS=$(unreleased)
    [ -n "$(echo "$BODY_SECTIONS" | tr -d '[:space:]')" ] || \
        die "CHANGELOG.md has nothing under [Unreleased], so there is nothing to say about $NEW_TAG."
    BODY=$(printf '**%s**\n\n%s' "$TITLE" "$BODY_SECTIONS")
    echo "📄 Release notes from the [Unreleased] section of CHANGELOG.md"
else
    FEATS=$(subjects | grep -iE "^feat" | clean_subjects || echo "")
    FIXES=$(subjects | grep -iE "^fix" | clean_subjects || echo "")
    OTHER=$(subjects | grep -viE "^(feat|fix)" | clean_subjects || echo "")
    BODY=$(
        printf '**%s**\n\n' "$TITLE"
        [ -n "$FEATS" ] && printf '### Added\n%s\n\n' "$FEATS"
        [ -n "$FIXES" ] && printf '### Fixed\n%s\n\n' "$FIXES"
        [ -n "$OTHER" ] && printf '### Changed\n%s\n' "$OTHER"
        true
    )
    echo "📄 Release notes generated from $RANGE_LABEL"
fi

if [ "$DRY_RUN" = "1" ]; then
    echo "   [dry-run] $NOTES_FILE would be:"
    printf '%s\n' "$BODY" | sed 's/^/     /'
else
    printf '%s\n' "$BODY" > "$NOTES_FILE"
fi

# Every release gets its own heading, betas included: somebody deciding whether
# to update wants to read what changed since the version they are running, and
# a beta they skipped is part of that.
if [ "$NOTES_FROM" = "changelog" ]; then
    echo "📝 Moving [Unreleased] into [$NEW_VERSION]..."
    if [ "$DRY_RUN" = "1" ]; then
        echo "   [dry-run] ## [$NEW_VERSION] - $(date +%Y-%m-%d)"
    else
        awk -v v="$NEW_VERSION" -v d="$(date +%Y-%m-%d)" '
            /^## \[Unreleased\]/ && !done { print "## [Unreleased]"; print ""; print "## [" v "] - " d; done=1; next }
            { print }
        ' CHANGELOG.md > CHANGELOG.md.tmp && mv CHANGELOG.md.tmp CHANGELOG.md
    fi
fi

# --- 6. Commit, tag, push --------------------------------------------------

echo "📦 Committing the release..."
# Named explicitly: `git add .` would sweep in anything else in the tree.
run git add "${STAMPED_FILES[@]}" CHANGELOG.md "$NOTES_FILE"
run git commit -m "release: $TITLE"
run git tag -a "$NEW_TAG" -m "$TITLE"

echo "📤 Pushing to Remote..."
# The tag goes last on purpose: it is what the CI builds from, so it must never
# point at commits the branch has not accepted.
run git push origin "$RELEASE_BRANCH" || \
    die "Push to $RELEASE_BRANCH failed. The tag $NEW_TAG stays local; delete it with 'git tag -d $NEW_TAG' once resolved."
run git push origin "$NEW_TAG" || \
    die "Tag push failed. $RELEASE_BRANCH is already pushed; retry with 'git push origin $NEW_TAG'."

# Cleanup: the notes live on in the tag and in the published release, but are
# removed from the branch so the root does not collect one file per release.
if [ "$DRY_RUN" = "1" ]; then
    echo "   [dry-run] rm $NOTES_FILE, commit cleanup, push"
    echo
    echo "🧪 Dry run complete — the working tree is untouched."
    exit 0
fi

rm -f "$NOTES_FILE"
git add "$NOTES_FILE"
git commit -q -m "chore(release): cleanup release notes [skip ci]"
git push origin "$RELEASE_BRANCH" >/dev/null

echo "✅ Successfully deployed $NEW_TAG!"
echo "   CI is building, signing and publishing it:"
echo "   https://git.codigosh.com/CodigoSH/$PROJECT/releases"
