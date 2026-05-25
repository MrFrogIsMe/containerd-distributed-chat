# 系統架構書 v1

> 期末專題：分散式聊天室 + containerd 容器協調
> 最後更新：2026-05-25

---

## 文件目錄

1. [系統全覽](#1-系統全覽)
2. [API 規格（所有人共同遵守）](#2-api-規格)
3. [元件一：Chat Server](#3-元件一chat-server)
4. [元件二：Heartbeat / Service Discovery](#4-元件二heartbeat--service-discovery)
5. [元件三：Gateway](#5-元件三gateway)
6. [元件四：containerd Manager](#6-元件四containerd-manager)
7. [元件五：Demo / 測試腳本](#7-元件五demo--測試腳本)
8. [元件六：報告 / 簡報 / README](#8-元件六報告--簡報--readme)
9. [開發時程與平行策略](#9-開發時程與平行策略)
10. [目錄結構](#10-目錄結構)

---

## 1. 系統全覽

### 架構圖

```
 Client (curl / browser)
        |
        | HTTP
        v
 +-------------+
 |   Gateway   |  :8080
 |  round-robin|
 |  dead check |
 +------+------+
        |          (只把訊息送到 alive 的 server)
   +----|----+
   |         |
   v         v
+--------+ +--------+
| Chat   | | Chat   |   各自 :9001 / :9002 / :9003
| Server | | Server |
|   #1   | |   #2   |
+--------+ +--------+
   ^   ^       ^
   |   |       |
   |   +-------+--- POST /register (啟動時)
   |               POST /heartbeat (每 3 秒)
   |
   v
+------------------+
| containerd       |
| Manager  :7000   |
|                  |
| pull image       |
| start container  |
| watch task exit  |
| auto restart     |
| freeze / resume  |
+------------------+
        |
        v
  containerd daemon
  (/run/containerd/containerd.sock)
```

### 節點清單

| 元件              | Port | 語言   | 說明                       |
|-------------------|------|--------|----------------------------|
| Gateway           | 8080 | Go     | 入口、分流、dead 判斷       |
| Chat Server #1    | 9001 | Python | 聊天室實體                  |
| Chat Server #2    | 9002 | Python | 聊天室實體                  |
| Chat Server #3    | 9003 | Python | 聊天室實體（備援）          |
| containerd Manager| 7000 | Go     | 容器生命週期管理            |

> 同一台機器跑即可，透過 localhost:port 網路溝通，符合「只透過網路溝通」規定。

### 分散式模式對應

| 模式                   | 哪個元件實作               | 如何 demo                          |
|------------------------|----------------------------|------------------------------------|
| Failure Detection      | Heartbeat 模組（Gateway 側）| kill chat server → Gateway 標 dead |
| Auto Recovery          | containerd Manager         | task exit → 自動 restart container  |
| Load Balancing         | Gateway round-robin        | 輪流印出 server id                  |
| Freeze / Resume        | containerd Manager         | freeze → Gateway 不送流量 → resume  |

---

## 2. API 規格

> 所有人第一天對齊這份規格，其他人可以先 mock，Chat Server 做好就直接替換。

### 2.1 Chat Server 對外 API（由 Chat Server 人員實作）

| Method | Path        | Body / Params            | Response                            | 說明               |
|--------|-------------|--------------------------|-------------------------------------|--------------------|
| POST   | /send       | `{"user":"","message":""}` | `{"ok":true,"server_id":"#1"}`    | 送訊息             |
| GET    | /messages   | -                        | `[{"user":"","message":"","ts":""}]`| 查歷史訊息         |
| GET    | /health     | -                        | `{"status":"ok","server_id":"#1"}` | 存活確認           |
| POST   | /register   | `{"id":"#1","addr":"localhost:9001"}` | `{"ok":true}`          | 啟動時向 Gateway 註冊 |
| POST   | /heartbeat  | `{"id":"#1"}`            | `{"ok":true}`                       | 每 3 秒打一次      |

### 2.2 Gateway 對外 API（由 Gateway 人員實作）

| Method | Path        | Body                     | Response                            | 說明               |
|--------|-------------|--------------------------|-------------------------------------|--------------------|
| POST   | /send       | `{"user":"","message":""}` | 同 Chat Server /send 回傳          | 轉發給 alive server |
| GET    | /messages   | -                        | 同 Chat Server /messages 回傳      | 轉發給任一 alive server |
| GET    | /status     | -                        | `{"servers":[{"id":"","addr":"","state":"alive/dead/frozen"}]}` | 查節點狀態 |

### 2.3 containerd Manager 對外 API（由 containerd 人員實作）

| Method | Path              | Body                        | Response          | 說明               |
|--------|-------------------|-----------------------------|-------------------|--------------------|
| POST   | /container/start  | `{"id":"#1","image":"...","port":9001}` | `{"ok":true}` | 起新 container |
| POST   | /container/stop   | `{"id":"#1"}`               | `{"ok":true}`     | 停 container       |
| POST   | /container/freeze | `{"id":"#1"}`               | `{"ok":true}`     | freeze task        |
| POST   | /container/resume | `{"id":"#1"}`               | `{"ok":true}`     | resume task        |
| GET    | /container/list   | -                           | `[{"id":"","state":""}]` | 列出所有 container |

---

## 3. 元件一：Chat Server

### 負責人職責

最小可用的聊天室服務。是整個系統的 source of truth，其他人都在等這個人。

### 實作清單

- [ ] Python Flask / FastAPI HTTP server
- [ ] 啟動時讀環境變數 `SERVER_ID`（如 #1）和 `PORT`（如 9001）
- [ ] `POST /send` — 把訊息存進 in-memory list
- [ ] `GET /messages` — 回傳所有訊息，含 server_id（讓 client 知道是誰處理的）
- [ ] `GET /health` — 回傳 `{"status":"ok","server_id":"..."}`
- [ ] 啟動時自動 `POST http://gateway:8080/register`（帶自己的 id 和 addr）
- [ ] 背景 thread 每 3 秒 `POST http://gateway:8080/heartbeat`
- [ ] 打包成 Dockerfile（給 containerd Manager 用）

### 傳給下一個人的東西

| 交付物                   | 誰需要                  |
|--------------------------|-------------------------|
| API 規格確認（port/路由）| Heartbeat、Gateway、containerd |
| Dockerfile               | containerd Manager      |
| image name（本地 tag）   | containerd Manager      |
| /health endpoint 可用    | Gateway、containerd 心跳偵測 |

### 關鍵注意

- `/register` 和 `/heartbeat` 是打向 **Gateway** 的，不是自己的 endpoint
- `SERVER_ID` 要從環境變數讀，這樣同一份 code 可以跑 #1/#2/#3
- messages 存在 in-memory 就好，不需要 DB

---

## 4. 元件二：Heartbeat / Service Discovery

### 負責人職責

實作在 **Gateway 側**。Gateway 收到 /register 和 /heartbeat 後，
這個模組負責維護節點狀態表，並把 dead/frozen 資訊暴露給分流邏輯。

### 實作清單

- [ ] Gateway 內建一個 `ServerRegistry`（dict 或 struct）
- [ ] `POST /register` — 把 `{id, addr}` 加入 registry，state = alive
- [ ] `POST /heartbeat` — 更新該 id 的 last_seen timestamp
- [ ] 背景 goroutine 每秒掃一次：超過 10 秒沒收到 heartbeat → state = dead
- [ ] state 可以被 containerd Manager 從外部設為 frozen
- [ ] `GET /status` — 把整個 registry 以 JSON 回傳

### 狀態機

```
          register
  (未知) ---------> alive
                     |
         10s 無 heartbeat
                     |
                     v
                    dead  <------ containerd Manager restart 後重新 register --> alive
                     
  alive ----freeze----> frozen
  frozen ---resume----> alive
```

### 傳給下一個人的東西

| 交付物                         | 誰需要       |
|--------------------------------|--------------|
| registry 的 alive server 列表  | Gateway 分流 |
| state 設 frozen 的 interface   | containerd Manager |
| GET /status 可用               | Demo 腳本    |

---

## 5. 元件三：Gateway

### 負責人職責

所有 client 的入口。分流訊息到 alive server，不送給 dead/frozen 的。

### 實作清單

- [ ] Go HTTP server，監聽 :8080
- [ ] `POST /send` — round-robin 挑一個 alive server，把請求轉發過去（HTTP proxy）
- [ ] `GET /messages` — 從任一 alive server 拿訊息回來
- [ ] 轉發失敗（連不到）→ 把該 server 標 dead，換下一個再試
- [ ] `GET /status` — 列出所有 server 狀態（給 demo 用）
- [ ] 接收 `/register` 和 `/heartbeat`（呼叫 Heartbeat 模組）

### Round-Robin 邏輯

```
alive_servers = [s for s in registry if s.state == "alive"]
target = alive_servers[counter % len(alive_servers)]
counter++
```

### 傳給下一個人的東西

| 交付物                   | 誰需要            |
|--------------------------|-------------------|
| GET /status 端點可用     | Demo 腳本         |
| POST /send 可轉發        | Demo 腳本         |
| server state 可被外部改  | containerd Manager（要能把 state 改成 frozen） |

### 關鍵注意

- Gateway 本身不存任何 message，它只是代理
- frozen 和 dead 的區別：dead 是真的掛了，frozen 是暫時暫停（之後會 resume）
- 轉發時加 timeout（建議 2 秒），避免卡住

---

## 6. 元件四：containerd Manager

### 負責人職責

用 Go client 直接操控 containerd daemon，管理 Chat Server container 的生命週期。
這是整個專題最能展示「我們真的用 containerd」的地方。

### 實作清單

- [ ] 連上 containerd socket（`/run/containerd/containerd.sock`）
- [ ] `POST /container/start`
  - pull image（如果本地沒有）
  - NewContainer + NewSnapshot
  - NewTask + Start
  - 起一個 goroutine watch task exit event
- [ ] `POST /container/stop` — task.Kill + task.Delete + container.Delete
- [ ] `POST /container/freeze` — task.Pause()（containerd 原生 freeze）
  - 同時通知 Gateway 把該 server 標 frozen
- [ ] `POST /container/resume` — task.Resume()
  - 同時通知 Gateway 把該 server 標 alive
- [ ] Watch task exit（auto restart）
  - task exit event → 等 2 秒 → 重新 /container/start 同一個 id
  - 重啟後 Chat Server 會自己 /register 回 Gateway
- [ ] `GET /container/list` — 列出目前管的 container 和狀態
- [ ] HTTP server 監聽 :7000 暴露上述 API

### containerd Go client 關鍵 code 片段

```go
// 連線
client, err := containerd.New("/run/containerd/containerd.sock")

// pull image
image, err := client.Pull(ctx, "docker.io/library/python:3.11-slim",
    containerd.WithPullUnpack)

// 起 container
container, err := client.NewContainer(ctx, containerID,
    containerd.WithImage(image),
    containerd.WithNewSnapshot(containerID+"-snapshot", image),
    containerd.WithNewSpec(
        oci.WithImageConfig(image),
        oci.WithEnv([]string{"SERVER_ID=#1","PORT=9001"}),
    ),
)

// 起 task
task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
task.Start(ctx)

// freeze / resume
task.Pause(ctx)
task.Resume(ctx)

// watch exit
statusC, err := task.Wait(ctx)
go func() {
    status := <-statusC
    // → trigger restart
}()
```

### 傳給下一個人的東西

| 交付物                          | 誰需要       |
|---------------------------------|--------------|
| POST /container/freeze 可用     | Demo 腳本    |
| POST /container/resume 可用     | Demo 腳本    |
| auto restart 可 demo（kill 後） | Demo 腳本    |
| container list 狀態可查         | Demo 腳本    |

### 關鍵注意

- 需要 root 或有 containerd socket 權限才能跑
- freeze 之後要通知 Gateway，否則 Gateway 不知道不要送流量
- Watch exit 的 goroutine 要記得 recover panic，不然 crash 了沒人知道

---

## 7. 元件五：Demo / 測試腳本

### 負責人職責

讓整個系統「看起來真的有跑」。老師看 demo 比看 code 更有感，這個人非常重要。

### 實作清單

- [ ] `scripts/start_all.sh` — 一鍵啟動 containerd Manager + Gateway + 3 個 Chat Server
- [ ] `scripts/demo_normal.sh` — 正常聊天，印出哪個 server 回應（輪流出現 #1/#2/#3）
- [ ] `scripts/demo_freeze.sh` — freeze #1 → 送幾條訊息（只有 #2/#3 回） → resume #1
- [ ] `scripts/demo_kill.sh` — kill #2 container → Gateway 偵測 dead → 自動 restart → 重新 alive
- [ ] `scripts/demo_status.sh` — 持續印 GET /status，視覺化節點狀態
- [ ] 整理 terminal 輸出（用顏色 / 對齊讓截圖好看）
- [ ] 錄影流程文件（哪個 terminal 開什麼，順序）

### Demo 腳本順序（建議錄影流程）

```
視窗 A: ./scripts/start_all.sh        ← 看 container 一個一個起來
視窗 B: watch -n1 ./scripts/demo_status.sh  ← 持續顯示節點狀態
視窗 C: ./scripts/demo_normal.sh      ← 正常輪流回應
--- 暫停，切換 ---
視窗 C: ./scripts/demo_freeze.sh      ← freeze，流量繞開
視窗 C: ./scripts/demo_kill.sh        ← kill，等 auto restart
```

---

## 8. 元件六：報告 / 簡報 / README

### 負責人職責

把技術包裝成學術語言，讓老師看得懂我們做了什麼有價值的事。

### 實作清單

- [ ] `README.md` 安裝與操作步驟（含環境需求：containerd、Go、Python）
- [ ] 系統架構圖（可用 draw.io / Excalidraw，白底）
- [ ] 流程圖：heartbeat → dead → restart 整個 sequence diagram
- [ ] 投影片架構
  - 問題背景（分散式系統為什麼需要容錯）
  - 系統架構與元件
  - containerd 如何應用（對應期中報告）
  - 分散式模式對應（Failure Detection / Auto Recovery）
  - Demo 截圖 + 實驗數據
  - 結論
- [ ] 實驗數據表格（由 Demo 人員提供數字）：
  - kill 到 Gateway 標 dead 的秒數
  - kill 到 container restart 完成的秒數
  - freeze 期間訊息遺失數

### 關鍵注意

- 投影片要含 GitHub URL 和 Demo 影片連結，否則不計分
- 繳交格式：pdf 上傳 Moodle

---

## 9. 開發時程與平行策略

### Phase 1（Day 1-2）：並行開發，靠 mock

| 誰             | 做什麼                                              |
|----------------|-----------------------------------------------------|
| Chat Server    | 實作所有 endpoint，這是最高優先                      |
| Heartbeat/Gateway | 先用 hardcode server list 測 round-robin 邏輯   |
| containerd     | 練習用 Go client pull / run nginx，確認環境可用     |
| 報告           | 畫架構圖、寫 README 骨架、投影片大綱               |

**Day 1 必做：所有人一起對齊 API 規格（30 分鐘），確認 port/路由/格式。**

### Phase 2（Day 3-4）：Chat Server 交付，開始整合

| 誰             | 做什麼                                              |
|----------------|-----------------------------------------------------|
| Chat Server    | 交付可用版本 + Dockerfile                           |
| Gateway        | 接上真實 Chat Server，測 register + heartbeat       |
| containerd     | 把 Chat Server image 用 Go client 跑起來            |
| Demo           | 開始寫 start_all.sh，確認環境能一鍵起              |

### Phase 3（Day 5）：全系統跑通，Demo 整合

- containerd freeze/resume + Gateway state 聯動測試
- kill container → auto restart → re-register → alive 全流程測試
- Demo 人員跑完整腳本，記錄數據

### Phase 4（Day 6）：錄影 + 報告

- 報告人員填實驗數據
- 錄影
- 投影片最終版 → pdf → 上傳 Moodle

---

## 10. 目錄結構

```
final/
├── README.md
├── docs/
│   ├── final_demo_requirements.md   期末規定
│   └── PLAN.md                      本文件
├── chat-server/
│   ├── main.py
│   ├── requirements.txt
│   └── Dockerfile
├── gateway/
│   └── main.go
├── containerd-manager/
│   ├── main.go
│   └── go.mod
└── scripts/
    ├── start_all.sh
    ├── demo_normal.sh
    ├── demo_freeze.sh
    ├── demo_kill.sh
    └── demo_status.sh
```
