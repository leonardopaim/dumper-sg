param([string]$Executable = '', [ValidateSet('auto','native','wsl')][string]$DockerRuntime = 'auto', [string]$WslDistro = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
if (-not $Executable) { $Executable = Join-Path $taskRoot 'dist\dumpersg.exe' }
$Executable = [IO.Path]::GetFullPath($Executable)
$taskQaDir = Join-Path $taskRoot ('data\startup qa ' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $taskQaDir,(Join-Path $taskQaDir 'logs'),(Join-Path $taskQaDir 'conf'),(Join-Path $taskQaDir 'temp') | Out-Null
$taskPorts = @()
$taskListeners = @()
try {
    for ($taskIndex=0; $taskIndex -lt 2; $taskIndex++) {
        $taskSocket = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
        $taskSocket.Start(); $taskPorts += $taskSocket.LocalEndpoint.Port; $taskListeners += $taskSocket
    }
} finally { foreach ($taskListener in $taskListeners) { $taskListener.Stop() } }
$taskBackendPort,$taskProxyPort = $taskPorts
$taskBackendUrl = 'http://127.0.0.1:' + $taskBackendPort
$taskProxyUrl = 'http://127.0.0.1:' + $taskProxyPort
$taskConfig = [ordered]@{ version=2; executable=$Executable; address=('127.0.0.1:' + $taskBackendPort); dataDir=(Join-Path $taskQaDir ('app data ' + [char]0xE7)); dockerRuntime=$DockerRuntime; wslDistro=$WslDistro; mysqlImage='mysql:8.4.3'; dumperImage='mydumper/mydumper:latest' }
$taskConfigPath = Join-Path $taskQaDir 'launcher.json'
[IO.File]::WriteAllText($taskConfigPath,($taskConfig | ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$taskProxyConfig = [IO.File]::ReadAllText((Join-Path $taskRoot 'deploy\nginx\dumpersg.conf')).Replace('8787', [string]$taskBackendPort).Replace('8788', [string]$taskProxyPort)
# The launcher permits only the production proxy origins. Exercise that exact
# allowed Origin while reaching this isolated proxy on a random port.
$taskOrigin = 'http://127.0.0.1:8788'
$taskMainConfig = "worker_processes 1;`r`npid logs/nginx.pid;`r`nerror_log logs/error.log;`r`nevents { worker_connections 64; }`r`nhttp { include C:/nginx/conf/mime.types; $taskProxyConfig }`r`n"
[IO.File]::WriteAllText((Join-Path $taskQaDir 'conf\qa.conf'),$taskMainConfig,[Text.UTF8Encoding]::new($false))
$taskPrefix = $taskQaDir.Replace('\','/') + '/'
$taskPowerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$taskWrapper = Start-Process -FilePath $taskPowerShell -ArgumentList @('-NoProfile','-NonInteractive','-WindowStyle','Hidden','-ExecutionPolicy','Bypass','-File',('"'+(Join-Path $taskRoot 'scripts\run-background.ps1')+'"'),'-ConfigPath',('"'+$taskConfigPath+'"')) -PassThru -WindowStyle Hidden -RedirectStandardError (Join-Path $taskQaDir 'wrapper-error.log')
$null = $taskWrapper.Handle
$taskNginxStarted = $false
$taskAppPid = $null
try {
    $taskReady = $false
    for ($taskAttempt=0; $taskAttempt -lt 80; $taskAttempt++) {
        try { if ((Invoke-RestMethod ($taskBackendUrl + '/api/v1/health') -TimeoutSec 1).status -eq 'ok') { $taskReady=$true; break } } catch { Start-Sleep -Milliseconds 100 }
    }
    if (-not $taskReady) { throw 'Background launcher did not start backend.' }
    if (-not (Test-Path -LiteralPath (Join-Path $taskConfig.dataDir 'dumper_sg.sqlite3'))) { throw 'Launcher changed the Unicode data path.' }
    $taskAppPid = (Get-NetTCPConnection -State Listen -LocalPort $taskBackendPort).OwningProcess
    $taskNginxTest = Start-Process -FilePath 'C:\nginx\nginx.exe' -ArgumentList @('-p',('"'+$taskPrefix+'"'),'-c','conf/qa.conf','-t') -PassThru -Wait -WindowStyle Hidden -RedirectStandardError (Join-Path $taskQaDir 'nginx-test.log')
    if ($taskNginxTest.ExitCode -ne 0) { throw 'Isolated Nginx syntax check failed.' }
    $taskNginx = Start-Process -FilePath 'C:\nginx\nginx.exe' -ArgumentList @('-p',('"'+$taskPrefix+'"'),'-c','conf/qa.conf') -PassThru -WindowStyle Hidden
    $taskNginxStarted = $true
    $taskReady = $false
    for ($taskAttempt=0; $taskAttempt -lt 50; $taskAttempt++) {
        try { if ((Invoke-RestMethod ($taskProxyUrl + '/api/v1/health') -TimeoutSec 1).status -eq 'ok') { $taskReady=$true; break } } catch { Start-Sleep -Milliseconds 100 }
    }
    if (-not $taskReady) { throw 'Isolated Nginx proxy did not start.' }
    $taskPage = Invoke-WebRequest ($taskProxyUrl + '/backup') -UseBasicParsing
    if ($taskPage.Content -notmatch 'href="/favicon.svg"') { throw 'Embedded favicon link missing.' }
    $taskIcon = Invoke-WebRequest ($taskProxyUrl + '/favicon.svg') -UseBasicParsing
    if ($taskIcon.Headers['Content-Type'] -notlike 'image/svg+xml*') { throw 'SVG not served through proxy.' }
    $taskSession = Invoke-RestMethod ($taskProxyUrl + '/api/v1/session') -Headers @{ Origin=$taskOrigin }
    $taskHeaders = @{ Origin=$taskOrigin; 'X-DumperSG-Token'=$taskSession.token }
    $taskBody = @{name='isolated nginx QA';host='db.invalid';port=3306;user='qa';password='qa-secret';database='production';threads=2;ssl=$false} | ConvertTo-Json
    $taskProfile = Invoke-RestMethod ($taskProxyUrl + '/api/v1/profiles') -Method Post -ContentType 'application/json' -Headers $taskHeaders -Body $taskBody
    if ($taskProfile.id -lt 1 -or -not $taskProfile.has_password) { throw 'Proxy mutation failed.' }
    Invoke-RestMethod ($taskProxyUrl + '/api/v1/profiles/' + $taskProfile.id) -Method Delete -Headers $taskHeaders | Out-Null
    foreach ($taskCase in @(@{ Host='evil.example'; Origin=$taskOrigin },@{ Origin='http://evil.example' })) {
        $taskBlocked = $false
        try { Invoke-WebRequest ($taskProxyUrl + '/api/v1/session') -Headers $taskCase -UseBasicParsing | Out-Null }
        catch { if ([int]$_.Exception.Response.StatusCode -eq 403) { $taskBlocked=$true } else { throw } }
        if (-not $taskBlocked) { throw 'Proxy accepted an external Host or Origin.' }
    }
    $taskBlocked=$false
    try { Invoke-RestMethod ($taskProxyUrl + '/api/v1/profiles') -Method Post -ContentType 'application/json' -Body $taskBody | Out-Null }
    catch { if ([int]$_.Exception.Response.StatusCode -eq 403) { $taskBlocked=$true } else { throw } }
    if (-not $taskBlocked) { throw 'Proxy accepted a mutation without session token.' }
    # Deliberately terminate only this isolated child to verify exit propagation.
    Stop-Process -Id $taskAppPid
    $taskAppPid = $null
    if (-not $taskWrapper.WaitForExit(10000)) { throw 'Launcher did not follow child termination.' }
    if ($taskWrapper.ExitCode -eq 0) { throw 'Launcher hid the child failure from Task Scheduler.' }
    Write-Output 'PASS: Windows PowerShell 5 launcher, path with spaces, lifetime and exit code; embedded SVG; isolated Nginx routes, session CRUD, Host/Origin/token protections.'
    Write-Output ('Artifacts: ' + $taskQaDir)
} finally {
    if ($taskNginxStarted) { Start-Process -FilePath 'C:\nginx\nginx.exe' -ArgumentList @('-p',('"'+$taskPrefix+'"'),'-c','conf/qa.conf','-s','quit') -Wait -WindowStyle Hidden | Out-Null }
    if ($taskAppPid) { Stop-Process -Id $taskAppPid -ErrorAction SilentlyContinue }
    if (-not $taskWrapper.HasExited) { Stop-Process -Id $taskWrapper.Id }
    $taskWrapper.Dispose()
}
