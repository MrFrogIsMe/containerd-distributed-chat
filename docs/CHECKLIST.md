# 各階段自驗 Checklist

每條都可以自己跑指令驗證，不需要等其他組。打勾後才算過。

分工：
- **containerd Manager + Heartbeat**：蔡芝帆、Jasmine Chiang、宥霖（死線 6/4）
- **Gateway + Chat Server**：忻黃魚、Harry Tseng（死線 5/30）
- **Demo / README**：彭啟則（死線 6/6）

---

## Phase 1 Checkpoint（Day 2 結束前）

### Gateway + Chat Server 組（死線最早，先跑）

**Chat Server：**
- [ ] `python main.py` 啟動不報錯，port 9001 有在 listen
- [ ] `curl http://localhost:9001/health` 回 `{"status":"ok","server_id":"#1"}`
- [ ] `curl -X POST http://localhost:9001/send -H 'Content-Type: application/json' -d '{"user":"a","message":"hi"}'` 回 `{"ok":true,"server_id":"#1"}`
- [ ] `curl http://localhost:9001/messages` 回傳含剛才那筆訊息的 list
- [ ] `SERVER_ID=2 PORT=9002 python main.py` 起第二個，health 回 `"server_id":"#2"`
- [ ] Dockerfile build 成功：`nerdctl build -t chat-server .`（或 docker build）

**Gateway + Heartbeat：**
- [ ] `go run main.go` 啟動不報錯，port 8080 有在 listen
- [ ] `curl -X POST http://localhost:8080/register -H 'Content-Type: application/json' -d '{"id":"#1","addr":"localhost:9001"}'` 回 `{"ok":true}`
- [ ] `curl http://localhost:8080/status` 回傳 #1 的 state = alive
- [ ] `curl -X POST http://localhost:8080/heartbeat -H 'Content-Type: application/json' -d '{"id":"#1"}'` 回 `{"ok":true}`
- [ ] 停止送 heartbeat，等 11 秒，`curl /status` 顯示 #1 state = dead
- [ ] `curl -X POST http://localhost:8080/notify -H 'Content-Type: application/json' -d '{"id":"#1","state":"frozen"}'`，/status 顯示 frozen
- [ ] round-robin 測試：register #1 #2 #3，送 6 個 /send，三個 server 各收 2 個

### containerd Manager + Heartbeat 組

- [ ] containerd daemon 已跑：`sudo systemctl status containerd` 顯示 active
- [ ] 能連上 socket：`sudo ctr version` 有回傳 containerd 版本
- [ ] `sudo go run main.go` 啟動不報錯，port 7000 有在 listen
- [ ] 用 Go client pull nginx 並跑起來：`curl -X POST http://localhost:7000/container/start -H 'Content-Type: application/json' -d '{"id":"test","image":"docker.io/library/nginx:alpine","port":8888}'` 成功
- [ ] `sudo ctr containers list` 看得到 test container
- [ ] `curl -X POST http://localhost:7000/container/stop -H 'Content-Type: application/json' -d '{"id":"test"}'` 成功，container 消失
- [ ] `curl http://localhost:7000/container/list` 回傳空 list

### Demo / README 組（彭啟則）

- [ ] `final/` 子目錄全部建好（chat-server/ gateway/ containerd-manager/ scripts/）
- [ ] `README.md` 有「環境需求」和「快速啟動」兩節
- [ ] `scripts/demo_status.sh` 可執行：輸出 `curl http://localhost:8080/status` 的格式化結果

---

## Phase 2 Checkpoint（Day 4 結束前）

> Gateway + Chat Server 組的死線是 5/30，這個 phase 是關鍵交付點。

### Gateway + Chat Server 組

- [ ] Chat Server 起來後自動 POST /register（看 Gateway log 有收到 register）
- [ ] 背景 heartbeat 有在打：Gateway log 每 3 秒一筆 heartbeat
- [ ] Dockerfile 確認可給 containerd Manager 用（說好 image tag 名稱）
- [ ] `POST /send` 轉發到真實 Chat Server 並拿到回應
- [ ] kill 一個 Chat Server process，10 秒內 /status 自動標 dead
- [ ] /send 時自動跳過 dead server
- [ ] `POST /notify {"id":"#1","state":"frozen"}` 後，/send 不送流量到 #1

### containerd Manager + Heartbeat 組

- [ ] 用 Go client 把 Chat Server image 跑起來（`POST /container/start`）
- [ ] container 跑起後 Chat Server 自動向 Gateway /register（看 Gateway log）
- [ ] console 有印 `[event] subscribed to /tasks/exit`（確認全域訂閱有啟動）
- [ ] `POST /container/freeze` 呼叫 task.Pause()，且 Gateway /status 顯示 frozen
- [ ] `POST /container/resume` 恢復，Gateway /status 顯示 alive

### Demo / README 組

- [ ] `scripts/start_all.sh` 能一鍵把 Gateway + containerd Manager + 3 個 Chat Server 全部跑起來
- [ ] `scripts/demo_status.sh` 輸出有顏色（alive=綠、dead=紅、frozen=黃）
- [ ] `scripts/demo_normal.sh` 看得到訊息輪流由 #1 / #2 / #3 回應

---

## Phase 3 Checkpoint（Day 5 結束前）

### 全組整合驗證

- [ ] **全流程 auto restart**
  1. 系統正常跑（/status 全 alive）
  2. `POST /container/stop {"id":"#2"}` kill 掉 #2
  3. 不超過 5 秒，containerd Manager log 印出 restart 訊息
  4. 不超過 10 秒，/status 顯示 #2 = alive
  5. 繼續送訊息，#2 有回應

- [ ] **freeze / resume 聯動**
  1. `POST /container/freeze {"id":"#1"}`
  2. 送 10 個訊息，全部由 #2 / #3 回
  3. `POST /container/resume {"id":"#1"}`
  4. 送 10 個訊息，#1 又有出現

- [ ] **實驗數據已記錄**（填進投影片）
  - recovery latency：kill → alive 的時間（秒）
  - detection latency：停 heartbeat → Gateway 標 dead 的時間
  - freeze 期間 #1 收到 0 個請求

### Demo / README 組（彭啟則）

- [ ] `scripts/demo_kill.sh` 完整跑通，console 輸出清楚
- [ ] `scripts/demo_freeze.sh` 完整跑通
- [ ] README 安裝步驟：照著從零走，系統能正常起來
- [ ] 試錄一次，確認畫面和流程正常

---

## Phase 4 Checkpoint（6/6 繳交前）

### 全組

- [ ] GitHub repo 設為 public，瀏覽器打開看得到 code
- [ ] `README.md` 有詳細安裝步驟，含 GitHub URL
- [ ] Demo 影片上傳雲端，連結有效（點開能播）
- [ ] 投影片含 GitHub URL + Demo 影片連結
- [ ] 投影片 export pdf，上傳 Moodle
- [ ] 投影片有「分散式系統模式對應」那頁（Failure Detection / Auto Recovery）
- [ ] 投影片有「實驗數據」那頁，有實際數字

