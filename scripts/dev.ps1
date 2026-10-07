param([ValidateSet('backend','frontend')][string]$Target = 'backend', [ValidateSet('auto','native','wsl')][string]$DockerRuntime = 'auto', [string]$WslDistro = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
Push-Location $taskRoot
try {
    if ($Target -eq 'frontend') {
        Push-Location (Join-Path $taskRoot 'web')
        try {
            if (-not (Test-Path -LiteralPath 'node_modules')) {
                npm.cmd ci
                if ($LASTEXITCODE -ne 0) { throw 'Falha ao instalar dependências web.' }
            }
            npm.cmd run dev
        } finally { Pop-Location }
    } else {
        $taskGoArguments = @('run', '-buildvcs=false', './cmd/dumpersg', '-docker-runtime', $DockerRuntime)
        if ($WslDistro) { $taskGoArguments += @('-wsl-distro', $WslDistro) }
        go @taskGoArguments
    }
    if ($LASTEXITCODE -ne 0) { throw 'Processo de desenvolvimento falhou.' }
} finally { Pop-Location }
