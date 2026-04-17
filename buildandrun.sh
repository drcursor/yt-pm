#!/usr/bin/env bash
set -e

export GOROOT="$(ls -d /opt/homebrew/Cellar/go/*/libexec | head -1)"
export PATH="$GOROOT/bin:$PATH"

echo "Building..."
go build -o yt-pm .

echo "Running..."
./yt-pm
