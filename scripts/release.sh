#!/bin/bash

# ==========================================
# 🚀 Unified Release Script
# Handles: Stable, Beta and RC releases
#
# Usage: ./scripts/release.sh [stable|beta|rc] [X.Y.Z] ["release title"]
#                                              [--dry-run] [--yes] [--skip-checks]
#
# This is CodigoSH's release script, and the point of it is that it is the same
# one everywhere: what differs between projects is the CONFIGURATION block below and
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

# The other products of the house. Their names never reach what a release
# publishes — its title, its notes, the changelog: what a project takes from
# another becomes its own, and is described by what it does.
OTHER_PRODUCTS=(Lastboard LastGYM LastDAV)

# Where the version is stamped, as "file|match|replacement|label". The binary
# takes its version from -ldflags at build time and the fallback in version.go
# deliberately says "dev" — a binary that cannot say where it came from should
# not claim a version number — so package.json is the only file stamped.
VERSION_STAMPS=(
    "package.json|\"version\": \".*\"|s/\"version\": \".*\"/\"version\": \"\$NEW_VERSION\"/|package.json"
)

# The preflight, as a list of "label|command". It runs before anything is
# written, because a tag and a push are what cannot be taken back cheaply.
#
# Entries are added as the things they check come to exist: `bun test` and
# `i18n:scan` join this list with the first frontend test and the first
# translated string. A preflight that invokes a script nobody wrote fails every
# release until someone deletes the line, which teaches people to delete lines.
# Pinned, and it is the only version of this analyser named anywhere in the
# project. A floating one puts the release in the red on a day nobody touched
# code, because somebody else published a stricter check.
# A pseudo-version, deliberately: the last tagged release reads the compiler's export
# data up to format 4, and Go 1.27.2 writes format 5, so every package that imports
# the standard library fails to load ("export data version 5 is greater than maximum
# supported version 4"). The commit named here is exact and immutable on the module
# proxy; it goes back to a tagged release the day one reads format 5.
STATICCHECK_VERSION="v0.7.0-0.dev.0.20261009230814-452d5bb86b45"

# The vulnerability scan is a preflight step because of where it fails. In CI it
# turns the repository red after the tag is public, which is where a user sees it;
# here it stops the release before anything is written. Pinned for the reason above:
# the tool is, the advisory database it reads is not, and that is the point.
GOVULNCHECK_VERSION="v1.9.0"

PREFLIGHT=(
    "gofmt|test -z \"\$(gofmt -l ./cmd ./internal)\""
    "go vet|go vet ./..."
    # go vet does not see dead code: a function that lost its last caller
    # compiles, passes vet and passes every test. Two of them had been sitting
    # here — a predicate about which forges share an adapter, and the accessor
    # for the settings store while seven places built that store by hand.
    "staticcheck|go run honnef.co/go/tools/cmd/staticcheck@\$STATICCHECK_VERSION ./..."
    "architecture rules|go test -count=1 ./internal/arch/"
    "bun install|bun install --frozen-lockfile"
    "frontend build|bun run build"
    # After the frontend build on purpose: the bundle is embedded, so nothing that
    # compiles this code can run before it exists. It exits non-zero only for what
    # this code reaches, and the usual cause is the Go toolchain being a patch behind.
    "govulncheck|go run golang.org/x/vuln/cmd/govulncheck@\$GOVULNCHECK_VERSION ./..."
    "go test -race|go test -count=1 -race ./..."
    "typecheck|bun run typecheck"
    "bun test|bun test"
    "i18n:sync|bun run i18n:sync"
    "i18n:scan|bun run i18n:scan"
)

# --- Everything below is the same in every CodigoSH project ---------------

