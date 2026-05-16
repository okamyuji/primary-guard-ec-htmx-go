#!/usr/bin/env bash
# go fmt が未適用なファイルがあれば一覧を出して 1 で終わる
set -euo pipefail

diff=$(gofmt -l .)
if [ -n "$diff" ]; then
    echo "go fmt未適用ファイル"
    echo "$diff"
    exit 1
fi
