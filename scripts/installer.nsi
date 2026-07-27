; livesync-sync Windows インストーラ用 NSIS スクリプト
; 使い方:
;   1. makensis をインストール (https://nsis.sourceforge.io)
;   2. build/livesync-nocgo.exe を livesync.exe にリネーム
;   3. makensis installer.nsi
;
; NSIS インストーラ形式にすることで、
; 裸の .exe より Defender に検知されにくくなることがあります。

!define PRODUCT_NAME "livesync-sync"
!define PRODUCT_VERSION "0.1.0"
!define PRODUCT_PUBLISHER "livesync"
!define PRODUCT_WEB_SITE "https://github.com/user/livesync-sync"

Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile "livesync-sync-setup.exe"
InstallDir "$LOCALAPPDATA\livesync-sync"
RequestExecutionLevel user

Section "Install"
  SetOutPath "$INSTDIR"
  File "livesync.exe"
  
  ; Create shortcut in Start Menu
  CreateDirectory "$SMPROGRAMS\livesync-sync"
  CreateShortCut "$SMPROGRAMS\livesync-sync\livesync-sync.lnk" "$INSTDIR\livesync.exe"
  CreateShortCut "$SMPROGRAMS\livesync-sync\Settings UI.lnk" "$INSTDIR\livesync.exe" "--open-ui"
  
  ; Write uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"
  
  ; Register in Add/Remove Programs
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\livesync-sync" "DisplayName" "livesync-sync"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\livesync-sync" "UninstallString" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\livesync.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\livesync-sync\livesync-sync.lnk"
  Delete "$SMPROGRAMS\livesync-sync\Settings UI.lnk"
  RMDir "$SMPROGRAMS\livesync-sync"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\livesync-sync"
SectionEnd