die() {
    echo "❌ $1" >&2
    # A real run starts from a clean tree, so anything a stop leaves modified
    # was written by this script: the version stamps, and the changelog of a
    # stable. Put back, the tree is as it was and the run can be repeated —
    # otherwise the next attempt is refused for a tree that is not clean.
    # After the release is committed these match HEAD and nothing changes.
    if [ "${DRY_RUN:-0}" != "1" ] && [ -n "${STAMPED_FILES+set}" ]; then
        git checkout -q -- ${STAMPED_FILES[@]+"${STAMPED_FILES[@]}"} CHANGELOG.md 2>/dev/null || true
    fi
    exit 1
}

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
    # Called directly, apply_version gets $NEW_VERSION expanded by the shell
    # inside a double-quoted argument and the escaped quotes survive. Holding
    # the expressions in an array means expanding them later, and `eval
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
    # Since the previous STABLE, so the entry covers the whole beta cycle: the
    # reader is somebody upgrading from the last stable, and every beta in
    # between is part of what they are about to get.
    #
    # And if there is no previous stable, the range is the whole history rather
    # than the last beta. Falling back to the last tag reads as harmless and is
    # not: the first stable of a project that shipped three betas would list
    # only what changed since the third, and the first two would appear in no
    # changelog entry ever written.
    LAST_TAG=$(git tag -l "v*" --sort=-v:refname | grep -v -E 'Beta|RC' | head -n 1 || true)
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
CONVENTIONAL_TYPES='feat|fix|refactor|perf|chore|docs|style|build|ci|test|i18n|revert|remove'

clean_subjects() {
    grep -vE "$HOUSEKEEPING" | \
    grep -viE '^(feat|fix|chore)?(\([a-z0-9_-]+\))?:? *minor updates$' | \
    sed -E "s/^($CONVENTIONAL_TYPES)(\([a-z0-9_ -]+\))?(!)?: *//I" | \
    awk 'NF {$1=toupper(substr($1,1,1))substr($1,2); print "- " $0}' | \
    sort -u
}
subjects() { git log "$RANGE" --pretty=format:"%s"; }

# The buckets. Keep-a-Changelog has a Removed section, and without a bucket for
# it a withdrawal has to be committed as a feature and comes out under "Added",
# reading to anyone scanning the notes as the opposite of what happened. Commit
# a withdrawal as `remove:`.
FEATS=$(subjects | grep -iE "^feat" | clean_subjects || echo "")
FIXES=$(subjects | grep -iE "^fix" | clean_subjects || echo "")
REFACTORS=$(subjects | grep -iE "^(refactor|perf)" | clean_subjects || echo "")
REMOVALS=$(subjects | grep -iE "^remove" | clean_subjects || echo "")
OTHER=$(subjects | grep -viE "^(feat|fix|refactor|perf|remove)" | clean_subjects || echo "")

CHANGED=""
[ -n "$REFACTORS" ] && CHANGED="$REFACTORS"
[ -n "$OTHER" ] && CHANGED="${CHANGED:+$CHANGED$'\n'}$OTHER"

# A hand-written file always wins: the generated one is a list of commit
# subjects, which is not what a reader wants. It wins for the changelog of a
# stable too — otherwise a subject the notes were written to leave out would
# still land in CHANGELOG.md.
NOTES_FILE="RELEASE_NOTES_$NEW_TAG.md"
HAND_WRITTEN=""
[ -f "$NOTES_FILE" ] && HAND_WRITTEN=$(cat "$NOTES_FILE")

# --- What is published names no other product ------------------------------
#
# Checked here, before anything is written, tagged or pushed, and in a dry run
# as well: the text that would be published — the title, and the notes as
# written or as generated, which on a stable are also the changelog entry.
#
# The notes are generated from commit subjects, so one subject saying where
# something came from ("…is Lastboard's size") is a line on the release page.
# It happened, which is why this exists. A subject cannot be changed once it
# is pushed; the way out is to write the notes by hand.
if [ -n "$HAND_WRITTEN" ]; then
    PUBLISHED=$(printf '%s\n' "$TITLE" "$HAND_WRITTEN")
else
    PUBLISHED=$(printf '%s\n' "$TITLE" "$FEATS" "$FIXES" "$CHANGED" "$REMOVALS")
fi
NAMED=$(IFS='|'; printf '%s\n' "$PUBLISHED" | grep -inwE "${OTHER_PRODUCTS[*]}" || true)
if [ -n "$NAMED" ]; then
    die "what this release would publish names another product of the house:

$(printf '%s\n' "$NAMED" | sed 's/^[0-9]*:/    /')

    What a project took from another is its own now, and the notes say what it
    does. Write $NOTES_FILE by hand without it and run this again — a
    hand-written file always wins, for the notes and for the changelog."
fi

