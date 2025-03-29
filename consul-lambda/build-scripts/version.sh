#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


git_commit=$(git rev-parse --short HEAD)
version=$(grep 'var VERSION' ../consul-lambda/consul-lambda-extension/version.go | awk -F\" '{print $2}')
prerelease=$(grep 'var PRE_RELEASE' ../consul-lambda/consul-lambda-extension/version.go | awk -F\" '{print $2}')

if [ "$prerelease" == "dev" ]; then
    echo "${version}-${prerelease}-${git_commit}"
elif [ -n "$prerelease" ]; then
    echo "${version}-${prerelease}"
else
    echo "${version}"
fi
