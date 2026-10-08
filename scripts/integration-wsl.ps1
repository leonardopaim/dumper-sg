param([string]$WslDistro = 'Ubuntu', [string]$MySQLImage = 'mysql:8.4.3', [string]$DumperImage = 'mydumper/mydumper:latest')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskExe = Join-Path $taskRoot 'dist\dumpersg.exe'
if (-not (Test-Path -LiteralPath $taskExe)) { throw 'Execute scripts/build.ps1 primeiro.' }
$taskNonce = [guid]::NewGuid().ToString('N')
$taskContainer = 'dumpersg-integration-' + $taskNonce
$taskLabel = 'com.dumpersg.integration=' + $taskNonce
$taskPassword = 'qa-' + $taskNonce
$taskData = Join-Path $taskRoot ('data\integration-wsl-' + $taskNonce)
New-Item -ItemType Directory -Force -Path $taskData | Out-Null
$taskProcess = $null
$taskCreated = $false
$taskOwnedJobs = [System.Collections.Generic.List[string]]::new()
function Invoke-TaskDocker([string[]]$DockerArguments) {
    $taskOutput = & wsl.exe --distribution $WslDistro --exec docker @DockerArguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw ('Docker WSL falhou: ' + ($taskOutput -join "`n")) }
    return ($taskOutput -join "`n")
}
function Get-TaskPort {
    $taskListener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $taskListener.Start()
    try { return $taskListener.LocalEndpoint.Port } finally { $taskListener.Stop() }
}
function Invoke-TaskAPI([string]$Route, [object]$Body) {
    $taskJSON = $Body | ConvertTo-Json -Depth 8 -Compress
    $taskResponse = Invoke-RestMethod -Uri ($taskBase + $Route) -Method Post -Headers $taskHeaders -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($taskJSON))
    if ($taskResponse.kind -and $taskResponse.id) { $taskOwnedJobs.Add($taskResponse.id) }
    return $taskResponse
}
function Wait-DumperJob([object]$Job, [string]$ExpectedStatus = 'succeeded') {
    for ($taskTry = 0; $taskTry -lt 180; $taskTry++) {
        $taskJob = Invoke-RestMethod -Uri ($taskBase + '/api/v1/jobs/' + $Job.id)
        if ($taskJob.status -eq $ExpectedStatus) { return $taskJob }
        if ($taskJob.status -in @('succeeded','failed','cancelled')) {
            $taskEvents = Invoke-RestMethod -Uri ($taskBase + '/api/v1/jobs/' + $Job.id + '/events')
            throw ('Job ' + $taskJob.kind + ' falhou: ' + $taskJob.message + "`n" + ($taskEvents | ConvertTo-Json -Depth 6))
        }
        Start-Sleep -Milliseconds 500
    }
    throw ('Timeout no job ' + $Job.id)
}
try {
    # Existing local images only. No pulls and no access to user containers/databases.
    Invoke-TaskDocker @('image','inspect',$MySQLImage,'--format','{{.Id}}') | Out-Null
    Invoke-TaskDocker @('image','inspect',$DumperImage,'--format','{{.Id}}') | Out-Null
    $taskMySQLPort = Get-TaskPort
    $taskCreated = $true
    Invoke-TaskDocker @('run','--detach','--name',$taskContainer,'--label',$taskLabel,'--publish',('127.0.0.1:' + $taskMySQLPort + ':3306'),'--env',('MYSQL_ROOT_PASSWORD=' + $taskPassword),'--env','MYSQL_ROOT_HOST=%',$MySQLImage) | Out-Null
    $taskReady = $false
    for ($taskTry = 0; $taskTry -lt 90; $taskTry++) {
        try {
            Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--execute=SELECT 1;') | Out-Null
            $taskReady = $true
            break
        } catch { Start-Sleep -Milliseconds 500 }
    }
    if (-not $taskReady) { throw 'MySQL descartavel nao ficou pronto.' }
    $taskSeed = "CREATE DATABASE qa_source; CREATE TABLE qa_source.sample (id INT PRIMARY KEY, label VARCHAR(30)); INSERT INTO qa_source.sample VALUES (1,'alpha'),(2,'beta');"
    Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),('--execute=' + $taskSeed)) | Out-Null
    $taskPort = Get-TaskPort
    $taskBase = 'http://127.0.0.1:' + $taskPort
    $taskArguments = @('-addr',('127.0.0.1:' + $taskPort),'-data-dir',('"' + $taskData + '"'),'-docker-runtime','wsl','-wsl-distro',('"' + $WslDistro + '"'),'-mysql-image',$MySQLImage,'-docker-image',$DumperImage)
    $taskProcess = Start-Process -FilePath $taskExe -ArgumentList $taskArguments -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskData 'server.stdout.log') -RedirectStandardError (Join-Path $taskData 'server.stderr.log')
    $taskReady = $false
    for ($taskTry = 0; $taskTry -lt 60; $taskTry++) {
        try { Invoke-RestMethod -Uri ($taskBase + '/api/v1/health') | Out-Null; $taskReady = $true; break } catch { Start-Sleep -Milliseconds 200 }
    }
    if (-not $taskReady) { throw 'Aplicacao nao iniciou.' }
    $taskDiagnostics = Invoke-RestMethod -Uri ($taskBase + '/api/v1/diagnostics')
    if (-not $taskDiagnostics.available -or $taskDiagnostics.message -notmatch 'WSL') { throw 'Daemon WSL nao detectado.' }
    $taskSession = Invoke-RestMethod -Uri ($taskBase + '/api/v1/session')
    $taskHeaders = @{'X-DumperSG-Token' = $taskSession.token}
    # localhost intentionally checks TCP rather than a container-local Unix socket.
    $taskProfile = Invoke-TaskAPI '/api/v1/profiles' @{name='WSL integration';host='localhost';port=$taskMySQLPort;user='root';password=$taskPassword;database='qa_source';threads=2;ssl=$false}
    Wait-DumperJob (Invoke-TaskAPI ('/api/v1/profiles/' + $taskProfile.id + '/test') @{}) | Out-Null
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/databases' @{profile_id=$taskProfile.id;database='qa_restore'}) | Out-Null
    $taskBackup = Wait-DumperJob (Invoke-TaskAPI '/api/v1/backups' @{profile_id=$taskProfile.id;database='qa_source';threads=2;compress=$true;non_locking=$true})
    if (-not (Test-Path -LiteralPath (Join-Path $taskBackup.path 'metadata'))) { throw 'Backup nao gravou metadata no Windows.' }
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskBackup.path;target_database='qa_restore';threads=2;overwrite_tables=$false}) | Out-Null
    $taskRows = Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--batch','--skip-column-names',"--execute=SELECT GROUP_CONCAT(CONCAT(id,':',label) ORDER BY id SEPARATOR '|') FROM qa_restore.sample;")
    if ($taskRows -notmatch '1:alpha\|2:beta') { throw 'Dados restaurados divergentes.' }
    # Repeat against existing tables: preserve-by-default must fail; explicit
    # overwrite must replace only backed-up tables in the isolated target.
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskBackup.path;target_database='qa_restore';threads=2;overwrite_tables=$false}) 'failed' | Out-Null
    Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),"--execute=UPDATE qa_restore.sample SET label='changed'; INSERT INTO qa_restore.sample VALUES (3,'extra'); CREATE TABLE qa_restore.unrelated (id INT); INSERT INTO qa_restore.unrelated VALUES (7);") | Out-Null
    $taskOverwrite = Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskBackup.path;target_database='qa_restore';threads=2;overwrite_tables=$true})
    $taskRows = Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--batch','--skip-column-names',"--execute=SELECT GROUP_CONCAT(CONCAT(id,':',label) ORDER BY id SEPARATOR '|') FROM qa_restore.sample; SELECT SUM(id) FROM qa_restore.unrelated; SELECT GROUP_CONCAT(CONCAT(id,':',label) ORDER BY id SEPARATOR '|') FROM qa_source.sample;")
    $taskValues = @($taskRows -split "`n" | Where-Object { $_ -notmatch '^mysql:' -and $_.Trim() } | ForEach-Object { $_.Trim() })
    if ($taskValues.Count -ne 3 -or $taskValues[0] -ne '1:alpha|2:beta' -or $taskValues[1] -ne '7' -or $taskValues[2] -ne '1:alpha|2:beta') { throw 'Sobrescrita nao preservou origem/tabela alheia ou nao substituiu os dados.' }
    $taskLog = Get-Content -LiteralPath (Join-Path $taskData ('logs\' + $taskOverwrite.id + '.log'))
    if (-not ($taskLog | Where-Object { $_ -match ' info \*\* Message:' }) -or ($taskLog | Where-Object { $_ -match ' error \*\* Message:' })) { throw 'Mensagens informativas classificadas incorretamente.' }
    Write-Output 'Integracao WSL aprovada: backup/restore, falha sem sobrescrita em tabelas existentes, sobrescrita explicita com dados conferidos e classificacao de logs.'
    Write-Output ('Artefatos isolados: ' + $taskData)
} finally {
    if ($taskProcess -and -not $taskProcess.HasExited -and $taskHeaders) {
        foreach ($taskJobId in @($taskOwnedJobs.ToArray())) {
            try {
                $taskPending = Invoke-RestMethod -Uri ($taskBase + '/api/v1/jobs/' + $taskJobId)
                if ($taskPending.status -in @('running','cancel_requested')) {
                    Invoke-TaskAPI ('/api/v1/jobs/' + $taskJobId + '/cancel') @{} | Out-Null
                    for ($taskTry = 0; $taskTry -lt 180; $taskTry++) {
                        $taskPending = Invoke-RestMethod -Uri ($taskBase + '/api/v1/jobs/' + $taskJobId)
                        if ($taskPending.status -notin @('running','cancel_requested')) { break }
                        Start-Sleep -Milliseconds 500
                    }
                    if ($taskPending.status -in @('running','cancel_requested')) { Write-Warning ('Timeout ao aguardar limpeza do job ' + $taskJobId + '; confira o container dumpersg-' + $taskJobId + ' no daemon ' + $WslDistro) }
                }
                if ($taskPending.cleanup_required) { Write-Warning ('Limpeza pendente no job ' + $taskJobId + '; confira os artefatos em ' + $taskData) }
            } catch { Write-Warning ('Nao foi possivel conferir limpeza do job ' + $taskJobId) }
        }
    }
    if ($taskProcess -and -not $taskProcess.HasExited) { Stop-Process -Id $taskProcess.Id -ErrorAction SilentlyContinue }
    if ($taskCreated) {
        $taskOwner = & wsl.exe --distribution $WslDistro --exec docker inspect --format '{{index .Config.Labels "com.dumpersg.integration"}}' $taskContainer 2>$null
        if (($taskOwner -join '').Trim() -eq $taskNonce) {
            & wsl.exe --distribution $WslDistro --exec docker rm --force --volumes $taskContainer | Out-Null
        }
    }
}