# --- CHANGELOG.md, on stables only -----------------------------------------
#
# A beta gets its own release page and its own notes; the file in the
# repository is the record of released versions, and a beta is not one. A
# stable aggregates the whole cycle, which is what somebody upgrading from the
# previous stable actually wants to read.
if [ "$TYPE" == "stable" ] && [ -f "CHANGELOG.md" ]; then
    echo "📝 Updating CHANGELOG.md..."
    DATE=$(date +%Y-%m-%d)

    # Built in a temp file with real newlines: composing it in a shell variable
    # strips the trailing ones and runs the section headers together, which
    # arrives as "…home### Fixed".
    ENTRY_FILE=$(mktemp)
    {
        printf '## [%s] - %s\n\n' "$NEW_VERSION" "$DATE"
        if [ -n "$HAND_WRITTEN" ]; then
            # The hand-written notes, without the bold title line that heads
            # the release page.
            printf '%s\n' "$HAND_WRITTEN" | sed '1{/^\*\*.*\*\*$/d}' | sed '/./,$!d'
            printf '\n'
        else
            [ -n "$FEATS" ]    && printf '### Added\n%s\n\n' "$FEATS"
            [ -n "$FIXES" ]    && printf '### Fixed\n%s\n\n' "$FIXES"
            [ -n "$CHANGED" ]  && printf '### Changed\n%s\n\n' "$CHANGED"
            [ -n "$REMOVALS" ] && printf '### Removed\n%s\n\n' "$REMOVALS"
        fi
        true
    } > "$ENTRY_FILE"

    if [ "$DRY_RUN" = "1" ]; then
        echo "   [dry-run] CHANGELOG.md entry that would be inserted:"
        sed 's/^/     /' "$ENTRY_FILE"
    else
        # Inserted before the first version heading, so the newest is on top.
        #
        # The END clause is what makes the FIRST entry work. Without it, a
        # changelog that has no version heading yet — a new project, or this
        # one before its first stable — matches nothing, the entry is never
        # printed, and the file comes out byte for byte identical. Measured:
        # the release is cut, the notes are published, and the changelog
        # silently gains nothing.
        awk 'NR==FNR { entry = entry $0 "\n"; next }
             /^## \[/ && !done { printf "%s", entry; done=1 }
             { print }
             END { if (!done) printf "%s", entry }' "$ENTRY_FILE" CHANGELOG.md > CHANGELOG.md.tmp \
            && mv CHANGELOG.md.tmp CHANGELOG.md
    fi
    rm -f "$ENTRY_FILE"
fi

# --- The release body ------------------------------------------------------
if [ -f "$NOTES_FILE" ]; then
    echo "📄 Using existing release notes: $NOTES_FILE"
else
    # The title leads the notes. It is the one line a reader sees first, and
    # keeping it out of the sections is what stops it from also being counted
    # as a feature of the next stable.
    compose_notes() {
        printf '**%s**\n\n' "$TITLE"
        [ -n "$FEATS" ]    && printf '### Added\n%s\n\n' "$FEATS"
        [ -n "$FIXES" ]    && printf '### Fixed\n%s\n\n' "$FIXES"
        [ -n "$CHANGED" ]  && printf '### Changed\n%s\n\n' "$CHANGED"
        [ -n "$REMOVALS" ] && printf '### Removed\n%s\n' "$REMOVALS"
        true
    }

    # Every subject in this range can be one the filter drops — a cycle of
    # `ci:`, `docs:` and `test:` work is a normal thing to have — and then the
    # notes are the title and nothing else. That is not a small blemish: the
    # release page is what somebody reads to decide whether to update, and a
    # page that says nothing tells them nothing.
    #
    # It happened twice in one day in a sibling project before anybody noticed,
    # because a dry run prints the empty result and it looks like a formatting
    # quirk rather than a refusal to say anything.
    #
    # So this stops, and names the way out: write the file by hand. The escape
    # hatch already exists and is checked for a few lines above.
    if [ -z "$FEATS$FIXES$CHANGED$REMOVALS" ]; then
        die "every commit since $RANGE_LABEL is one the notes filter drops, so the
    release page would carry the title and nothing else.

    Write $NOTES_FILE by hand and run this again — a hand-written file always
    wins. Say what the release is for, even if the answer is that nothing in the
    product changed."
    fi

    if [ "$DRY_RUN" = "1" ]; then
        echo "   [dry-run] $NOTES_FILE would be generated from $RANGE_LABEL as:"
        compose_notes | sed 's/^/     /'
    else
        compose_notes > "$NOTES_FILE"
        echo "📄 Release notes generated from $RANGE_LABEL"
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
