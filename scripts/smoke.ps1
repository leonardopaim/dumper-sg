param([string]$Executable = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
if (-not $Executable) { $Executable = Join-Path $taskRoot 'dist\dumpersg.exe' }
if (-not (Test-Path -LiteralPath $Executable)) { throw 'Compile a aplicação antes do smoke test.' }
$taskListener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$taskListener.Start()
$taskPort = $taskListener.LocalEndpoint.Port
$taskListener.Stop()
$taskData = Join-Path $taskRoot ('data\smoke-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $taskData | Out-Null
$taskBase = 'http://127.0.0.1:' + $taskPort
$taskArgs = @('-addr', ('127.0.0.1:' + $taskPort), '-data-dir', ('"' + $taskData + '"'))
$taskProcess = Start-Process -FilePath $Executable -ArgumentList $taskArgs -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $taskData 'stdout.log') -RedirectStandardError (Join-Path $taskData 'stderr.log')
try {
    $taskReady = $false
    for ($taskAttempt = 0; $taskAttempt -lt 60; $taskAttempt++) {
        try {
            $taskHealth = Invoke-RestMethod ($taskBase + '/api/v1/health') -TimeoutSec 1
            if ($taskHealth.status -eq 'ok') { $taskReady = $true; break }
        } catch { Start-Sleep -Milliseconds 100 }
    }
    if (-not $taskReady) { throw 'Servidor não iniciou; consulte stderr.log no diretório do teste.' }
    $taskSession = Invoke-RestMethod ($taskBase + '/api/v1/session')
    $taskHeaders = @{ 'X-DumperSG-Token' = $taskSession.token }
    $taskProfileBody = @{ name='smoke'; host='db.example.invalid'; port=3306; user='root'; password='smoke-secret'; database='production'; threads=8; ssl=$false } | ConvertTo-Json
    $taskProfile = Invoke-RestMethod ($taskBase + '/api/v1/profiles') -Method Post -Headers $taskHeaders -ContentType 'application/json' -Body $taskProfileBody
    if (-not $taskProfile.has_password -or $taskProfile.PSObject.Properties.Name -contains 'password') { throw 'Perfil público expôs senha ou perdeu estado.' }
    $taskProfiles = Invoke-RestMethod ($taskBase + '/api/v1/profiles')
    if (@($taskProfiles).Count -ne 1) { throw 'CRUD de perfis falhou.' }
    $taskRequest = @{ profile_id=$taskProfile.id; database='isolated' } | ConvertTo-Json
    $taskBlocked = $false
    try { Invoke-RestMethod ($taskBase + '/api/v1/databases') -Method Post -Headers $taskHeaders -ContentType 'application/json' -Body $taskRequest | Out-Null }
    catch { if ([int]$_.Exception.Response.StatusCode -eq 400) { $taskBlocked=$true } else { throw } }
    if (-not $taskBlocked) { throw 'Criação de banco remoto não foi bloqueada no core.' }
    $taskNoToken = $false
    try { Invoke-RestMethod ($taskBase + '/api/v1/profiles') -Method Post -ContentType 'application/json' -Body $taskProfileBody | Out-Null }
    catch { if ([int]$_.Exception.Response.StatusCode -eq 403) { $taskNoToken=$true } else { throw } }
    if (-not $taskNoToken) { throw 'Mutação sem token aceita.' }
    $taskPage = Invoke-WebRequest ($taskBase + '/profiles')
    if ($taskPage.StatusCode -ne 200 -or $taskPage.Content -notmatch 'id="root"') { throw 'Frontend embarcado/rota SPA falhou.' }
    if ($taskPage.Content -notmatch 'rel="icon"[^>]+href="/favicon.svg"') { throw 'Favicon ausente no frontend embarcado.' }
    $taskIcon = Invoke-WebRequest ($taskBase + '/favicon.svg')
    if ($taskIcon.StatusCode -ne 200 -or $taskIcon.Headers['Content-Type'] -notlike 'image/svg+xml*' -or $taskIcon.Content -notmatch '<svg') { throw 'Favicon embarcado nao foi servido corretamente.' }
    Invoke-RestMethod ($taskBase + '/api/v1/profiles/' + $taskProfile.id) -Method Delete -Headers $taskHeaders | Out-Null
    Write-Output 'Smoke test aprovado: executável, sessão, CRUD, senha oculta, proteção do core e frontend embarcado.'
    Write-Output ('Dados isolados do teste: ' + $taskData)
} finally {
    if (-not $taskProcess.HasExited) { Stop-Process -Id $taskProcess.Id }
    $taskProcess.Dispose()
}
