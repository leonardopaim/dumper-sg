[CmdletBinding()]
param([switch]$Force, [string]$ConfigPath = '', [string]$Address = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
$taskName = 'DumperSG-' + $taskIdentity.User.Value
if (-not $ConfigPath) { $ConfigPath = Join-Path $taskRoot ('data\autostart-' + $taskIdentity.User.Value + '.json') }
$ConfigPath = [IO.Path]::GetFullPath($ConfigPath)
. (Join-Path $PSScriptRoot 'startup-config.ps1')
$taskConfig = $null
if (Test-Path -LiteralPath $ConfigPath -PathType Leaf) {
    $taskConfig = Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    $null = Get-DumperStartupArguments -Config $taskConfig
    if (-not $Address) { $Address = $taskConfig.address }
    if ($Force -and $Address -ne $taskConfig.address) { throw 'Forced restart must use the configured address.' }
} elseif ($Force) { throw 'Forced restart requires an installed autostart configuration.' }
if (-not $Address) { $Address = '127.0.0.1:8787' }
if ($Address -notmatch '^127\.0\.0\.1:[1-9][0-9]{0,4}$') { throw 'Restart must target IPv4 loopback.' }
$taskBase = 'http://' + $Address + '/api/v1'
$taskOldInstance = ''
try { $taskOldInstance = (Invoke-RestMethod ($taskBase + '/application') -TimeoutSec 3).instance_id } catch {}
if (-not $Force) {
    try {
        $taskSession = Invoke-RestMethod ($taskBase + '/session') -TimeoutSec 3
        $taskReply = Invoke-RestMethod ($taskBase + '/application/restart') -Method Post -ContentType 'application/json' -Body '{}' -Headers @{'X-DumperSG-Token'=$taskSession.token} -TimeoutSec 5
        $taskOldInstance = $taskReply.instance_id
    } catch {
        if ($_.Exception.Response) { throw }
        throw 'Application did not respond. For a frozen backend, use scripts/restart.ps1 -Force; interrupted jobs may need container cleanup.'
    }
} else {
    $taskSchedule = Get-ScheduledTask -TaskName $taskName -TaskPath '\' -ErrorAction Stop
    if ($taskSchedule.Description -ne ('DumperSG local startup: ' + $taskRoot)) { throw 'Unexpected scheduled task owner.' }
    $taskOwnedConfig = [IO.Path]::GetFullPath((Join-Path $taskRoot ('data\autostart-' + $taskIdentity.User.Value + '.json')))
    if ($ConfigPath -ne $taskOwnedConfig) { throw 'Forced restart requires this installations logon configuration.' }
    if ([IO.Path]::GetFullPath($taskConfig.executable) -ne [IO.Path]::GetFullPath((Join-Path $taskRoot 'dist\dumpersg.exe'))) { throw 'Unexpected installation executable.' }
    # A responsive app can still refuse interruption of an active operation.
    $taskJobs = $null
    try { $taskJobs = Invoke-RestMethod ($taskBase + '/jobs') -TimeoutSec 3 } catch {}
    if (@($taskJobs | Where-Object { $_.status -in @('running','cancel_requested') }).Count) { throw 'Finish or cancel the active operation before restarting.' }
    $taskStatePath = Join-Path $taskConfig.dataDir 'startup\process.json'
    if (-not (Test-Path -LiteralPath $taskStatePath)) { throw 'No verified launcher process is available for forced restart.' }
    $taskState = Get-Content -LiteralPath $taskStatePath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($taskState.configPath -ne $ConfigPath -or $taskState.executable -ne [IO.Path]::GetFullPath($taskConfig.executable) -or $taskState.dataDir -ne [IO.Path]::GetFullPath($taskConfig.dataDir)) { throw 'Launcher process configuration does not match.' }
    $taskChildId = [int]$taskState.pid
    if ($taskChildId -le 0) { throw 'Invalid launcher process.' }
    $taskChild = Get-Process -Id $taskChildId -ErrorAction SilentlyContinue
    if ($taskChild -and ($taskChild.Path -ne $taskState.executable -or $taskChild.StartTime.ToUniversalTime().Ticks.ToString() -ne $taskState.startedTicks)) { throw 'Process identity changed; refusing to stop it.' }
    Stop-ScheduledTask -TaskName $taskName -TaskPath '\'
    for ($taskTry=0; $taskTry -lt 30; $taskTry++) {
        if ((Get-ScheduledTask -TaskName $taskName -TaskPath '\').State -ne 'Running') { break }
        Start-Sleep -Milliseconds 100
    }
    if ((Get-ScheduledTask -TaskName $taskName -TaskPath '\').State -eq 'Running') { throw 'Background launcher did not stop.' }
    # A soft restart may have completed while the task was stopping. Read its
    # last child again only after the supervisor has stopped creating processes.
    $taskState = Get-Content -LiteralPath $taskStatePath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($taskState.configPath -ne $ConfigPath -or $taskState.executable -ne [IO.Path]::GetFullPath($taskConfig.executable) -or $taskState.dataDir -ne [IO.Path]::GetFullPath($taskConfig.dataDir)) { throw 'Launcher process configuration changed.' }
    $taskChildId = [int]$taskState.pid
    if ($taskChildId -le 0) { throw 'Invalid launcher process.' }
    $taskChild = Get-Process -Id $taskChildId -ErrorAction SilentlyContinue
    if ($taskChild) {
        if ($taskChild.Path -ne $taskState.executable -or $taskChild.StartTime.ToUniversalTime().Ticks.ToString() -ne $taskState.startedTicks) { throw 'Process identity changed; refusing to stop it.' }
        $null = $taskChild.Handle
        $taskChild.Kill()
        $null = $taskChild.WaitForExit(5000)
        if (-not $taskChild.HasExited) { throw 'Backend did not stop.' }
    }
    Start-ScheduledTask -TaskName $taskName -TaskPath '\'
}
$taskReady=$false
for ($taskTry=0; $taskTry -lt 90; $taskTry++) {
    try {
        $taskApplication = Invoke-RestMethod ($taskBase + '/application') -TimeoutSec 1
        if ($taskApplication.instance_id -and $taskApplication.instance_id -ne $taskOldInstance) { $taskReady=$true; break }
    } catch {}
    Start-Sleep -Milliseconds 500
}
if (-not $taskReady) { throw 'Restart was requested, but the new application did not respond. Check startup logs.' }
Write-Output ('DumperSG restarted. Open http://' + $Address)
