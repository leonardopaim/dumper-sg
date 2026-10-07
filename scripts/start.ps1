param([string]$Address = '127.0.0.1:8787', [string]$DataDir = '', [ValidateSet('auto','native','wsl')][string]$DockerRuntime = 'auto', [string]$WslDistro = '', [string]$MySQLImage = '', [string]$DumperImage = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskExe = Join-Path $taskRoot 'dist\dumpersg.exe'
if (-not (Test-Path -LiteralPath $taskExe)) { & (Join-Path $PSScriptRoot 'build.ps1') }
if (-not (Test-Path -LiteralPath $taskExe)) { throw 'Executável não gerado.' }
$taskArguments = @('-addr', $Address)
$taskArguments += @('-docker-runtime', $DockerRuntime)
if ($WslDistro) { $taskArguments += @('-wsl-distro', $WslDistro) }
if ($MySQLImage) { $taskArguments += @('-mysql-image', $MySQLImage) }
if ($DumperImage) { $taskArguments += @('-docker-image', $DumperImage) }
if ($DataDir) { $taskArguments += @('-data-dir', $DataDir) }
& $taskExe @taskArguments
