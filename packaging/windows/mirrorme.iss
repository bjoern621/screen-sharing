; The Windows installer, over the same staged directory the zip is made from
; (scripts/package-windows.ps1).
;
; Everything the app runs is inside that directory: both binaries, ffmpeg, ffplay,
; the GStreamer tools and the plugin set.
; So this recipe copies a tree, writes shortcuts and registers an uninstaller,
; and there is no dependency for it to fetch and no runtime for it to install.
;
; Per user, under PrivilegesRequired=lowest.
; A machine-wide install raises a UAC prompt on top of the SmartScreen warning
; an unsigned binary already gets (docs/install.md).
;
;   iscc /DVersion=0.6.2 /DNumericVersion=0.6.2 /DStage=<staged directory> packaging/windows/mirrorme.iss

#ifndef Version
  #error "Version is undefined: iscc /DVersion=0.6.2"
#endif
; The file-version resource takes digits and dots alone,
; and a run behind no release is stamped 0.0.0.dev.<commit> (.github/workflows/version.yml).
; scripts/installer-windows.ps1 cuts that down to what the resource accepts.
#ifndef NumericVersion
  #error "NumericVersion is undefined: iscc /DNumericVersion=0.6.2"
#endif
#ifndef Stage
  #error "Stage is undefined: iscc /DStage=<staged directory>"
#endif
#ifndef OutputDir
  #define OutputDir "..\..\build\dist"
#endif

[Setup]
; The identity Windows carries the install under.
; Fixed for as long as the app is this app: a changed value installs beside the old one
; rather than over it, and leaves two entries in Apps & features.
AppId={{C38CE7F0-81CD-4D0B-A988-478F818DEAA8}
AppName=MirrorMe
AppVersion={#Version}
VersionInfoVersion={#NumericVersion}
AppPublisher=Björn Blessin
AppPublisherURL=https://github.com/bjoern621/screen-sharing
AppSupportURL=https://github.com/bjoern621/screen-sharing/issues
AppUpdatesURL=https://github.com/bjoern621/screen-sharing/releases
; {autopf} answers %LocalAppData%\Programs under PrivilegesRequired=lowest.
DefaultDirName={autopf}\MirrorMe
DefaultGroupName=MirrorMe
PrivilegesRequired=lowest
; The backend links a 64-bit GStreamer and the shell is published win-x64,
; so a 32-bit install would produce a tree nothing in it can run.
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
LicenseFile={#Stage}\LICENSE
OutputDir={#OutputDir}
OutputBaseFilename=mirrorme-{#Version}-windows-x86_64-setup
SetupIconFile=..\..\avalonia\ScreenShare.App\Assets\mirrorme.ico
UninstallDisplayIcon={app}\mirrorme.exe
UninstallDisplayName=MirrorMe
; LZMA2 over a tree carrying the whole GStreamer plugin set,
; which is most of what this installer weighs.
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
DisableProgramGroupPage=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; The staged directory whole. Which files belong in it is the packaging script's answer,
; and a list repeated here would be a second one, free to disagree.
Source: "{#Stage}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\MirrorMe"; Filename: "{app}\mirrorme.exe"
Name: "{autodesktop}\MirrorMe"; Filename: "{app}\mirrorme.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\mirrorme.exe"; Description: "{cm:LaunchProgram,MirrorMe}"; Flags: nowait postinstall skipifsilent
