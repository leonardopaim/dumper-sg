param([Parameter(Mandatory=$true)][string]$ConfigPath)
$ErrorActionPreference = 'Stop'
try {
    $taskConfig = Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($taskConfig.version -ne 1) { throw 'Unsupported startup configuration.' }
    if ($taskConfig.address -notmatch '^127\.0\.0\.1:[1-9][0-9]{0,4}$') { throw 'Startup must bind to IPv4 loopback.' }
    if (-not (Test-Path -LiteralPath $taskConfig.executable -PathType Leaf)) { throw 'Build dist/dumpersg.exe first.' }
    New-Item -ItemType Directory -Force -Path $taskConfig.dataDir | Out-Null
    $taskLogDir = Join-Path $taskConfig.dataDir 'startup'
    New-Item -ItemType Directory -Force -Path $taskLogDir | Out-Null
    # Do not hide a failed startup behind an unbounded log file.
    foreach ($taskLogName in @('stdout.log', 'stderr.log')) {
        $taskLog = Join-Path $taskLogDir $taskLogName
        if (Test-Path -LiteralPath $taskLog) {
            Move-Item -LiteralPath $taskLog -Destination ($taskLog + '.previous') -Force
        }
    }
    $taskArgs = @('-addr', $taskConfig.address, '-data-dir', $taskConfig.dataDir,
        '-docker-runtime', 'wsl', '-wsl-distro', $taskConfig.wslDistro,
        '-mysql-image', $taskConfig.mysqlImage, '-docker-image', $taskConfig.dumperImage,
        '-origins', 'http://127.0.0.1:8788,http://localhost:8788')
    # Start-Process joins arguments on Windows: quote every value explicitly.
    $taskQuotedArgs = foreach ($taskArg in $taskArgs) {
        if ($taskArg -match '["\r\n]') { throw 'Invalid startup argument.' }
        '"' + ($taskArg -replace '(\\+)$', '$1$1') + '"'
    }
    $taskChild = Start-Process -FilePath $taskConfig.executable -ArgumentList $taskQuotedArgs -WorkingDirectory (Split-Path -Parent $taskConfig.executable) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskLogDir 'stdout.log') -RedirectStandardError (Join-Path $taskLogDir 'stderr.log')
    $null = $taskChild.Handle
    # Keep the task running for the full lifetime of the application.
    $taskChild.WaitForExit()
    $taskExitCode = $taskChild.ExitCode
    $taskChild.Dispose()
    exit $taskExitCode
} catch {
    Write-Error $_ -ErrorAction Continue
    exit 1
}
