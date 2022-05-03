#!/usr/bin/env bash

set -euo pipefail

aws lambda invoke --function-name $(terraform output -raw lambda_name) /dev/stdout | cat
