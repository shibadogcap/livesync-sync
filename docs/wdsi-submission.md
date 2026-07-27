# WDSI Submission Text

Microsoft Defender 誤検知報告 (https://www.microsoft.com/en-us/wdsi/filesubmission) の **Additional information** 欄に貼り付けてください。

```
This is an open-source file synchronization tool written in Go (Golang).

Repository: https://github.com/shibadogcap/livesync-sync
Language: Go 1.25
Build: CGO_ENABLED=0, GOOS=windows, GOARCH=amd64, tags=notray

The binary is a single-file sync daemon that:
1. Watches a local folder for file changes via fsnotify
2. Syncs changes bidirectionally with a CouchDB database
3. Uses AES-256-GCM encryption (PBKDF2+HKDF) compatible with obsidian-livesync
4. Provides a local REST API (localhost:2324) for configuration
5. Exposes an MCP (Model Context Protocol) server at /mcp

No network connections except to the configured CouchDB server.
No persistence, no registry changes, no process injection.
No obfuscation or anti-debugging techniques.

Source code is available at the repository above for full transparency.
Build instructions: CGO_ENABLED=0 go build -tags notray ./cmd/livesync

False positive triggers:
- Go runtime characteristics (network + crypto + file I/O in one static binary)
- Standard library crypto (AES-256-GCM, PBKDF2, HKDF) flagged as suspicious
- Single executable with multiple capabilities appears unusual to heuristics
- No Authenticode signature (open-source project)
```
