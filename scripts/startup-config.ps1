# Shared by the launcher and installer; version 1 retains its explicit WSL mode.
function Get-DumperStartupArguments {
    param([Parameter(Mandatory=$true)]$Config)
    if ($Config.version -notin @(1,2)) { throw 'Unsupported startup configuration.' }
    if ($Config.address -notmatch '^127\.0\.0\.1:[1-9][0-9]{0,4}$') { throw 'Startup must bind to IPv4 loopback.' }
    $taskMode = 'wsl'
    if ($Config.version -eq 2) { $taskMode = $Config.dockerRuntime }
    if ($taskMode -notin @('auto','native','wsl')) { throw 'Docker runtime must be auto, native or wsl.' }
    $taskDistro = [string]$Config.wslDistro
    if ($taskMode -eq 'native' -and $taskDistro) { throw 'Native Docker cannot select a WSL distribution.' }
    if ($taskDistro.StartsWith('-') -or $taskDistro -match '["\r\n]') { throw 'Invalid WSL distribution.' }
    if ($Config.version -eq 1 -and [string]::IsNullOrWhiteSpace($taskDistro)) { throw 'Legacy configuration requires a WSL distribution.' }
    foreach ($taskValue in @($Config.executable,$Config.dataDir,$Config.mysqlImage,$Config.dumperImage)) {
        if ([string]::IsNullOrWhiteSpace($taskValue) -or $taskValue -match '["\r\n]') { throw 'Invalid startup parameter.' }
    }
    $taskArgs = @('-addr', $Config.address, '-data-dir', $Config.dataDir, '-docker-runtime', $taskMode)
    if ($taskDistro) { $taskArgs += @('-wsl-distro', $taskDistro) }
    $taskArgs += @('-restart-enabled', '-mysql-image', $Config.mysqlImage, '-docker-image', $Config.dumperImage,
        '-origins', 'http://127.0.0.1:8788,http://localhost:8788')
    return $taskArgs
}
