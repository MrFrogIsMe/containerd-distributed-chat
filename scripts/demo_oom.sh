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
    print(f'Parse error: {e}')
    exit(1)
G='\033[0;32m'; R='\033[0;31m'; Y='\033[0;33m'; B='\033[0;34m'; N='\033[0m'
for nid in sorted(data.keys()):
    nd=data[nid]; st=nd.get('status','unknown'); url=nd.get('url','N/A'); s=nd.get('seconds_ago','?')
    c=G if st=='alive' else (Y if st=='frozen' else R)
    print(f'  {B}{nid}{N}  [{c}{st}{N}]  url={url}  heartbeat={s}s ago')
"
}

get_node_status() {
    local node_id=$1
    curl -s http://localhost:8080/status 2>/dev/null | python3 -c "
import json, sys
try:
    d = json.loads(sys.stdin.read())
    print(d.get('$node_id', {}).get('status', 'unknown'))
except:
    print('unknown')
"
}

echo -e "${BLUE}============================================================${NC}"
echo -e "${BLUE}  Demo: OOM Kill — container 記憶體超限自動重啟${NC}"
echo -e "${BLUE}============================================================${NC}"
echo ""
echo -e "${YELLOW}原理：chat-server-2 限制 64MB RAM，故意塞入大量資料觸發 OOM${NC}"
echo -e "${YELLOW}kernel OOM killer 強制殺掉 container，containerd Manager 自動重啟${NC}"
echo ""

echo -e "${BLUE}[Step 1] 目前狀態：${NC}"
show_status
echo ""

echo -e "${RED}[Step 2] 在 chat-server-2 container 內執行記憶體炸彈...${NC}"
echo -e "  指令：python3 -c \"x = 'A' * 200000000\"  (吃 ~200MB，超過 64MB 限制)"
KILL_TIME=$(date +%s)

sudo ctr tasks exec --exec-id oom-bomb chat-server-2 python3 -c "x = 'A' * 200000000" &
OOM_PID=$!
echo -e "  ${RED}OOM 觸發於 $(date '+%H:%M:%S')${NC}"
echo ""

echo -e "${YELLOW}[Step 3] 等待 kernel OOM killer 介入，觀察 container 死亡與重啟...${NC}"
ALIVE_TIME=0
for i in $(seq 1 20); do
    sleep 2
    NODE2=$(get_node_status "#2")
    ELAPSED=$(( $(date +%s) - KILL_TIME ))
    if [ "$NODE2" = "alive" ] && [ $ELAPSED -gt 3 ]; then
        ALIVE_TIME=$(date +%s)
        echo -e "  ${ELAPSED}s: #2 = ${GREEN}${NODE2}${NC}  <- 重啟完成！"
        break
    else
        echo -e "  ${ELAPSED}s: #2 = ${YELLOW}${NODE2}${NC}"
    fi
done
wait $OOM_PID 2>/dev/null
echo ""

echo -e "${BLUE}[Step 4] 最終狀態：${NC}"
show_status
echo ""

echo -e "${BLUE}============================================================${NC}"
echo -e "${BLUE}  Summary${NC}"
echo -e "${BLUE}============================================================${NC}"
if [ $ALIVE_TIME -gt 0 ]; then
    echo -e "  OOM 觸發到重啟完成：${GREEN}$(( ALIVE_TIME - KILL_TIME ))s${NC}"
else
    echo -e "  未在時間內觀測到重啟"
fi
echo ""
