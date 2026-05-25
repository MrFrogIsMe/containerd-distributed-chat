# 各階段自驗 Checklist

每個 checkpoint 都可以自己跑指令驗證，不需要等其他組。
打勾後才算過，否則不要往下一階段走。

---

## Phase 1 Checkpoint（Day 2 結束前）

### Chat Server 組

- [ ] `python main.py` 啟動不報錯，port 9001 有在 listen
- [ ] `curl http://localhost:9001/health` 回傳 `{"status":"ok","server_id":"#1"}`
- [ ] `curl -X POST http://localhost:9001/send -d '{"user":"a","message":"hi"}'` 回傳 `{"ok":true,"server_id":"#1"}`
- [ ] `curl http://localhost:9001/messages` 回傳含剛才那筆訊息的 list
- [ ] 換 `SERVER_ID=2 PORT=9002 python main.py` 起第二個，health 回 `server_id: #2`
- [ ] Dockerfile build 成功：`docker build -t chat-server .`（或用 nerdctl）

### Gateway + Heartbeat 組

- [ ] `go run main.go` 啟動不報錯，port 8080 有在 listen
- [ ] `curl -X POST http://localhost:8080/register -d '{"id":"#1","addr":"localhost:9001"}'` 回 `{"ok":true}`
- [ ] `curl http://localhost:8080/status` 回傳 `#1` 的 state = alive
- [ ] `curl -X POST http://localhost:8080/heartbeat -d '{"id":"#1"}'` 回 `{"ok":true}`
- [ ] 停止送 heartbeat，等 11 秒後 `curl /status`，#1 的 state = dead
- [ ] `curl -X POST http://localhost:8080/notify -d '{"id":"#1","state":"frozen"}'` 回 `{"ok":true}`，/status 顯示 frozen
- [ ] round-robin 測試：register #1 #2 #3，連送 6 個 /send，每個 server 各收 2 個

### containerd Manager 組

- [ ] `sudo go run main.go` 啟動不報錯，port 7000 有在 listen
- [ ] containerd daemon 已跑：`sudo systemctl status containerd` 顯示 active
- [ ] 能連上 socket：`sudo ctr version` 有回傳 containerd 版本
- [ ] 用 Go client pull nginx：`POST /container/start {"id":"test","image":"docker.io/library/nginx:alpine","port":8888}` 成功，`sudo ctr containers list` 看得到
- [ ] `POST /container/stop {"id":"test"}` 成功，container 消失
- [ ] `GET /container/list` 回傳空 list

### Demo / README 組

- [ ] `final/` 目錄結構建好，子目錄都存在
- [ ] `README.md` 有「環境需求」和「快速啟動」兩節，內容正確
- [ ] `scripts/demo_status.sh` 可以跑：`curl http://localhost:8080/status` 有格式化輸出

---

## Phase 2 Checkpoint（Day 4 結束前）

### Chat Server 組

- [ ] Chat Server 起來後，自動 POST /register 到 Gateway（看 Gateway log 有收到）
- [ ] 背景 heartbeat 有在打：Gateway log 每 3 秒出現一筆 heartbeat
- [ ] Dockerfile 可以讓 containerd Manager 用 Go client 拉起來跑

### Gateway + Heartbeat 組

- [ ] 接上真實 Chat Server，`POST /send` 能成功轉發並拿到回應
- [ ] 把其中一個 Chat Server process kill，10 秒內 /status 自動標 dead
- [ ] /send 時跳過 dead server，改打 alive 的
- [ ] `POST /notify {"id":"#1","state":"frozen"}` 後，/send 不送流量到 #1

### containerd Manager 組

- [ ] 用 Go client 把 Chat Server image 跑起來（`POST /container/start`）
- [ ] 跑起來的 container 自動向 Gateway /register（看 Gateway log）
- [ ] 全域 event 訂閱有啟動：console 有印出 `[event] subscribed to /tasks/exit`
- [ ] `POST /container/freeze` 呼叫 task.Pause()，且成功通知 Gateway state=frozen（`GET /status` 顯示 frozen）
- [ ] `POST /container/resume` 恢復，且 Gateway state=alive

### Demo / README 組

- [ ] `scripts/start_all.sh` 能一鍵把 Gateway + containerd Manager + 3 個 Chat Server 全部跑起來
- [ ] `scripts/demo_status.sh` 輸出有顏色區分 alive / dead / frozen
- [ ] `scripts/demo_normal.sh` 能看到訊息輪流由 #1 / #2 / #3 回應

---

## Phase 3 Checkpoint（Day 5 結束前）

### 全組整合驗證

- [ ] **全流程 auto restart**
  1. 系統正常跑（/status 全部 alive）
  2. `POST /container/stop {"id":"#2"}` kill 掉 #2
  3. 不超過 5 秒，containerd Manager log 印出 `[restart] #2 restarting`
  4. 不超過 10 秒，/status 顯示 #2 = alive
  5. 繼續送訊息，#2 有回應

- [ ] **freeze / resume 聯動**
  1. `POST /container/freeze {"id":"#1"}`
  2. 送 10 個訊息，/status 顯示 #1 frozen，訊息全由 #2 / #3 回
  3. `POST /container/resume {"id":"#1"}`
  4. 送 10 個訊息，#1 又有出現在輪換中

- [ ] **實驗數據已記錄**
  - recovery latency（t1 到 t4）數字有了
  - Gateway 標 dead 的偵測時間數字有了
  - freeze 期間 #1 收到 0 個請求已確認

### Demo / README 組

- [ ] `scripts/demo_kill.sh` 完整跑通，console 輸出清楚
- [ ] `scripts/demo_freeze.sh` 完整跑通
- [ ] README 安裝步驟：照著步驟從零開始，系統能正常跑起來
- [ ] 試錄一次影，確認畫面和流程沒問題

---

## Phase 4 Checkpoint（繳交前）

### 全組

- [ ] GitHub repo 設為 public，能用瀏覽器打開看到 code
- [ ] `README.md` 有詳細安裝步驟，GitHub URL 確認可訪問
- [ ] Demo 影片有上傳到雲端，連結有效（點開能播）
- [ ] 投影片含 GitHub URL 和 Demo 影片連結
- [ ] 投影片 export 成 pdf，上傳到 Moodle
- [ ] 投影片有「分散式系統模式對應」那頁，Failure Detection / Auto Recovery 都提到
- [ ] 投影片有「實驗數據」那頁，有數字不是空白
