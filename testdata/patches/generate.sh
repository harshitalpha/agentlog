#!/bin/sh
# Regenerates the sample patches in this folder from real git output, using the
# same flags the server uses (see internal/gitx/patch.go). The expected values
# live in e2e/tests/diff.unit.spec.ts: if you regenerate, re-check them.
#
#   testdata/patches/generate.sh
set -eu
out="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 LC_ALL=C
git init -q .
git config user.name t; git config user.email t@example.com; git config commit.gpgsign false

FLAGS="--format= --patch -M --no-color --first-parent --no-ext-diff --no-textconv --src-prefix=a/ --dst-prefix=b/"
snap() { git -c core.quotePath=false show $FLAGS HEAD > "$out/$1.patch"; }
commit() { git add -A; git commit -q -m "$1"; }

# root-commit: every file added
printf 'package main\n\nfunc main() {}\n' > main.go
printf '# demo\n' > README.md
commit "root"; snap root-commit

# add: a new two-line file
printf 'hello\nworld\n' > hello.txt
commit "add"; snap add

# empty-file: new empty file has no hunks
: > empty.txt
commit "empty"; snap empty-file

# multi-hunk: three separate hunks in one file
i=1; : > long.txt; while [ $i -le 40 ]; do echo "line $i" >> long.txt; i=$((i+1)); done
commit "long"
sed -e 's/^line 3$/line 3 changed/' -e 's/^line 20$/line 20 changed/' -e 's/^line 38$/line 38 changed/' long.txt > long.tmp && mv long.tmp long.txt
commit "multi hunk"; snap multi-hunk

# no-count-hunk: a one-line file gives "@@ -1 +1 @@"
printf 'old\n' > one.txt; commit "one"
printf 'new\n' > one.txt; commit "one changed"; snap no-count-hunk

# no-newline: old side lacks the final newline, new side has it, plus a file that loses it
printf 'a\nb' > nonl.txt; printf 'x\ny\n' > nonl2.txt; commit "nonl"
printf 'a\nc\n' > nonl.txt; printf 'x\ny' > nonl2.txt; commit "nonl changed"; snap no-newline

# delete
git rm -q hello.txt; commit "delete"; snap delete

# rename-pure: 100% similarity, no hunks
printf 'alpha\nbeta\ngamma\ndelta\n' > old-name.txt; commit "old name"
git mv old-name.txt new-name.txt; commit "rename"; snap rename-pure

# rename-edit: renamed and modified
printf 'l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n' > before.txt; commit "before"
git mv before.txt after.txt; printf 'l1\nl2\nl3 edited\nl4\nl5\nl6\nl7\nl8\n' > after.txt
commit "rename edit"; snap rename-edit

# binary: added, then modified
printf '\000\001\002\000\377' > img.bin; commit "bin"; snap binary
printf '\000\001\002\000\377\376' > img.bin; commit "bin2"; snap binary-modify

# spaces-unicode: spaces and non-ASCII (core.quotePath=false keeps them readable)
mkdir -p "dir with space"; printf 'hi\n' > "dir with space/héllo wörld.txt"
commit "unicode"; snap spaces-unicode

# quoted-path: a tab and a double quote force git to quote the path
printf 'q\n' > "tab	here.txt"; printf 'q\n' > 'quote"name.txt'
commit "quoted"; snap quoted-path

# dash-lines: removed "-- x" and added "++ y" look like ---/+++ headers inside the hunk
printf 'keep\n-- sql comment\nkeep2\n' > dash.txt; commit "dash"
printf 'keep\n++ counter\nkeep2\n' > dash.txt; commit "dash changed"; snap dash-lines

# mode-change: chmod only, no hunks
printf '#!/bin/sh\necho hi\n' > run.sh; commit "script"
chmod +x run.sh; commit "chmod"; snap mode-change

# multi-file: add + modify + delete + rename + binary in one commit
printf 'brand new\n' > multi-new.txt
printf 'line 3 changed\n' >> one.txt
git rm -q new-name.txt
git mv after.txt moved.txt
printf '\000\001' > multi.bin
commit "multi"; snap multi-file

# untracked: how the server shows a not-yet-added file (git diff --no-index against /dev/null)
mkdir -p sub; printf 'draft\nnotes\n' > sub/draft.txt
git diff --no-index --no-color --no-ext-diff --no-textconv --src-prefix=a/ --dst-prefix=b/ -- /dev/null sub/draft.txt > "$out/untracked.patch" || true

echo "wrote $(ls "$out"/*.patch | wc -l | tr -d ' ') patches to $out"
