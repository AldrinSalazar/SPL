#!/bin/sh
# Reproducible build: native CLI, WASM engine, and web production bundle.
# Run from repo root: sh scripts/build-all.sh
set -e
go version
node --version
npm --version
go vet ./...
go test ./... -count=1
go build -o spl ./cmd/spl
GOOS=js GOARCH=wasm go build -o web/public/spl.wasm ./cmd/splwasm
npm run build --prefix web
echo "Build complete: spl/spl.exe, web/dist/"
