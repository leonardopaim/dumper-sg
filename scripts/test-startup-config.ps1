$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'startup-config.ps1')
$taskBase = @{ version=2; executable='C:\Dumper SG\dumpersg.exe'; address='127.0.0.1:8787'; dataDir=('C:\Dados ' + [char]0xE7); dockerRuntime='auto'; wslDistro=''; mysqlImage='mysql:8.4.3'; dumperImage='mydumper/mydumper:latest' }
foreach ($taskCase in @(
    @{version=2; mode='auto'; distro=''; expected='auto'},
    @{version=2; mode='native'; distro=''; expected='native'},
    @{version=2; mode='wsl'; distro='Ubuntu Test'; expected='wsl'},
    @{version=2; mode='auto'; distro='Ubuntu Test'; expected='auto'},
    @{version=1; mode=$null; distro='Ubuntu'; expected='wsl'}
)) {
    $taskConfig = $taskBase.Clone()
    $taskConfig.version=$taskCase.version; $taskConfig.dockerRuntime=$taskCase.mode; $taskConfig.wslDistro=$taskCase.distro
    $taskArgs = @(Get-DumperStartupArguments -Config ([PSCustomObject]$taskConfig))
    $taskRuntimeIndex = [Array]::IndexOf($taskArgs,'-docker-runtime')
    $taskDistroIndex = [Array]::IndexOf($taskArgs,'-wsl-distro')
    if ($taskRuntimeIndex -lt 0 -or $taskArgs[$taskRuntimeIndex+1] -ne $taskCase.expected) { throw 'Wrong Docker runtime.' }
    if ($taskCase.distro) {
        if ($taskDistroIndex -lt 0 -or $taskArgs[$taskDistroIndex+1] -ne $taskCase.distro) { throw 'WSL distribution changed.' }
    } elseif ($taskDistroIndex -ge 0) { throw 'Default mode unexpectedly forced WSL.' }
    if ($taskArgs[[Array]::IndexOf($taskArgs,'-data-dir')+1] -ne $taskBase.dataDir) { throw 'Unicode or spaces changed.' }
}
foreach ($taskOverride in @(
    @{version=3}, @{dockerRuntime='other'}, @{dockerRuntime='native';wslDistro='Ubuntu'},
    @{wslDistro='--exec'}, @{wslDistro="Ubuntu`nOther"}, @{version=1;wslDistro=''},
    @{address='0.0.0.0:8787'}, @{dataDir=''}, @{mysqlImage='image"bad'}
)) {
    $taskConfig = $taskBase.Clone()
    foreach ($taskKey in $taskOverride.Keys) { $taskConfig[$taskKey]=$taskOverride[$taskKey] }
    $taskRejected=$false
    try { $null=Get-DumperStartupArguments -Config ([PSCustomObject]$taskConfig) } catch { $taskRejected=$true }
    if (-not $taskRejected) { throw ('Invalid configuration accepted: ' + ($taskOverride | ConvertTo-Json -Compress)) }
}
Write-Output 'PASS: startup auto/native/WSL, legacy WSL, Unicode paths and invalid configurations.'
