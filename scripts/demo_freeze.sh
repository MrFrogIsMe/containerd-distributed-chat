#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

show_status() {
    RAW=$(curl -s http://localhost:8080/status 2>/dev/null)
    if [ -z "$RAW" ]; then
        echo -e "${RED}[ERROR] Cannot reach gateway${NC}"
        return
    fi
    python3 - <<PYEOF
import json
raw = '''$RAW'''
try:
    data = json.loads(raw)
except Exception as e:
    print(f"Parse error: {e}  raw={raw}")
    exit()
GREEN  = '\033[0;32m'
RED    = '\033[0;31m'
YELLOW = '\033[0;33m'
BLUE   = '\033[0;34m'
NC     = '\033[0m'
for node_id in sorted(data.keys()):
    node = data[node_id]
    status = node.get('status', 'unknown')
    url    = node.get('url', 'N/A')
    secs   = node.get('seconds_ago', '?')
    color  = GREEN if status == 'alive' else (YELLOW if status == 'frozen' else RED)
    print(f"  {BLUE}{node_id}{NC}  [{color}{status}{NC}]  url={url}  heartbeat={secs}s ago")
PYEOF
}

send_messages() {
    local count=$1
    for i in $(seq 1 $count); do
        RESPONSE=$(curl -s -X POST http://localhost:8080/send \
            -H "Content-Type: application/json" \
            -d "{\"user\":\"demo\",\"message\":\"freeze-test msg $i\"}" 2>/dev/null)
        python3 - <<PYEOF
import json
raw = '''$RESPONSE'''
try:
    data = json.loads(raw)
    server = data.get('server_id', data.get('server', 'unknown'))
    GREEN = '\033[0;32m'; NC = '\033[0m'
    print(f"  msg $i  ->  ${GREEN}server={server}${NC}   {json.dumps(data)}")
except:
    print(f"  msg $i  ->  raw: {raw}")
PYEOF
        sleep 0.3
    done
}

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Demo: Freeze / Resume chat-server-2${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Step 1
echo -e "${BLUE}[Step 1] Current cluster status:${NC}"
show_status
echo ""

# Step 2
echo -e "${YELLOW}[Step 2] Freezing chat-server-2...${NC}"
FREEZE_RESP=$(curl -s -X POST "http://localhost:7000/freeze?id=chat-server-2" 2>/dev/null)
echo -e "  Response: ${FREEZE_RESP}"
sleep 1
echo ""

# Step 3
echo -e "${BLUE}[Step 3] Status after freeze (#2 should be frozen):${NC}"
show_status
echo ""

# Step 4
echo -e "${YELLOW}[Step 4] Sending 6 messages (expect only #1 and #3):${NC}"
send_messages 6
echo ""

# Step 5
echo -e "${GREEN}[Step 5] Resuming chat-server-2...${NC}"
RESUME_RESP=$(curl -s -X POST "http://localhost:7000/resume?id=chat-server-2" 2>/dev/null)
echo -e "  Response: ${RESUME_RESP}"
sleep 1
echo ""

# Step 6
echo -e "${BLUE}[Step 6] Status after resume (#2 should be alive):${NC}"
show_status
echo ""

# Step 7
echo -e "${GREEN}[Step 7] Sending 6 more messages (expect #1, #2, #3 all active):${NC}"
send_messages 6
echo ""

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Freeze/Resume demo complete!${NC}"
echo -e "${GREEN}========================================${NC}"
