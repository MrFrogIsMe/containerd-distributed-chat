#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Demo: Normal Round-Robin Messaging${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "${BLUE}Sending 9 messages to gateway (round-robin across #1, #2, #3)...${NC}"
echo ""

for i in $(seq 1 9); do
    echo -e "${YELLOW}--- Request $i ---${NC}"
    RESPONSE=$(curl -s -X POST http://localhost:8080/send \
        -H "Content-Type: application/json" \
        -d "{\"user\":\"demo\",\"message\":\"msg $i\"}" 2>/dev/null)

    if [ -z "$RESPONSE" ]; then
        echo -e "${RED}[ERROR] No response from gateway${NC}"
    else
        python3 - <<PYEOF
import json

raw = '''$RESPONSE'''
try:
    data = json.loads(raw)
except:
    print(f"Raw response: {raw}")
    exit()

GREEN  = '\033[0;32m'
BLUE   = '\033[0;34m'
NC     = '\033[0m'

server = data.get('server_id', data.get('server', 'unknown'))
msg    = data.get('message', data.get('status', ''))
print(f"  {GREEN}server_id={server}{NC}  response={msg}")
print(f"  Full: {json.dumps(data)}")
PYEOF
    fi

    sleep 0.3
done

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Done! Servers #1, #2, #3 took turns.${NC}"
echo -e "${GREEN}========================================${NC}"
