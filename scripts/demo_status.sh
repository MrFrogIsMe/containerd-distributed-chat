#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Starting status monitor... Press Ctrl+C to stop${NC}"
sleep 1

while true; do
    clear
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  Distributed Chat System - Status Monitor${NC}"
    echo -e "${BLUE}  $(date '+%Y-%m-%d %H:%M:%S')${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    RAW=$(curl -s http://localhost:8080/status 2>/dev/null)

    if [ -z "$RAW" ]; then
        echo -e "${RED}[ERROR] Cannot reach gateway at localhost:8080${NC}"
        sleep 2
        continue
    fi

    python3 - <<PYEOF
import json, sys

raw = '''$RAW'''
try:
    data = json.loads(raw)
except Exception as e:
    print(f"JSON parse error: {e}")
    print(f"Raw: {raw}")
    sys.exit(1)

GREEN  = '\033[0;32m'
RED    = '\033[0;31m'
YELLOW = '\033[0;33m'
BLUE   = '\033[0;34m'
NC     = '\033[0m'

for node_id in sorted(data.keys()):
    node = data[node_id]
    status  = node.get('status', 'unknown')
    url     = node.get('url', 'N/A')
    secs    = node.get('seconds_ago', '?')

    if status == 'alive':
        color = GREEN
    elif status == 'frozen':
        color = YELLOW
    else:
        color = RED

    print(f"  {BLUE}{node_id}{NC}  [{color}{status}{NC}]  url={url}  heartbeat={secs}s ago")
PYEOF

    echo ""
    echo -e "${BLUE}Refreshing every 2s...${NC}"
    sleep 2
done
