#!/usr/bin/env bash

git_commit=$(git rev-parse --short HEAD)
version="0.1.0"
prerelease="alpha1"

if [ "$prerelease" == "dev" ]; then
    echo "${version}-${prerelease}-${git_commit}"
elif [ -n "$prerelease" ]; then
    echo "${version}-${prerelease}"
else
    echo "${version}"
fi
