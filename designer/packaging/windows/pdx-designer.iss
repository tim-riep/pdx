; Setup-Programm von pdx Designer für Windows (Inno Setup 6).
; Installation pro Benutzer ohne Administratorrechte nach
; %LOCALAPPDATA%\Programs\pdx, Startmenüeintrag für den Benutzer,
; Deinstallation über Einstellungen, Apps.
;
;   ISCC -DVersion=0.3.0 -DTag=v0.3.0 pdx-designer.iss

#ifndef Version
  #define Version "0.0.0"
#endif
#ifndef Tag
  #define Tag "v" + Version
#endif
#ifndef Src
  #define Src "..\..\..\out\package\pdx-designer"
#endif
#ifndef Out
  #define Out "..\..\..\out"
#endif

[Setup]
AppId={{4DC20B2C-1750-47BC-9650-8474CCD91CC5}
AppName=pdx Designer
AppVersion={#Version}
AppVerName=pdx Designer {#Version}
AppPublisher=Tim Riep
AppPublisherURL=https://github.com/tim-riep/pdx
AppSupportURL=https://github.com/tim-riep/pdx/issues
AppUpdatesURL=https://github.com/tim-riep/pdx/releases
AppCopyright=Copyright 2026 Tim Riep
VersionInfoVersion={#Version}
PrivilegesRequired=lowest
DefaultDirName={localappdata}\Programs\pdx
DisableDirPage=yes
DisableProgramGroupPage=yes
UsePreviousAppDir=no
LicenseFile={#Src}\LICENSE
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayName=pdx Designer
UninstallDisplayIcon={app}\pdx-designer.exe
OutputDir={#Out}
OutputBaseFilename=pdx-designer-{#Tag}-windows-amd64-setup
WizardStyle=modern
SetupIconFile=..\icon\pdx-designer.ico
Compression=lzma2/max
SolidCompression=yes
CloseApplications=yes

[Languages]
Name: "de"; MessagesFile: "compiler:Languages\German.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#Src}\pdx-designer.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Src}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Src}\NOTICE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Src}\THIRD_PARTY_NOTICES.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Src}\README.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{userprograms}\pdx Designer"; Filename: "{app}\pdx-designer.exe"; WorkingDir: "{app}"

[Run]
Filename: "{app}\pdx-designer.exe"; Description: "{cm:LaunchProgram,pdx Designer}"; Flags: nowait postinstall skipifsilent

[Code]
// Der Designer braucht die WebView2-Laufzeit (in Windows 11 enthalten).
// Fehlt sie, weist das Setup darauf hin, installiert aber trotzdem.
const
  WebView2Key = 'SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
  WebView2UserKey = 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';

function HasWebView2(): Boolean;
var
  V: String;
begin
  Result := (RegQueryStringValue(HKLM, WebView2Key, 'pv', V) and (V <> '') and (V <> '0.0.0.0'))
    or (RegQueryStringValue(HKCU, WebView2UserKey, 'pv', V) and (V <> '') and (V <> '0.0.0.0'));
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and not HasWebView2() and not WizardSilent() then
    MsgBox('pdx Designer braucht die Microsoft Edge WebView2-Laufzeit. Sie ist in Windows 11 enthalten, unter Windows 10 bitte von https://developer.microsoft.com/microsoft-edge/webview2/ installieren.', mbInformation, MB_OK);
end;
