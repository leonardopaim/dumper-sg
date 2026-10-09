param([string]$Installer = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
if (-not $Installer) { $Installer = Join-Path $taskRoot 'dist\releases\DumperSG-Setup-1.0.0-windows-x64.exe' }
if (-not (Test-Path -LiteralPath $Installer -PathType Leaf)) { throw 'Build the installer first.' }
# This smoke installs only to an isolated workspace folder, without shortcuts or startup.
$taskRegistryPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{EC0F6C8E-8CD6-40EA-B7A6-83B43A40F30D}_is1'
if (Test-Path -LiteralPath $taskRegistryPath) { throw 'An installed DumperSG package already exists; use a clean Windows test account for this smoke.' }
$taskQa = Join-Path $taskRoot ('data\installer-qa-' + [guid]::NewGuid().ToString('N'))
$taskInstall = [IO.Path]::GetFullPath((Join-Path $taskQa 'install path'))
$taskData = [IO.Path]::GetFullPath((Join-Path $taskQa 'user data'))
if (-not $taskInstall.StartsWith([IO.Path]::GetFullPath($taskQa) + '\') -or -not $taskData.StartsWith([IO.Path]::GetFullPath($taskQa) + '\')) { throw 'Unsafe QA paths.' }
New-Item -ItemType Directory -Path $taskQa,$taskData -Force | Out-Null
$taskListener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
$taskListener.Start()
$taskAddress = '127.0.0.1:' + $taskListener.LocalEndpoint.Port
$taskListener.Stop()
$taskBase = 'http://' + $taskAddress
$taskLauncher = Join-Path $taskInstall 'DumperSG.exe'
$taskLauncherArgs = @('-background','-quiet','-addr',$taskAddress,'-data-dir',('"'+$taskData+'"'))
$taskProcess = $null

function Install-QAPackage {
    $taskArgs = @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/NOICONS','/TASKS=""',('/GROUP="DumperSG-QA-'+[IO.Path]::GetFileName($taskQa)+'"'),('/DIR="'+$taskInstall+'"'),('/LOG="'+(Join-Path $taskQa 'install.log')+'"'))
    $taskSetup = Start-Process -FilePath $Installer -ArgumentList $taskArgs -Wait -PassThru -WindowStyle Hidden
    if ($taskSetup.ExitCode -ne 0) { throw ('Installer returned ' + $taskSetup.ExitCode) }
    foreach ($taskFile in @('DumperSG.exe','dumpersg-core.exe','DumperSG.ico','LEIA-ME.txt','unins000.exe')) {
        if (-not (Test-Path -LiteralPath (Join-Path $taskInstall $taskFile) -PathType Leaf)) { throw ('Missing installed file: ' + $taskFile) }
    }
}
function Start-QALauncher {
    $taskStarted = Start-Process -FilePath $taskLauncher -ArgumentList $taskLauncherArgs -PassThru -WindowStyle Hidden
    for ($taskTry=0; $taskTry -lt 90; $taskTry++) {
        if ($taskStarted.HasExited) { throw 'Installed launcher exited during startup.' }
        try {
            $taskApp = Invoke-RestMethod ($taskBase + '/api/v1/application') -TimeoutSec 1
            $taskStatePath = Join-Path $taskData 'installed-launcher\process.json'
            if (Test-Path -LiteralPath $taskStatePath) {
                $taskState = Get-Content -LiteralPath $taskStatePath -Raw | ConvertFrom-Json
                if ($taskState.instance_id -eq $taskApp.instance_id) { return $taskStarted }
            }
        } catch {}
        Start-Sleep -Milliseconds 200
    }
    throw 'Installed launcher did not become ready.'
}
function Stop-QALauncher {
    $taskStop = Start-Process -FilePath $taskLauncher -ArgumentList @('-shutdown','-quiet','-addr',$taskAddress,'-data-dir',('"'+$taskData+'"')) -Wait -PassThru -WindowStyle Hidden
    if ($taskStop.ExitCode -ne 0) { throw ('Installed shutdown returned ' + $taskStop.ExitCode) }
    if (-not $taskProcess.WaitForExit(15000)) { throw 'Supervisor did not exit after safe shutdown.' }
}
try {
    Install-QAPackage
    $taskProcess = Start-QALauncher
    $taskApp = Invoke-RestMethod ($taskBase + '/api/v1/application')
    $taskInstance = $taskApp.instance_id
    $taskSession = Invoke-RestMethod ($taskBase + '/api/v1/session')
    $taskHeaders = @{'X-DumperSG-Token'=$taskSession.token}
    $taskBody = @{name='QA installer';host='db.example.invalid';port=3306;user='qa';password='qa-only-secret';database='source';threads=2;ssl=$false} | ConvertTo-Json
    $taskProfile = Invoke-RestMethod ($taskBase + '/api/v1/profiles') -Method Post -ContentType 'application/json' -Headers $taskHeaders -Body $taskBody
    $taskMarker = Join-Path $taskData 'preserve-me.txt'
    [IO.File]::WriteAllText($taskMarker,'QA user data must survive upgrade/uninstall',[Text.UTF8Encoding]::new($false))
    $taskAgain = Start-Process -FilePath $taskLauncher -ArgumentList $taskLauncherArgs -PassThru -WindowStyle Hidden
    if (-not $taskAgain.WaitForExit(10000) -or $taskAgain.ExitCode -ne 0) { throw 'Second launch did not reuse the running application.' }
    if ((Invoke-RestMethod ($taskBase + '/api/v1/application')).instance_id -ne $taskInstance) { throw 'Second launch replaced the running core.' }
    Invoke-RestMethod ($taskBase + '/api/v1/application/restart') -Method Post -ContentType 'application/json' -Body '{}' -Headers $taskHeaders | Out-Null
    $taskRestarted = $false
    for ($taskTry=0; $taskTry -lt 90; $taskTry++) {
        try {
            $taskNew = Invoke-RestMethod ($taskBase + '/api/v1/application') -TimeoutSec 1
            $taskState = Get-Content -LiteralPath (Join-Path $taskData 'installed-launcher\process.json') -Raw | ConvertFrom-Json
            if ($taskNew.instance_id -ne $taskInstance -and $taskState.instance_id -eq $taskNew.instance_id) { $taskRestarted=$true; break }
        } catch {}
        Start-Sleep -Milliseconds 200
    }
    if (-not $taskRestarted) { throw 'Installed supervisor did not restart the core.' }
    Stop-QALauncher
    Install-QAPackage
    if (-not (Test-Path -LiteralPath $taskMarker)) { throw 'Upgrade removed user data.' }
    $taskProcess = Start-QALauncher
    $taskSaved = Invoke-RestMethod ($taskBase + '/api/v1/profiles/' + $taskProfile.id)
    if ($taskSaved.name -ne 'QA installer' -or -not $taskSaved.has_password) { throw 'Upgrade changed the profile.' }
    Stop-QALauncher
    $taskDb = Join-Path $taskData 'dumper_sg.sqlite3'
    $taskBefore = (Get-FileHash -LiteralPath $taskDb -Algorithm SHA256).Hash
    $taskUninstaller = Join-Path $taskInstall 'unins000.exe'
    $taskUninstall = Start-Process -FilePath $taskUninstaller -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',('/LOG="'+(Join-Path $taskQa 'uninstall.log')+'"')) -PassThru -WindowStyle Hidden
    if (-not $taskUninstall.WaitForExit(30000) -or $taskUninstall.ExitCode -ne 0) { throw 'Uninstall did not complete.' }
    for ($taskTry=0; $taskTry -lt 30 -and (Test-Path -LiteralPath $taskLauncher); $taskTry++) { Start-Sleep -Milliseconds 100 }
    if (Test-Path -LiteralPath $taskLauncher) { throw 'Uninstaller left the application executable.' }
    if (-not (Test-Path -LiteralPath $taskMarker) -or (Get-FileHash -LiteralPath $taskDb -Algorithm SHA256).Hash -ne $taskBefore) { throw 'Uninstall modified user data.' }
    if (Test-Path -LiteralPath $taskRegistryPath) { throw 'QA uninstall registration remains.' }
    Write-Output 'Installer smoke passed: install, duplicate launch, supervised restart, safe shutdown, upgrade and uninstall; user data preserved.'
    Write-Output ('Isolated QA files: ' + $taskQa)
} catch {
    Write-Output ('QA failed; preserve logs and installation at ' + $taskQa)
    throw
}
