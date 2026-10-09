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
AppMutex=Local\DumperSG.Installed.Running
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
