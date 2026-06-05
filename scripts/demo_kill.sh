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
        return 1
    fi
    echo "$RAW" | python3 -c "
import json, sys
raw = sys.stdin.read().strip()
try:
    data = json.loads(raw)
except Exception as e:
    print(f'Parse error: {e}  raw={raw}')
    exit(1)
G='\033[0;32m'; R='\033[0;31m'; Y='\033[0;33m'; B='\033[0;34m'; N='\033[0m'
for nid in sorted(data.keys()):
    nd=data[nid]; st=nd.get('status','unknown'); url=nd.get('url','N/A'); s=nd.get('seconds_ago','?')
    c=G if st=='alive' else (Y if st=='frozen' else R)
    print(f'  {B}{nid}{N}  [{c}{st}{N}]  url={url}  heartbeat={s}s ago')
"
}

get_node2_status() {
    curl -s http://localhost:8080/status 2>/dev/null | python3 -c "
import json, sys
try:
    d = json.loads(sys.stdin.read())
    print(d.get('#2', {}).get('status', 'unknown'))
except:
    print('unknown')
"
}

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Demo: Kill & Auto-Recovery of chat-server-2${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Step 1
echo -e "${BLUE}[Step 1] Current cluster status:${NC}"
show_status
echo ""

# Step 2 & 3
echo -e "${RED}[Step 2] Killing chat-server-2 via containerd...${NC}"
KILL_TIME=$(date +%s)
sudo ctr tasks kill -s SIGKILL chat-server-2
KILL_STATUS=$?
if [ $KILL_STATUS -eq 0 ]; then
    echo -e "  ${GREEN}Kill executed successfully.${NC}"
else
    echo -e "  ${RED}Kill exit code: $KILL_STATUS (may already be dead)${NC}"
fi
echo ""

# Step 4: 等 dead
echo -e "${YELLOW}[Step 3] Polling until #2 goes dead (max 20s)...${NC}"
DEAD_DETECTED=0
DEAD_TIME=0
for i in $(seq 1 10); do
    sleep 2
    NODE2=$(get_node2_status)
    ELAPSED=$(( $(date +%s) - KILL_TIME ))
    echo -e "  ${ELAPSED}s: #2 = ${YELLOW}${NODE2}${NC}"
    if [ "$NODE2" = "dead" ]; then
        DEAD_TIME=$(date +%s)
        DEAD_DETECTED=1
        echo -e "  ${RED}#2 marked dead! Detection latency: $(( DEAD_TIME - KILL_TIME ))s${NC}"
        break
    fi
done
echo ""

# Step 5: 等 alive (auto restart)
echo -e "${YELLOW}[Step 4] Polling until #2 recovers (max 30s)...${NC}"
ALIVE_TIME=0
for i in $(seq 1 15); do
    sleep 2
    NODE2=$(get_node2_status)
    ELAPSED=$(( $(date +%s) - KILL_TIME ))
    if [ "$NODE2" = "alive" ]; then
        ALIVE_TIME=$(date +%s)
        echo -e "  ${ELAPSED}s: #2 = ${GREEN}${NODE2}${NC}  <- recovered!"
        break
    else
        echo -e "  ${ELAPSED}s: #2 = ${RED}${NODE2}${NC}"
    fi
done
echo ""

# Step 6
echo -e "${BLUE}[Step 5] Final cluster status:${NC}"
show_status
echo ""

# Summary
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Recovery Timing Summary${NC}"
echo -e "${BLUE}========================================${NC}"
if [ $DEAD_DETECTED -eq 1 ]; then
    echo -e "  Kill -> Dead detected : ${RED}$(( DEAD_TIME - KILL_TIME ))s${NC}"
else
    echo -e "  Kill -> Dead detected : ${YELLOW}not detected within window${NC}"
fi
if [ $ALIVE_TIME -gt 0 ]; then
    echo -e "  Kill -> Alive recovery: ${GREEN}$(( ALIVE_TIME - KILL_TIME ))s${NC}"
else
    echo -e "  Kill -> Alive recovery: ${YELLOW}not recovered within window${NC}"
fi
echo ""
