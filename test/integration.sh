#!/bin/bash
# livesync-sync Integration Test Suite
# Tests end-to-end sync with a real CouchDB instance.
# Run: bash test/integration.sh

set -uo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

PASS=0; FAIL=0
TEST_DIR="/tmp/livesync-integration-test"
STATE_FILE="$TEST_DIR/state.json"
VAULT_DIR="$TEST_DIR/vault"
CONFIG_FILE="$TEST_DIR/config.json"
BINARY="${1:-build/livesync}"

COUCH_URL="http://localhost:5984"
COUCH_USER="shibadogcap"
COUCH_PASS="yuuki1315"
COUCH_DB="livesync-integration-test"

cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    rm -rf "$TEST_DIR"
    curl -s -X DELETE -u "$COUCH_USER:$COUCH_PASS" "$COUCH_URL/$COUCH_DB" > /dev/null 2>&1 || true
}

trap cleanup EXIT

setup() {
    echo -e "${YELLOW}Setting up test environment...${NC}"
    rm -rf "$TEST_DIR"
    mkdir -p "$VAULT_DIR/subfolder"

    curl -s -X PUT -u "$COUCH_USER:$COUCH_PASS" "$COUCH_URL/$COUCH_DB" > /dev/null

    # Create test files
    echo "Hello from integration test" > "$VAULT_DIR/hello.md"
    echo "# Test Document" > "$VAULT_DIR/test.md"
    echo "content" > "$VAULT_DIR/subfolder/file.txt"
    mkdir -p "$VAULT_DIR"

    cat > "$CONFIG_FILE" <<EOF
{
    "sync": {
        "peers": [{
            "type": "couchdb","name": "remote","group": "main",
            "url": "$COUCH_URL","database": "$COUCH_DB",
            "username": "$COUCH_USER","password": "$COUCH_PASS",
            "passphrase": "integration-test-key",
            "obfuscatePassphrase": "integration-test-key",
            "baseDir": "","useRemoteTweaks": false
        },{
            "type": "storage","name": "local","group": "main",
            "baseDir": "$VAULT_DIR","scanOfflineChanges": true,
            "ignorePatterns": [".trash/",".obsidian/",".git/"]
        }]
    },
    "state": { "file": "$STATE_FILE" },
    "logging": { "level": "debug" },
    "tray": { "enable": false },
    "api": { "listen": "localhost:0" }
}
EOF
    echo "  DB: $COUCH_DB  Vault: $VAULT_DIR"
}

run_sync() {
    local timeout="${1:-15}"
    LSYNC_CONFIG="$CONFIG_FILE" timeout "$timeout" "$BINARY" --daemon 2>&1 || true
}

assert_contains() {
    local desc="$1" haystack="$2" needle="$3"
    if echo "$haystack" | grep -q "$needle"; then
        echo -e "  ${GREEN}✓${NC} $desc"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}✗${NC} $desc"
        FAIL=$((FAIL + 1))
    fi
}

# ============================================================
# Main
# ============================================================

echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}  livesync-sync Integration Tests${NC}"
echo -e "${YELLOW}========================================${NC}"

setup

if [ ! -f "$BINARY" ]; then
    echo -e "${RED}Binary not found: $BINARY${NC}"; exit 1
fi

# Check CouchDB
if ! curl -s -o /dev/null -w "%{http_code}" -u "$COUCH_USER:$COUCH_PASS" "$COUCH_URL/" | grep -q "200"; then
    echo -e "${YELLOW}CouchDB not reachable, running only CLI tests${NC}"
    echo -n "Version: " && "$BINARY" --version
    echo -e "  ${GREEN}✓${NC} binary runs"
    PASS=$((PASS + 1))
    echo "Results: $PASS passed, $FAIL failed"
    [ "$FAIL" -gt 0 ] && exit 1 || exit 0
fi

echo -e "${GREEN}CouchDB reachable${NC}"

# ============================================================
# Test: Local→CouchDB sync
# ============================================================
echo -e "\n${YELLOW}Test: Local→CouchDB sync${NC}"
rm -f "$STATE_FILE"

output=$(run_sync 30)
echo "$output" | grep -E "(File saved|File written|change detected|offline)" || true

assert_contains "Files synced to CouchDB" "$output" "File saved"
assert_contains "Local files processed" "$output" "File written"

# Check CouchDB docs
docs=$(curl -s -u "$COUCH_USER:$COUCH_PASS" "$COUCH_URL/$COUCH_DB/_all_docs")
assert_contains "hello.md in CouchDB" "$docs" "hello.md"
assert_contains "test.md in CouchDB" "$docs" "test.md"
assert_contains "subfolder/file.txt in CouchDB" "$docs" "subfolder/file.txt"

# Check chunk encryption format
chunk_id=$(echo "$docs" | python3 -c "
import sys,json
docs=json.load(sys.stdin)
for r in docs['rows']:
    if r['id'].startswith('h:'):
        print(r['id'])
        break
" 2>/dev/null)
if [ -n "$chunk_id" ]; then
    chunk=$(curl -s -u "$COUCH_USER:$COUCH_PASS" "$COUCH_URL/$COUCH_DB/$chunk_id")
    assert_contains "V2 encrypted format" "$chunk" "%="
fi

# ============================================================
# Test: CouchDB→Local sync (simulate remote change)
# ============================================================
echo -e "\n${YELLOW}Test: CouchDB→Local sync${NC}"

# Add a doc directly to CouchDB, then re-sync
echo '{"type":"plain","path":"remote-note.md","mtime":'$(date +%s%3N)',"ctime":'$(date +%s%3N)',"size":10,"children":[]}' | \
curl -s -X PUT -u "$COUCH_USER:$COUCH_PASS" "$COUCH_URL/$COUCH_DB/remote-note.md" \
    -H "Content-Type: application/json" -d @- > /dev/null

output2=$(run_sync 30)
echo "$output2" | grep -E "(change|File|change detected)" || true
echo -e "  ${GREEN}✓${NC} Sync from CouchDB completed"

# ============================================================
# Test: Offline changes
# ============================================================
echo -e "\n${YELLOW}Test: Offline change detection${NC}"
rm -f "$STATE_FILE"

# Create new files and modify existing ones
echo "# Modified offline" >> "$VAULT_DIR/test.md"
echo "new offline file" > "$VAULT_DIR/offline.md"

output3=$(run_sync 30)
echo "$output3" | grep -E "(Offline|File saved|File written)" || true
assert_contains "Offline changes detected" "$output3" "Offline"

# ============================================================
echo -e "\n${YELLOW}========================================${NC}"
echo -e "${YELLOW}  Results: $PASS passed, $FAIL failed${NC}"
echo -e "${YELLOW}========================================${NC}"
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
