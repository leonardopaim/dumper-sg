[CmdletBinding(SupportsShouldProcess=$true)]
param([string]$NginxDir = 'C:\nginx', [switch]$Remove, [switch]$NoReload)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskSource = Join-Path $taskRoot 'deploy\nginx\dumpersg.conf'
$taskNginxRoot = (Resolve-Path -LiteralPath $NginxDir).Path
$taskNginxExe = Join-Path $taskNginxRoot 'nginx.exe'
$taskSitesDir = Join-Path $taskNginxRoot 'conf\sites-enabled'
$taskTarget = Join-Path $taskSitesDir 'dumpersg.conf'
if (-not (Test-Path -LiteralPath $taskNginxExe -PathType Leaf) -or -not (Test-Path -LiteralPath $taskSitesDir -PathType Container)) { throw 'Expected nginx.exe and conf/sites-enabled in NginxDir.' }
$taskMarker = '# Included by C:/nginx/conf/sites-enabled/*.conf inside the http block.'
$taskPrevious = if (Test-Path -LiteralPath $taskTarget) { [IO.File]::ReadAllBytes($taskTarget) } else { $null }
if ($null -ne $taskPrevious -and -not [Text.Encoding]::UTF8.GetString($taskPrevious).StartsWith($taskMarker)) { throw 'Existing dumpersg.conf is not owned by this script.' }
if (-not $PSCmdlet.ShouldProcess($taskTarget, $(if ($Remove) {'Remove DumperSG local proxy'} else {'Install DumperSG local proxy and validate Nginx'}))) { return }
if ($Remove -and $null -eq $taskPrevious) { Write-Output 'DumperSG proxy is not installed.'; return }
$taskPrefix = $taskNginxRoot.Replace('\','/') + '/'
Push-Location $taskNginxRoot
try {
    if ($Remove) { Remove-Item -LiteralPath $taskTarget }
    else { Copy-Item -LiteralPath $taskSource -Destination $taskTarget }
    & $taskNginxExe -p $taskPrefix -t
    if ($LASTEXITCODE -ne 0) { throw 'Nginx validation failed; restoring the previous configuration.' }
    if (-not $NoReload) {
        & $taskNginxExe -p $taskPrefix -s reload
        if ($LASTEXITCODE -ne 0) { throw 'Nginx reload failed; restoring the previous configuration.' }
    }
} catch {
    if ($null -ne $taskPrevious) { [IO.File]::WriteAllBytes($taskTarget,$taskPrevious) }
    elseif (Test-Path -LiteralPath $taskTarget) { Remove-Item -LiteralPath $taskTarget }
    throw
} finally { Pop-Location }
Write-Output $(if ($NoReload) {'Configuration validated and installed; Nginx reload is still required.'} elseif ($Remove) {'DumperSG proxy removed.'} else {'DumperSG proxy ready at http://127.0.0.1:8788 (backend must allow this origin).'} )
