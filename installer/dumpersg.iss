#ifndef PackageVersion
  #define PackageVersion "1.0.0"
#endif
#ifndef PackageSource
  #error PackageSource must point to the compiled package directory
#endif
#ifndef PackageOutput
  #error PackageOutput must point to the release directory
#endif

[Setup]
AppId={{EC0F6C8E-8CD6-40EA-B7A6-83B43A40F30D}
AppName=DumperSG
AppVersion={#PackageVersion}
AppPublisher=DumperSG
AppComments=Backup e restauracao MySQL em uma aplicacao web local.
DefaultDirName={localappdata}\Programs\DumperSG
DefaultGroupName=DumperSG
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.17763
WizardStyle=modern
OutputDir={#PackageOutput}
OutputBaseFilename=DumperSG-Setup-{#PackageVersion}-windows-x64
SetupIconFile={#PackageSource}\DumperSG.ico
UninstallDisplayIcon={app}\DumperSG.ico
; Setup offers a safe shutdown before copying files; Uninstall keeps its startup check.
AppMutex={code:AppRunningMutex}
CloseApplications=no
RestartApplications=no
Compression=lzma2
SolidCompression=yes
InfoBeforeFile={#PackageSource}\LEIA-ME.txt

[Languages]
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"

[Tasks]
Name: "desktopicon"; Description: "Criar atalho na area de trabalho"; GroupDescription: "Atalhos:"
Name: "autostart"; Description: "Iniciar ao entrar na minha conta do Windows"; GroupDescription: "Inicializacao:"; Flags: unchecked

[Files]
Source: "{#PackageSource}\DumperSG.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageSource}\dumpersg-core.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageSource}\DumperSG.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageSource}\LEIA-ME.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\DumperSG"; Filename: "{app}\DumperSG.exe"; IconFilename: "{app}\DumperSG.ico"
Name: "{group}\Encerrar DumperSG"; Filename: "{app}\DumperSG.exe"; Parameters: "-shutdown"; IconFilename: "{app}\DumperSG.ico"
Name: "{autodesktop}\DumperSG"; Filename: "{app}\DumperSG.exe"; IconFilename: "{app}\DumperSG.ico"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "DumperSG"; ValueData: """{app}\DumperSG.exe"" -background"; Tasks: autostart; Flags: uninsdeletevalue

[Run]
Filename: "{app}\DumperSG.exe"; Description: "Abrir DumperSG"; Flags: nowait postinstall skipifsilent

[Code]
const
  InstalledAppMutex = 'Local\DumperSG.Installed.Running';

function AppRunningMutex(Param: String): String;
begin
  if IsUninstaller then
    Result := InstalledAppMutex
  else
    Result := '';
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Launcher: String;
  ExitCode, Attempt: Integer;
begin
  Result := '';
  if not CheckForMutexes(InstalledAppMutex) then
    Exit;

  Result := 'O DumperSG está em execução. Feche a aplicação pela bandeja ou pelo atalho Encerrar DumperSG e tente novamente.';
  // Silent updates must never close the application without an explicit choice.
  if WizardSilent then
    Exit;

  if SuppressibleMsgBox(
    'O DumperSG está em execução.' + #13#10#13#10 +
    'Deseja fechar a aplicação automaticamente e continuar a instalação?' + #13#10 +
    'Se houver uma operação em andamento, o encerramento será bloqueado.',
    mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDNO) <> IDYES then
    Exit;

  Launcher := ExpandConstant('{app}\DumperSG.exe');
  if not FileExists(Launcher) then
  begin
    Result := 'Não foi encontrado o launcher da instalação em execução. Mantenha a pasta da instalação existente ou encerre o DumperSG manualmente e tente novamente.';
    Exit;
  end;

  Log('Solicitando encerramento seguro do DumperSG instalado.');
  if not Exec(Launcher, '-shutdown -quiet', '', SW_HIDE, ewWaitUntilTerminated, ExitCode) then
  begin
    Result := 'Não foi possível solicitar o encerramento do DumperSG: ' + SysErrorMessage(ExitCode);
    Exit;
  end;
  if ExitCode <> 0 then
  begin
    Result := 'O DumperSG não pôde ser encerrado com segurança. Pode haver uma operação em andamento ou outra instalação usando os dados. Conclua ou cancele a operação e aguarde o término, ou encerre a aplicação manualmente, antes de tentar novamente.';
    Exit;
  end;

  // The API can stop responding before the supervisor releases its executable.
  for Attempt := 1 to 50 do
  begin
    if not CheckForMutexes(InstalledAppMutex) then
    begin
      Result := '';
      Log('DumperSG encerrado; instalação liberada.');
      Exit;
    end;
    Sleep(100);
  end;
  Result := 'O encerramento foi solicitado, mas o DumperSG ainda está saindo. Aguarde e tente novamente.';
end;
