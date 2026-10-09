param([string]$Address = '127.0.0.1:8787', [string]$DataDir = '', [ValidateSet('auto','native','wsl')][string]$DockerRuntime = 'auto', [string]$WslDistro = '', [string]$MySQLImage = 'mysql:8.4.3', [string]$DumperImage = '', [string]$Origins = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskExe = Join-Path $taskRoot 'dist\dumpersg.exe'
if (-not (Test-Path -LiteralPath $taskExe)) { & (Join-Path $PSScriptRoot 'build.ps1') }
if (-not (Test-Path -LiteralPath $taskExe)) { throw 'Executável não gerado.' }
$taskArguments = @('-addr', $Address, '-restart-enabled')
$taskArguments += @('-docker-runtime', $DockerRuntime)
if ($WslDistro) { $taskArguments += @('-wsl-distro', $WslDistro) }
if ($MySQLImage) { $taskArguments += @('-mysql-image', $MySQLImage) }
if ($DumperImage) { $taskArguments += @('-docker-image', $DumperImage) }
if ($DataDir) { $taskArguments += @('-data-dir', $DataDir) }
if ($Origins) { $taskArguments += @('-origins', $Origins) }
do {
    & $taskExe @taskArguments
    $taskExitCode = $LASTEXITCODE
} while ($taskExitCode -eq 75)
if ($taskExitCode -ne 0) { throw ('DumperSG encerrou com codigo ' + $taskExitCode) }
