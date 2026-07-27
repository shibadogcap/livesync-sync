# Windows Defender 誤検知とその対策

## なぜ起こるのか

`livesync-sync` は Go で書かれたシングルバイナリで、以下の特徴から Defender に誤検知されます:

1. **Go のランタイム特性** — ネットワーク通信 + ファイルアクセス + 暗号化処理を含むバイナリは「未知の脅威」と判定されやすい
2. **CGO/MinGW** — CGO でビルドすると MinGW のランタイムが含まれ、さらに誤検知率が上昇
3. **署名なし** — コードサイニング証明書がないため、Microsoft のクラウド評価で「信頼できない」と判定
4. **新規性** — 配布開始直後はシグネチャデータベースに存在しない

## 対策（効果順）

### ❶ Microsoft WDSI に誤検知報告（効果最大・必須）

https://www.microsoft.com/en-us/wdsi/filesubmission

1. ファイルをアップロード
2. "I believe this file is incorrectly detected as malware" を選択
3. カテゴリで "Potentially unwanted software" または "False positive" を選択
4. コメントに "Open source file synchronization tool written in Go" と記入

**効果**: 1〜2週間で Microsoft のクラウドシグネチャが更新され、全世界の Defender で誤検知が解消されます。

### ❷ 除外設定（即効性）

```powershell
# PowerShell (管理者) で実行
Add-MpPreference -ExclusionPath "C:\path\to\livesync-folder"
```

**注意**: クラウドブロック（Block at First Seen）が有効だと除外を無視する場合があります。その場合は以下も実行:

```powershell
Set-MpPreference -DisableBlockAtFirstSeen $true
```

### ❸ base64 デコード方式（SmartScreen 回避）

ブラウザの Mark of the Web によるブロックを回避:

```powershell
certutil -decode livesync.b64 livesync.exe
```

または:
```powershell
powershell -ExecutionPolicy Bypass -File decode.ps1
```

### ❹ 証明書のインストール

`setup.ps1` を実行すると自己署名証明書を TrustedPublisher ストアにインストールします。
管理者権限が必要ですが、インストール後は Defender の信頼度が上がります。

```powershell
powershell -ExecutionPolicy Bypass -File setup.ps1
```

### ❺ Docker / WSL 運用（完全回避）

Windows 上でも WSL の中で Linux 版を実行すれば Defender の影響を受けません。

```bash
# WSL 内で
./livesync-server --daemon
# Windows ブラウザ → http://localhost:2324/settings
```

## ファイル構成

```
livesync.exe          ← CGO=OFF + 自己署名済み (推奨)
livesync-signed.b64   ← 上記の base64 (SmartScreen回避)
livesync.crt.b64      ← 証明書の base64
setup.ps1             ← セットアップスクリプト
decode.ps1            ← base64 デコードのみ
```

## MSI / インストーラについて

MSI/NSIS インストーラ形式でも、**中身の EXE が同じバイナリであれば Defender の誤検知自体は解決しません**。インストーラ自体がブロックされる可能性があります。

根本的には **コードサイニング証明書の取得** ($200〜300/年) または **Microsoft WDSI への報告** が唯一の恒久対策です。
