param([Parameter(Mandatory=$true)][string]$ConfigPath)
$ErrorActionPreference = 'Stop'
try {
    $taskConfig = Get-Content -LiteralPath $ConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    . (Join-Path $PSScriptRoot 'startup-config.ps1')
    $taskArgs = @(Get-DumperStartupArguments -Config $taskConfig)
    if (-not (Test-Path -LiteralPath $taskConfig.executable -PathType Leaf)) { throw 'Build dist/dumpersg.exe first.' }
    New-Item -ItemType Directory -Force -Path $taskConfig.dataDir | Out-Null
    $taskLogDir = Join-Path $taskConfig.dataDir 'startup'
    New-Item -ItemType Directory -Force -Path $taskLogDir | Out-Null
    do {
        # Do not hide a failed startup behind an unbounded log file.
        foreach ($taskLogName in @('stdout.log', 'stderr.log')) {
            $taskLog = Join-Path $taskLogDir $taskLogName
            if (Test-Path -LiteralPath $taskLog) {
                Move-Item -LiteralPath $taskLog -Destination ($taskLog + '.previous') -Force
            }
        }
        # Start-Process joins arguments on Windows: quote every value explicitly.
        $taskQuotedArgs = foreach ($taskArg in $taskArgs) {
            if ($taskArg -match '["\r\n]') { throw 'Invalid startup argument.' }
            '"' + ($taskArg -replace '(\\+)$', '$1$1') + '"'
        }
        $taskChild = Start-Process -FilePath $taskConfig.executable -ArgumentList $taskQuotedArgs -WorkingDirectory (Split-Path -Parent $taskConfig.executable) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskLogDir 'stdout.log') -RedirectStandardError (Join-Path $taskLogDir 'stderr.log')
        $null = $taskChild.Handle
        # Record only this launcher's child, so forced restart never kills an
        # unrelated application or a reused PID. No database credentials are stored.
        $taskState = [ordered]@{ pid=$taskChild.Id; startedTicks=$taskChild.StartTime.ToUniversalTime().Ticks.ToString(); executable=[IO.Path]::GetFullPath($taskConfig.executable); configPath=[IO.Path]::GetFullPath($ConfigPath); dataDir=[IO.Path]::GetFullPath($taskConfig.dataDir); wrapperPid=$PID }
        try {
            $taskStateTemp = Join-Path $taskLogDir 'process.json.tmp'
            [IO.File]::WriteAllText($taskStateTemp, ($taskState | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
            Move-Item -LiteralPath $taskStateTemp -Destination (Join-Path $taskLogDir 'process.json') -Force
        } catch {
            if (-not $taskChild.HasExited) { $taskChild.Kill(); $taskChild.WaitForExit() }
            $taskChild.Dispose()
            throw
        }
        # Keep the task running for the full lifetime of the application.
        $taskChild.WaitForExit()
        $taskExitCode = $taskChild.ExitCode
        $taskChild.Dispose()
    } while ($taskExitCode -eq 75)
    exit $taskExitCode
} catch {
    Write-Error $_ -ErrorAction Continue
    exit 1
}
