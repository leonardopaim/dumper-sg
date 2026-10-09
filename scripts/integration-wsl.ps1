param([string]$WslDistro = 'Ubuntu', [string]$MySQLImage = 'mysql:8.4.3', [string]$DumperImage = 'mydumper/mydumper:latest', [string]$Executable = '')
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskExe = if ($Executable) { [System.IO.Path]::GetFullPath($Executable) } else { Join-Path $taskRoot 'dist\dumpersg.exe' }
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
function Invoke-TaskAPI([string]$Route, [object]$Body, [string]$Method = 'Post') {
    $taskJSON = $Body | ConvertTo-Json -Depth 8 -Compress
    $taskResponse = Invoke-RestMethod -Uri ($taskBase + $Route) -Method $Method -Headers $taskHeaders -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($taskJSON))
    if ($taskResponse.kind -in @('backup','restore','table_list','connection_test','create_database') -and $taskResponse.id -is [string] -and $taskResponse.id -match '^[a-f0-9]{32}$') { $taskOwnedJobs.Add($taskResponse.id) }
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
    # Metadata and selected exports, including a literal name with regex/list
    # metacharacters. Only this disposable MySQL receives fixture changes.
    $taskCatalogSeed = 'CREATE TABLE qa_source.large_payload (id INT, payload MEDIUMTEXT) ENGINE=MyISAM; INSERT INTO qa_source.large_payload VALUES (1,REPEAT(''x'',300000)); CREATE TABLE qa_source.`literal,+.[x]` (id INT); INSERT INTO qa_source.`literal,+.[x]` VALUES (11); CREATE VIEW qa_source.sample_view AS SELECT id FROM qa_source.sample;'
    Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),('--execute=' + $taskCatalogSeed)) | Out-Null
    $taskCatalogJob = Wait-DumperJob (Invoke-TaskAPI '/api/v1/tables' @{profile_id=$taskProfile.id;database='qa_source';ssl=$false})
    $taskCatalog = Invoke-RestMethod -Uri ($taskBase + '/api/v1/jobs/' + $taskCatalogJob.id + '/tables')
    if ($taskCatalog.Count -ne 4 -or $taskCatalog[0].name -ne 'large_payload' -or $taskCatalog[0].size_bytes -lt 300000) { throw ('Catalogo incompleto ou sem ordem por tamanho: total=' + $taskCatalog.Count + ', primeiro=' + $taskCatalog[0].name + ', tamanho=' + $taskCatalog[0].size_bytes) }
    $taskCatalogLog = Get-Content -LiteralPath (Join-Path $taskData ('logs\' + $taskCatalogJob.id + '.log'))
    if (-not ($taskCatalogLog | Where-Object { $_ -match ' warning mysql: \[Warning\]' }) -or ($taskCatalogLog | Where-Object { $_ -match ' error mysql: \[Warning\]' })) { throw 'Aviso do MySQL classificado como erro.' }
    $taskView = @($taskCatalog | Where-Object { $_.name -eq 'sample_view' })
    if ($taskView.Count -ne 1 -or $taskView[0].table_type -ne 'VIEW') { throw 'View ausente no catalogo.' }
    $taskViewBackup = Wait-DumperJob (Invoke-TaskAPI '/api/v1/backups' @{profile_id=$taskProfile.id;database='qa_source';threads=2;compress=$true;non_locking=$true;tables=@('sample','sample_view')})
    $taskViewCatalog = Invoke-TaskAPI '/api/v1/backups/tables' @{backup_dir=$taskViewBackup.path}
    if ($taskViewCatalog.Count -ne 2 -or @($taskViewCatalog | Where-Object { $_.name -eq 'sample_view' -and $_.table_type -eq 'VIEW' }).Count -ne 1) { throw 'View ausente ou incorreta no catalogo dos arquivos.' }
    $taskSelected = Wait-DumperJob (Invoke-TaskAPI '/api/v1/backups' @{profile_id=$taskProfile.id;database='qa_source';threads=2;compress=$true;non_locking=$true;tables=@('sample','literal,+.[x]')})
    $taskBackupCatalog = Invoke-TaskAPI '/api/v1/backups/tables' @{backup_dir=$taskSelected.path}
    if ($taskBackupCatalog.Count -ne 2 -or @($taskBackupCatalog | Where-Object { $_.database -ne 'qa_source' -or $_.size_bytes -le 0 }).Count -gt 0 -or @($taskBackupCatalog | Where-Object { $_.name -eq 'literal,+.[x]' }).Count -ne 1) { throw 'Catalogo dos arquivos nao preservou nomes reais/tamanhos.' }
    $taskPresetProfile = Invoke-TaskAPI ('/api/v1/profiles/' + $taskProfile.id) @{table_presets=@(@{name='Essenciais';database='qa_source';tables=@('sample','literal,+.[x]')})} 'Patch'
    if ($taskPresetProfile.table_presets.Count -ne 1 -or $taskPresetProfile.table_presets[0].tables.Count -ne 2) { throw 'Selecao do perfil nao foi persistida.' }
    Wait-DumperJob (Invoke-TaskAPI ('/api/v1/profiles/' + $taskProfile.id + '/test') @{}) | Out-Null
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskSelected.path;target_database='qa_selected';threads=2;overwrite_tables=$false}) | Out-Null
    $taskSelectedSQL = 'SELECT GROUP_CONCAT(TABLE_NAME ORDER BY TABLE_NAME SEPARATOR ''|'') FROM information_schema.TABLES WHERE TABLE_SCHEMA=''qa_selected''; SELECT SUM(id) FROM qa_selected.`literal,+.[x]`; SELECT COUNT(*) FROM qa_selected.sample;'
    $taskSelectedRows = Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--batch','--skip-column-names',('--execute=' + $taskSelectedSQL))
    $taskSelectedValues = @($taskSelectedRows -split "`n" | Where-Object { $_ -notmatch '^mysql:' -and $_.Trim() } | ForEach-Object { $_.Trim() })
    if ($taskSelectedValues.Count -ne 3 -or $taskSelectedValues[0] -ne 'literal,+.[x]|sample' -or $taskSelectedValues[1] -ne '11' -or $taskSelectedValues[2] -ne '2') { throw 'Backup seletivo incluiu objetos nao selecionados ou perdeu dados.' }
    $taskModifySelected = 'UPDATE qa_selected.sample SET label=''changed''; INSERT INTO qa_selected.sample VALUES (3,''extra''); DELETE FROM qa_selected.`literal,+.[x]`; INSERT INTO qa_selected.`literal,+.[x]` VALUES (999);'
    Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),('--execute=' + $taskModifySelected)) | Out-Null
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskSelected.path;target_database='qa_selected';threads=2;overwrite_tables=$true;tables=@(@{database='qa_source';name='sample'})}) | Out-Null
    $taskPartialSQL = 'SELECT GROUP_CONCAT(CONCAT(id,'':'',label) ORDER BY id SEPARATOR ''|'') FROM qa_selected.sample; SELECT SUM(id) FROM qa_selected.`literal,+.[x]`;'
    $taskPartialRows = Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--batch','--skip-column-names',('--execute=' + $taskPartialSQL))
    $taskPartialValues = @($taskPartialRows -split "`n" | Where-Object { $_ -notmatch '^mysql:' -and $_.Trim() } | ForEach-Object { $_.Trim() })
    if ($taskPartialValues.Count -ne 2 -or $taskPartialValues[0] -ne '1:alpha|2:beta' -or $taskPartialValues[1] -ne '999') { throw 'Restore seletivo nao preservou a tabela nao selecionada.' }
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskSelected.path;target_database='qa_only_special';threads=2;overwrite_tables=$false;tables=@(@{database='qa_source';name='literal,+.[x]'})}) | Out-Null
    $taskSpecialSQL = 'SELECT GROUP_CONCAT(TABLE_NAME) FROM information_schema.TABLES WHERE TABLE_SCHEMA=''qa_only_special''; SELECT SUM(id) FROM qa_only_special.`literal,+.[x]`;'
    $taskSpecialRows = Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--batch','--skip-column-names',('--execute=' + $taskSpecialSQL))
    $taskSpecialValues = @($taskSpecialRows -split "`n" | Where-Object { $_ -notmatch '^mysql:' -and $_.Trim() } | ForEach-Object { $_.Trim() })
    if ($taskSpecialValues.Count -ne 2 -or $taskSpecialValues[0] -ne 'literal,+.[x]' -or $taskSpecialValues[1] -ne '11') { throw 'Restore seletivo por nome real falhou.' }
    $taskAliasSQL = 'CREATE DATABASE `qa.db`; CREATE TABLE `qa.db`.sample (id INT); INSERT INTO `qa.db`.sample VALUES (13);'
    Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),('--execute=' + $taskAliasSQL)) | Out-Null
    $taskAliasBackup = Wait-DumperJob (Invoke-TaskAPI '/api/v1/backups' @{profile_id=$taskProfile.id;database='qa.db';threads=2;compress=$true;non_locking=$true;tables=@('sample')})
    $taskAliasCatalog = @(Invoke-TaskAPI '/api/v1/backups/tables' @{backup_dir=$taskAliasBackup.path})
    if ($taskAliasCatalog.Count -ne 1 -or $taskAliasCatalog[0].database -ne 'qa.db' -or $taskAliasCatalog[0].name -ne 'sample') { throw 'Alias do banco compactado nao foi recuperado.' }
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskAliasBackup.path;target_database='qa_alias';threads=2;overwrite_tables=$false;tables=@(@{database='qa.db';name='sample'})}) | Out-Null
    $taskAliasRows = Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--batch','--skip-column-names','--execute=SELECT SUM(id) FROM qa_alias.sample;')
    if (@($taskAliasRows -split "`n" | Where-Object { $_.Trim() -eq '13' }).Count -ne 1) { throw 'Restore selecionado de database aliasado falhou.' }
    $taskFiltered = Wait-DumperJob (Invoke-TaskAPI '/api/v1/backups' @{profile_id=$taskProfile.id;database='qa_source';threads=2;non_locking=$true;tables=@('sample','literal,+.[x]');ignore_regex='literal'})
    Wait-DumperJob (Invoke-TaskAPI '/api/v1/restores' @{profile_id=$taskProfile.id;backup_dir=$taskFiltered.path;target_database='qa_filtered';threads=2;overwrite_tables=$false}) | Out-Null
    $taskFilteredRows = Invoke-TaskDocker @('exec',$taskContainer,'mysql','--protocol=TCP','--host=127.0.0.1','--user=root',('--password=' + $taskPassword),'--batch','--skip-column-names',"--execute=SELECT GROUP_CONCAT(TABLE_NAME ORDER BY TABLE_NAME) FROM information_schema.TABLES WHERE TABLE_SCHEMA='qa_filtered';")
    $taskFilteredValues = @($taskFilteredRows -split "`n" | Where-Object { $_ -notmatch '^mysql:' -and $_.Trim() } | ForEach-Object { $_.Trim() })
    if ($taskFilteredValues.Count -ne 1 -or $taskFilteredValues[0] -ne 'sample') { throw 'Regex de exclusao nao foi aplicada sobre a selecao.' }
    $taskMissingCatalog = Wait-DumperJob (Invoke-TaskAPI '/api/v1/tables' @{profile_id=$taskProfile.id;database='qa_missing';ssl=$false}) 'failed'
    try {
        Invoke-RestMethod -Uri ($taskBase + '/api/v1/jobs/' + $taskMissingCatalog.id + '/tables') | Out-Null
        throw 'Catalogo de consulta falha foi retornado.'
    } catch {
        if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne 409) { throw }
    }
    Write-Output 'Integracao WSL aprovada: catalogos de origem/arquivos, selecoes persistidas no perfil, backup e restore seletivos, nomes especiais, sobrescrita restrita e dados conferidos.'
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
