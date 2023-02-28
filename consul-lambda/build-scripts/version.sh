#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


git_commit=$(git rev-parse --short HEAD)
version="0.1.0"
prerelease="dev"

if [ "$prerelease" == "dev" ]; then
    echo "${version}-${prerelease}-${git_commit}"
elif [ -n "$prerelease" ]; then
    echo "${version}-${prerelease}"
else
    echo "${version}"
fi
