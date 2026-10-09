param([string]$Version = '1.0.0', [string]$Compiler = '', [switch]$SkipTests, [switch]$SkipWebBuild)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw 'Version must have the form 1.0.0.' }
if (-not $Compiler) {
    $taskCompilerCommand = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($taskCompilerCommand) { $Compiler = $taskCompilerCommand.Source }
    else { $Compiler = Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe' }
}
if (-not (Test-Path -LiteralPath $Compiler -PathType Leaf)) { throw 'Install Inno Setup 6 or pass -Compiler with the full ISCC.exe path.' }
$taskPackage = Join-Path $taskRoot ('dist\package-' + $Version)
$taskOutput = Join-Path $taskRoot 'dist\releases'
New-Item -ItemType Directory -Path $taskPackage,$taskOutput -Force | Out-Null
Push-Location $taskRoot
try {
    Push-Location (Join-Path $taskRoot 'web')
    try {
        if (-not $SkipWebBuild) {
            npm.cmd ci
            if ($LASTEXITCODE -ne 0) { throw 'Web dependencies failed.' }
            npm.cmd run build
            if ($LASTEXITCODE -ne 0) { throw 'Web build failed.' }
        }
        if (-not $SkipTests) {
            npm.cmd test
            if ($LASTEXITCODE -ne 0) { throw 'Web tests failed.' }
        }
    } finally { Pop-Location }
    if (-not $SkipTests) {
        go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
        go vet -buildvcs=false ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $taskRoot 'web\dist\index.html'))) { throw 'Compile the frontend before packaging.' }
    go build -buildvcs=false -trimpath -o (Join-Path $taskPackage 'dumpersg-core.exe') ./cmd/dumpersg
    if ($LASTEXITCODE -ne 0) { throw 'Core build failed.' }
    go build -buildvcs=false -trimpath -ldflags '-H=windowsgui' -o (Join-Path $taskPackage 'DumperSG.exe') ./cmd/dumpersg-launcher
    if ($LASTEXITCODE -ne 0) { throw 'Launcher build failed.' }
    & (Join-Path $PSScriptRoot 'build-icon.ps1') -OutputPath (Join-Path $taskPackage 'DumperSG.ico')
    Copy-Item -LiteralPath (Join-Path $taskRoot 'installer\LEIA-ME.txt') -Destination (Join-Path $taskPackage 'LEIA-ME.txt') -Force
    & $Compiler ('/DPackageVersion=' + $Version) ('/DPackageSource=' + $taskPackage) ('/DPackageOutput=' + $taskOutput) (Join-Path $taskRoot 'installer\dumpersg.iss')
    if ($LASTEXITCODE -ne 0) { throw 'Installer compilation failed.' }
    $taskInstaller = Join-Path $taskOutput ('DumperSG-Setup-' + $Version + '-windows-x64.exe')
    $taskHash = (Get-FileHash -LiteralPath $taskInstaller -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText(($taskInstaller + '.sha256'), ($taskHash + '  ' + [IO.Path]::GetFileName($taskInstaller) + "`r`n"), [Text.UTF8Encoding]::new($false))
    Write-Output ('Share this installer: ' + $taskInstaller)
} finally { Pop-Location }
