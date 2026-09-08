# Reproducible build: native CLI, WASM engine, and web production bundle.
# Run from repo root: powershell -ExecutionPolicy Bypass -File scripts/build-all.ps1
$ErrorActionPreference = "Stop"
Write-Host "Go: $(go version)"
Write-Host "Node: $(node --version) / npm: $(cmd /c npm --version)"
go vet ./...
go test ./... -count=1
go build -o spl.exe ./cmd/spl
$env:GOOS = "js"; $env:GOARCH = "wasm"
go build -o web/public/spl.wasm ./cmd/splwasm
$env:GOOS = ""; $env:GOARCH = ""
cmd /c "npm run build --prefix web"
Write-Host "Build complete: spl.exe, web/dist/"
