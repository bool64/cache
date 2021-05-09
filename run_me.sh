#!/usr/bin/env bash
set -e

# Init script to kick-start your project
url=$(git remote get-url origin)
gh_repo=${url#"git@github.com:"}
gh_repo=${gh_repo#"https://github.com/"}
gh_repo=${gh_repo%".git"}
copyright="$(date +%Y) $(git config user.name)"
project_name=$(basename $gh_repo)

echo "## Replacing all go-library-template references by $project_name"
find . -type f -not -name run_me.sh -print0 | xargs -0 perl -i -pe "s|2021 bool64|$copyright|g"
find . -type f -not -name run_me.sh -print0 | xargs -0 perl -i -pe "s|bool64/go-library-template|$gh_repo|g"
find . -type f -not -name run_me.sh -print0 | xargs -0 perl -i -pe "s|go-library-template|$project_name|g"

echo "## Removing this script"
rm ./run_me.sh

echo "## Please check the @TODO's:"
git grep TODO | grep -v run_me.sh

