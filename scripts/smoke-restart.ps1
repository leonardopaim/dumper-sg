param([string]$Executable = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
if (-not $Executable) { $Executable = Join-Path $taskRoot 'dist\dumpersg.exe' }
$Executable = [IO.Path]::GetFullPath($Executable)
$taskQa = Join-Path $taskRoot ('data\restart qa ' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskQa -Force | Out-Null
$taskListener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
$taskListener.Start(); $taskPort=$taskListener.LocalEndpoint.Port; $taskListener.Stop()
$taskBase = 'http://127.0.0.1:' + $taskPort + '/api/v1'
$taskConfig = [ordered]@{ version=2; executable=$Executable; address=('127.0.0.1:'+$taskPort); dataDir=(Join-Path $taskQa ('app data '+[char]0xE7)); dockerRuntime='native'; wslDistro=''; mysqlImage='mysql:8.4.3'; dumperImage='mydumper/mydumper:latest' }
$taskConfigPath=Join-Path $taskQa 'launcher.json'
[IO.File]::WriteAllText($taskConfigPath,($taskConfig|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$taskPowerShell=Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$taskWrapper=Start-Process -FilePath $taskPowerShell -ArgumentList @('-NoProfile','-NonInteractive','-WindowStyle','Hidden','-ExecutionPolicy','Bypass','-File',('"'+(Join-Path $taskRoot 'scripts\run-background.ps1')+'"'),'-ConfigPath',('"'+$taskConfigPath+'"')) -PassThru -WindowStyle Hidden -RedirectStandardError (Join-Path $taskQa 'wrapper-error.log')
$null=$taskWrapper.Handle
function Wait-RestartInstance([string]$Previous) {
    for($taskTry=0;$taskTry -lt 100;$taskTry++) {
        try {
            $taskCurrent=Invoke-RestMethod ($taskBase+'/application') -TimeoutSec 1
            if($taskCurrent.instance_id -and $taskCurrent.instance_id -ne $Previous){return $taskCurrent}
        } catch {}
        Start-Sleep -Milliseconds 100
    }
    throw 'New backend instance did not appear.'
}
try {
    $taskBefore=Wait-RestartInstance ''
    if(-not $taskBefore.restart_available){throw 'Supervised restart is unavailable.'}
    $taskStatePath=Join-Path $taskConfig.dataDir 'startup\process.json'
    $taskOldState=Get-Content -LiteralPath $taskStatePath -Raw -Encoding UTF8|ConvertFrom-Json
    $taskSession=Invoke-RestMethod ($taskBase+'/session')
    $taskHeaders=@{'X-DumperSG-Token'=$taskSession.token}
    $taskProfile=Invoke-RestMethod ($taskBase+'/profiles') -Method Post -Headers $taskHeaders -ContentType 'application/json' -Body (@{name='Restart QA';host='db.invalid';port=3306;user='qa';password='qa-only';database='source';ssl=$false;threads=2}|ConvertTo-Json)
    $taskDirectory=Join-Path $taskQa 'backups'
    New-Item -ItemType Directory -Path $taskDirectory -Force | Out-Null
    $taskSettings=Invoke-RestMethod ($taskBase+'/settings') -Method Patch -Headers $taskHeaders -ContentType 'application/json' -Body (@{default_backup_dir=$taskDirectory}|ConvertTo-Json)
    $taskReply=Invoke-RestMethod ($taskBase+'/application/restart') -Method Post -Headers $taskHeaders -ContentType 'application/json' -Body '{}'
    if($taskReply.status -ne 'restarting' -or $taskReply.instance_id -ne $taskBefore.instance_id){throw 'Restart acceptance was not confirmed.'}
    $taskAfter=Wait-RestartInstance $taskBefore.instance_id
    if($taskWrapper.HasExited){throw 'Supervisor exited on a requested restart.'}
    $taskNewState=Get-Content -LiteralPath $taskStatePath -Raw -Encoding UTF8|ConvertFrom-Json
    if($taskNewState.pid -eq $taskOldState.pid){throw 'Backend process was not replaced.'}
    $taskOldProcess=Get-Process -Id $taskOldState.pid -ErrorAction SilentlyContinue
    if($taskOldProcess -and $taskOldProcess.StartTime.ToUniversalTime().Ticks.ToString() -eq $taskOldState.startedTicks){throw 'Old backend process survived.'}
    $taskProfiles=Invoke-RestMethod ($taskBase+'/profiles')
    if($taskProfiles.Count -ne 1 -or $taskProfiles[0].id -ne $taskProfile.id -or -not $taskProfiles[0].has_password){throw 'Profile was not preserved.'}
    if((Invoke-RestMethod ($taskBase+'/settings')).default_backup_dir -ne $taskSettings.default_backup_dir){throw 'Settings were not preserved.'}
    $taskBlocked=$false
    try { Invoke-RestMethod ($taskBase+'/application/restart') -Method Post -Headers $taskHeaders -ContentType 'application/json' -Body '{}'|Out-Null }
    catch { if([int]$_.Exception.Response.StatusCode -eq 403){$taskBlocked=$true}else{throw} }
    if(-not $taskBlocked){throw 'Previous session token remained valid.'}
    $taskCommand=Start-Process -FilePath $taskPowerShell -ArgumentList @('-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',('"'+(Join-Path $taskRoot 'scripts\restart.ps1')+'"'),'-ConfigPath',('"'+$taskConfigPath+'"')) -PassThru -Wait -WindowStyle Hidden -RedirectStandardOutput (Join-Path $taskQa 'command-output.log') -RedirectStandardError (Join-Path $taskQa 'command-error.log')
    if($taskCommand.ExitCode -ne 0){throw 'Single-command restart failed.'}
    $null=Wait-RestartInstance $taskAfter.instance_id
    Write-Output 'PASS: HTTP restart, new process and session, preserved profiles/settings, live supervisor and single-command restart in Windows PowerShell 5.'
    Write-Output ('Artifacts: '+$taskQa)
} finally {
    # Stop this isolated supervisor before ending only its last verified child.
    if(-not $taskWrapper.HasExited){$taskWrapper.Kill();$null=$taskWrapper.WaitForExit(5000)}
    $taskStatePath=Join-Path $taskConfig.dataDir 'startup\process.json'
    if(Test-Path -LiteralPath $taskStatePath){
        $taskState=Get-Content -LiteralPath $taskStatePath -Raw -Encoding UTF8|ConvertFrom-Json
        $taskChild=Get-Process -Id $taskState.pid -ErrorAction SilentlyContinue
        if($taskChild -and $taskChild.Path -eq $Executable -and $taskChild.StartTime.ToUniversalTime().Ticks.ToString() -eq $taskState.startedTicks){$taskChild.Kill();$null=$taskChild.WaitForExit(5000)}
    }
    $taskWrapper.Dispose()
}
