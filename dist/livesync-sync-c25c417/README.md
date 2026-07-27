# livesync-sync — 使い方

## クイックスタート

```bash
# 1. 設定ファイルを作成
cp config.example.json ~/.livesync/config.json
# 編集: CouchDBのURL、認証情報、同期フォルダを設定

# 2. 起動
./livesync --daemon

# 3. ログ確認
tail -f ~/.livesync/sync.log
```

## Docker

```bash
docker build -t livesync-sync .
docker run -d \
  -v ~/.livesync:/etc/livesync \
  -v /path/to/vault:/vault \
  livesync-sync
```

## CLIオプション

| フラグ | 説明 |
|--------|------|
| `--daemon` | デーモンモード（トレイなし） |
| `--config <path>` | 設定ファイルのパス |
| `--reset` | 同期状態をリセット |
| `--version` | バージョン表示 |

## 設定

設定はJSONまたはYAML形式。`~/.livesync/config.json` または `./config.json` に配置。

```json
{
  "sync": {
    "peers": [
      {
        "type": "couchdb",
        "name": "remote",
        "url": "http://localhost:5984",
        "database": "myvault",
        "username": "admin",
        "password": "secret",
        "passphrase": "e2ee-key",
        "baseDir": ""
      },
      {
        "type": "storage",
        "name": "local",
        "baseDir": "./vault",
        "scanOfflineChanges": true
      }
    ]
  }
}
```
