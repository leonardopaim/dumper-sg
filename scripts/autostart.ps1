[CmdletBinding(SupportsShouldProcess=$true)]
param(
    [ValidateSet('Install','Status','Remove')][string]$Action = 'Status',
    [string]$DataDir = (Join-Path $env:APPDATA 'DumperSG\web'),
    [string]$WslDistro = 'Ubuntu',
    [string]$MySQLImage = 'mysql:8.4.3',
    [string]$DumperImage = 'mydumper/mydumper:latest',
    [switch]$StartNow
)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskExe = Join-Path $taskRoot 'dist\dumpersg.exe'
$taskLauncher = Join-Path $PSScriptRoot 'run-background.ps1'
$taskIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
$taskName = 'DumperSG-' + $taskIdentity.User.Value
$taskConfigPath = Join-Path $taskRoot ('data\autostart-' + $taskIdentity.User.Value + '.json')
$taskMarker = 'DumperSG local startup: ' + $taskRoot
$taskExisting = Get-ScheduledTask -TaskName $taskName -TaskPath '\' -ErrorAction SilentlyContinue
if ($taskExisting -and $taskExisting.Description -ne $taskMarker) { throw 'Task name is already owned by another installation.' }
if ($Action -eq 'Status') {
    if (-not $taskExisting) { Write-Output 'DumperSG autostart is not installed.'; return }
    $taskExisting | Select-Object TaskName,State,Description
    Get-ScheduledTaskInfo -TaskName $taskName -TaskPath '\' | Select-Object LastRunTime,LastTaskResult,NextRunTime
    return
}
if ($Action -eq 'Remove') {
    if ($taskExisting -and $PSCmdlet.ShouldProcess($taskName, 'Remove DumperSG logon task')) {
        Unregister-ScheduledTask -TaskName $taskName -TaskPath '\' -Confirm:$false
        Write-Output 'Autostart removed. A running application is left running to preserve active jobs.'
    }
    return
}
if (-not (Test-Path -LiteralPath $taskExe -PathType Leaf)) { throw 'Run scripts/build.ps1 before installing autostart.' }
$taskDataDir = [IO.Path]::GetFullPath($DataDir)
foreach ($taskValue in @($taskConfigPath,$taskLauncher,$taskDataDir,$WslDistro,$MySQLImage,$DumperImage)) {
    if ([string]::IsNullOrWhiteSpace($taskValue) -or $taskValue -match '["\r\n]') { throw 'Invalid startup parameter.' }
}
if ($taskExisting -and $taskExisting.State -eq 'Running') { throw 'Autostart is already running; use Status. Reconfigure after the application has exited.' }
if ($PSCmdlet.ShouldProcess($taskName, 'Install logon startup for the current Windows user')) {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $taskConfigPath) | Out-Null
    $taskConfig = [ordered]@{ version=1; executable=$taskExe; address='127.0.0.1:8787'; dataDir=$taskDataDir; wslDistro=$WslDistro; mysqlImage=$MySQLImage; dumperImage=$DumperImage }
    [IO.File]::WriteAllText($taskConfigPath, ($taskConfig | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    $taskPowerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $taskArgs = '-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "' + $taskLauncher + '" -ConfigPath "' + $taskConfigPath + '"'
    $taskJobAction = New-ScheduledTaskAction -Execute $taskPowerShell -Argument $taskArgs -WorkingDirectory $taskRoot
    $taskTrigger = New-ScheduledTaskTrigger -AtLogOn -User $taskIdentity.Name
    $taskPrincipal = New-ScheduledTaskPrincipal -UserId $taskIdentity.Name -LogonType Interactive -RunLevel Limited
    $taskSettings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
    Register-ScheduledTask -TaskName $taskName -TaskPath '\' -Action $taskJobAction -Trigger $taskTrigger -Principal $taskPrincipal -Settings $taskSettings -Description $taskMarker -Force | Out-Null
    if ($StartNow) { Start-ScheduledTask -TaskName $taskName -TaskPath '\' }
    Write-Output ('Autostart installed for ' + $taskIdentity.Name + '. Open http://127.0.0.1:8787')
}
