param([switch]$SkipTests)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
try {
    Push-Location (Join-Path $taskRoot 'web')
    try {
        npm.cmd ci
        if ($LASTEXITCODE -ne 0) { throw 'Falha ao instalar dependências web.' }
        if (-not $SkipTests) {
            npm.cmd test -- --run
            if ($LASTEXITCODE -ne 0) { throw 'Testes web falharam.' }
        }
        npm.cmd run build
        if ($LASTEXITCODE -ne 0) { throw 'Build web falhou.' }
    } finally { Pop-Location }
    if (-not $SkipTests) {
        go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Testes Go falharam.' }
        go vet -buildvcs=false ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go vet falhou.' }
    }
    New-Item -ItemType Directory -Force -Path (Join-Path $taskRoot 'dist') | Out-Null
    go build -buildvcs=false -trimpath -o dist/dumpersg.exe ./cmd/dumpersg
    if ($LASTEXITCODE -ne 0) { throw 'Build Go falhou.' }
    Write-Output ('Executável: ' + (Join-Path $taskRoot 'dist\dumpersg.exe'))
} finally { Pop-Location }
