# 系統架構書 v2

> 期末專題：分散式聊天室 + containerd 容器協調
> 最後更新：2026-05-25

---

## 文件目錄

1. [問題背景與設計動機](#1-問題背景與設計動機)
2. [系統全覽](#2-系統全覽)
3. [API 規格](#3-api-規格)
4. [元件一：Chat Server](#4-元件一chat-server)
5. [元件二：Gateway + Heartbeat](#5-元件二gateway--heartbeat)
6. [元件三：containerd Manager](#6-元件三containerd-manager)
7. [元件四：Demo 腳本](#7-元件四demo-腳本)
8. [元件五：報告 / 簡報](#8-元件五報告--簡報)
9. [實驗設計](#9-實驗設計)
10. [開發時程](#10-開發時程)
11. [目錄結構](#11-目錄結構)

---

## 1. 問題背景與設計動機

### 使用情境

<!-- SECTION:motivation -->
假設你維運一個即時聊天服務，部署在一台機器上用三個 Chat Server 分流負載。
現實中會遇到三個痛點：

```
痛點一：Chat Server 因 bug 或 OOM 崩掉
        → 那個 port 的流量永久中斷，必須人工介入重啟

痛點二：需要對其中一個 server 做滾動更新
        → 直接停掉會讓進行中的請求失敗，需要先把流量繞開

痛點三：凌晨無人值班，server 掛掉沒人知道
        → 需要自動偵測 + 自動恢復，不依賴人工
```

這不是假設性的問題。任何跑多個服務實體的系統（聊天、API server、微服務）
都會面臨相同挑戰。我們用聊天室當作具體的功能性載體，
核心要展示的是**分散式容錯協調的機制**。
<!-- /SECTION:motivation -->

### containerd 原本做了什麼

<!-- SECTION:containerd_gap -->
containerd 能管理單機上 container 的生命週期，但它有三個刻意的設計邊界：

```
1. 不偵測服務健康
   containerd 只知道 container process 有沒有在跑，
   不知道裡面的 HTTP server 有沒有正常回應。

2. Restart 是 opt-in 且是 polling
   containerd 有 restart plugin，但要手動設 label，
   且是每 10 秒輪詢一次，不是即時的。
   （containerd issue #1731，2018 年開，至今無人填）

3. 不做跨元件協調
   container restart 後，Gateway 不會自動知道它活了；
   freeze 之後，Gateway 也不知道要停止送流量給它。
   這個協調邏輯完全不在 containerd 的設計範圍內。
```

這三個邊界正好是我們要補的地方。
<!-- /SECTION:containerd_gap -->

### 我們解決的問題

<!-- SECTION:our_solution -->
> containerd 管得了單機容器的生命週期，但不知道服務層的健康狀態，
> 也不會自己協調恢復。我們在 containerd 之上實作 heartbeat 偵測與
> 自動恢復的協調層，讓聊天服務在 container 故障時對 client 透明地自癒。

**為什麼用 container 而不是直接跑 process？**

| 比較項目       | 直接跑 process        | 用 containerd 管 container     |
|--------------|----------------------|-------------------------------|
| 環境隔離       | 無                   | 每個 container 獨立 rootfs     |
| Freeze 原語   | 自送 SIGSTOP（粗糙）  | task.Pause()（cgroup freezer） |
| Restart 狀態  | 環境可能髒掉          | 每次從乾淨 image 啟動          |
| 生命週期介面   | 各自為政              | 統一 containerd Go client API  |
| Event 系統    | 無                   | 訂閱 TaskExit 感知所有 container 死亡 |

**我們自己寫的部分（不依賴外部 orchestrator）：**

- Heartbeat registry：偵測服務健康，不只是 process 存活
- Event-driven restart：訂閱 containerd TaskExit event，即時觸發
- 跨元件協調：restart / freeze / resume 都同步通知 Gateway
- 實驗量測：記錄 recovery time，用數據證明達成容錯目標
<!-- /SECTION:our_solution -->

---

## 2. 系統全覽

<!-- SECTION:overview -->
### 架構圖

```
 Client (curl / browser)
         │ HTTP
         ▼
 +---------------+
 |   Gateway     |  :8080
 |  round-robin  |  ← 只把訊息送到 alive 的 server
 |  heartbeat    |    frozen/dead 的自動繞開
 +-------+-------+
         │
    ┌────┴────┐
    │         │
    ▼         ▼
+--------+ +--------+ +--------+
| Chat   | | Chat   | | Chat   |  :9001 / :9002 / :9003
| Server | | Server | | Server |
|   #1   | |   #2   | |   #3   |
+--------+ +--------+ +--------+
    │           │           │
    └───────────┴───────────┘
          │ POST /register（啟動時）
          │ POST /heartbeat（每 3 秒）
          ▼
      Gateway /register、/heartbeat

+----------------------+
| containerd Manager   |  :7000
|                      |
| 訂閱 TaskExit event  |  ← event-driven，非 polling
| auto restart         |
| freeze / resume      |
| 通知 Gateway 狀態    |
+----------+-----------+
           │
           ▼
  containerd daemon
  (/run/containerd/containerd.sock)
```

### 節點清單

| 元件               | Port      | 語言   | 說明                              |
|--------------------|-----------|--------|-----------------------------------|
| Gateway            | 8080      | Go     | 入口、分流、heartbeat registry    |
| Chat Server #1     | 9001      | Python | 聊天室服務實體                    |
| Chat Server #2     | 9002      | Python | 聊天室服務實體                    |
| Chat Server #3     | 9003      | Python | 聊天室服務實體（備援）            |
| containerd Manager | 7000      | Go     | 容器生命週期管理與跨元件協調      |

> 同一台機器跑即可，透過 localhost:port 網路溝通，符合「只透過網路溝通」規定。

### 分散式模式對應

| 模式                | 哪個元件實作              | Demo 方式                               |
|---------------------|---------------------------|-----------------------------------------|
| Failure Detection   | Gateway heartbeat registry| kill server → 10s 後自動標 dead         |
| Auto Recovery       | containerd Manager        | TaskExit event → 即時 restart           |
| Load Balancing      | Gateway round-robin       | 訊息輪流由 #1/#2/#3 回應                |
| Freeze / Resume     | containerd Manager        | task.Pause() / Resume() + 通知 Gateway  |
<!-- /SECTION:overview -->

---

## 3. API 規格

<!-- SECTION:api -->
### 3.1 Chat Server 對外 API

| Method | Path        | Body                                  | Response                              | 說明               |
|--------|-------------|---------------------------------------|---------------------------------------|--------------------|
| POST   | /send       | `{"user":"","message":""}`            | `{"ok":true,"server_id":"#1"}`        | 送訊息             |
| GET    | /messages   | -                                     | `[{"user":"","message":"","ts":""}]`  | 查歷史訊息         |
| GET    | /health     | -                                     | `{"status":"ok","server_id":"#1"}`    | 存活確認           |
| POST   | /register   | `{"id":"#1","addr":"localhost:9001"}` | `{"ok":true}`                         | 啟動時向 Gateway 註冊 |
| POST   | /heartbeat  | `{"id":"#1"}`                         | `{"ok":true}`                         | 每 3 秒打一次      |

### 3.2 Gateway 對外 API

| Method | Path        | Body                                  | Response                              | 說明               |
|--------|-------------|---------------------------------------|---------------------------------------|--------------------|
| POST   | /send       | `{"user":"","message":""}`            | 同 Chat Server /send 回傳             | 轉發給 alive server |
| GET    | /messages   | -                                     | 同 Chat Server /messages 回傳         | 轉發給任一 alive server |
| GET    | /status     | -                                     | `{"servers":[{"id":"","addr":"","state":"alive/dead/frozen"}]}` | 查節點狀態 |
| POST   | /register   | `{"id":"","addr":""}`                 | `{"ok":true}`                         | 供 Chat Server 呼叫 |
| POST   | /heartbeat  | `{"id":""}`                           | `{"ok":true}`                         | 供 Chat Server 呼叫 |
| POST   | /notify     | `{"id":"","state":"frozen/alive"}`    | `{"ok":true}`                         | 供 containerd Manager 呼叫，更新節點狀態 |

### 3.3 containerd Manager 對外 API

| Method | Path               | Body                                       | Response              | 說明                        |
|--------|--------------------|--------------------------------------------|-----------------------|-----------------------------|
| POST   | /container/start   | `{"id":"#1","image":"...","port":9001}`    | `{"ok":true}`         | pull image + 起 container   |
| POST   | /container/stop    | `{"id":"#1"}`                              | `{"ok":true}`         | 停 container                |
| POST   | /container/freeze  | `{"id":"#1"}`                              | `{"ok":true}`         | task.Pause() + 通知 Gateway |
| POST   | /container/resume  | `{"id":"#1"}`                              | `{"ok":true}`         | task.Resume() + 通知 Gateway|
| GET    | /container/list    | -                                          | `[{"id":"","state":""}]` | 列出所有 container       |
<!-- /SECTION:api -->

---

## 4. 元件一：Chat Server

<!-- SECTION:chatserver -->
### 職責

最小可用的聊天室服務。是整個系統的功能核心，其他元件都依賴它的 API。

### 實作清單

- [ ] Python Flask HTTP server
- [ ] 啟動時讀環境變數 `SERVER_ID`（如 #1）和 `PORT`（如 9001）
- [ ] `POST /send` — 把訊息存進 in-memory list，回傳 server_id
- [ ] `GET /messages` — 回傳所有訊息，含 server_id
- [ ] `GET /health` — 回傳 `{"status":"ok","server_id":"..."}`
- [ ] 啟動時自動 `POST http://localhost:8080/register`
- [ ] 背景 thread 每 3 秒 `POST http://localhost:8080/heartbeat`
- [ ] 打包成 Dockerfile（給 containerd Manager 用）

### 關鍵注意

- `SERVER_ID` 從環境變數讀，同一份 code 跑 #1/#2/#3
- messages 存 in-memory 即可，不需要 DB
- `/register` 和 `/heartbeat` 是打向 Gateway，不是自己的 endpoint
- Dockerfile 要用 `ENV SERVER_ID` 和 `ENV PORT` 讓 containerd Manager 帶入

### 交付物

| 交付物              | 誰需要                         |
|---------------------|--------------------------------|
| API 全部可用        | Gateway、containerd Manager    |
| Dockerfile          | containerd Manager             |
| image 本地 tag 名稱 | containerd Manager             |
<!-- /SECTION:chatserver -->

---

## 5. 元件二：Gateway + Heartbeat

<!-- SECTION:gateway -->
### 職責

所有 client 的入口，同時維護節點狀態。分流邏輯和容錯偵測都在這裡。

### 實作清單

- [ ] Go HTTP server，監聽 :8080
- [ ] `ServerRegistry`：dict 維護 `{id → {addr, state, last_seen}}`
- [ ] `POST /register` — 加入 registry，state = alive
- [ ] `POST /heartbeat` — 更新 last_seen timestamp
- [ ] 背景 goroutine 每秒掃一次：超過 10 秒沒收到 → state = dead
- [ ] `POST /send` — round-robin 挑一個 alive server 轉發
- [ ] `GET /messages` — 從任一 alive server 拿訊息
- [ ] 轉發失敗（連不到）→ 標 dead，換下一個再試
- [ ] `GET /status` — 列出所有 server 狀態
- [ ] `POST /notify` — 供 containerd Manager 呼叫，外部更新 state（frozen/alive）

### 狀態機

```
  (未知) --register--> alive
                         │
              10s 無 heartbeat
                         │
                         ▼
                        dead <--restart 後重新 register-- alive

  alive --/notify frozen--> frozen
  frozen --/notify alive--> alive
```

### 關鍵注意

- frozen 和 dead 的區別：dead = 真的掛了；frozen = 刻意暫停（之後會 resume）
- round-robin 時跳過 dead 和 frozen 的 server
- 轉發加 2 秒 timeout，避免卡住整個 Gateway
- `/notify` 要驗 id 存在，否則 containerd Manager 送錯 id 會 silent fail
<!-- /SECTION:gateway -->

---

## 6. 元件三：containerd Manager

<!-- SECTION:ctrdmgr -->
### 職責

用 containerd Go client 直接操控 containerd daemon，管理 Chat Server container
的完整生命週期，並在狀態變化時通知 Gateway。這是展示「真的用 containerd」的核心。

### 實作清單

- [ ] 連上 containerd socket（`/run/containerd/containerd.sock`）
- [ ] 啟動時用 `client.Subscribe()` 訂閱全域 TaskExit event（非 per-task wait）
- [ ] `POST /container/start`
  - pull image（若本地已有則跳過）
  - `client.NewContainer` + snapshot
  - `container.NewTask` + `task.Start`
  - 在 event subscriber goroutine 裡等待此 container 的 TaskExit
- [ ] `POST /container/stop` — `task.Kill` → `task.Delete` → `container.Delete`
- [ ] `POST /container/freeze`
  - `task.Pause()`（底層是 cgroup freezer）
  - `POST http://localhost:8080/notify` 通知 Gateway state = frozen
- [ ] `POST /container/resume`
  - `task.Resume()`
  - `POST http://localhost:8080/notify` 通知 Gateway state = alive
- [ ] TaskExit event handler（auto restart）
  - 收到 TaskExit → 等 1 秒 → 重新執行 /container/start 同一個 id
  - restart 完成後 Chat Server 會自己 POST /register 回 Gateway
  - 記錄 exit_time 和 restart_complete_time，計算 recovery latency
- [ ] `GET /container/list` — 列出目前管的 container 和狀態

### containerd Go client 關鍵程式碼

```go
// 連線
client, _ := containerd.New("/run/containerd/containerd.sock",
    containerd.WithDefaultNamespace("default"))

// 全域 event 訂閱（event-driven，非 per-task polling）
evCh, errCh := client.Subscribe(ctx, `topic=="/tasks/exit"`)
go func() {
    for {
        select {
        case e := <-evCh:
            var exit events.TaskExit
            typeurl.UnmarshalTo(e.Event, &exit)
            go handleExit(exit.ContainerID, exit.ExitStatus)
        case err := <-errCh:
            log.Printf("event error: %v", err)
        }
    }
}()

// pull image
image, _ := client.Pull(ctx, "docker.io/library/python:3.11-slim",
    containerd.WithPullUnpack)

// 起 container
container, _ := client.NewContainer(ctx, containerID,
    containerd.WithImage(image),
    containerd.WithNewSnapshot(containerID+"-snap", image),
    containerd.WithNewSpec(
        oci.WithImageConfig(image),
        oci.WithEnv([]string{"SERVER_ID=#1", "PORT=9001"}),
    ),
)

// 起 task
task, _ := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
task.Start(ctx)

// freeze / resume（底層是 cgroup freezer）
task.Pause(ctx)
task.Resume(ctx)
```

### 關鍵注意

- 需要 root 或 containerd socket 存取權限才能跑
- Subscribe 要在程式啟動時就開，不是每個 container 各自 wait
- freeze 之後**必須**通知 Gateway，否則流量還是會進來
- handleExit 裡要排除主動 stop 的 container（避免誤觸 restart）
- restart 的 goroutine 要 recover panic，crash 了要能感知
<!-- /SECTION:ctrdmgr -->

---

## 7. 元件四：Demo 腳本

<!-- SECTION:demo -->
### 實作清單

- [ ] `scripts/start_all.sh` — 啟動 containerd Manager、Gateway、3 個 Chat Server container
- [ ] `scripts/demo_normal.sh` — 持續送訊息，印出哪個 server 回應（輪流出現 #1/#2/#3）
- [ ] `scripts/demo_freeze.sh` — freeze #1 → 送幾條訊息（只有 #2/#3 回）→ resume #1
- [ ] `scripts/demo_kill.sh` — kill #2 container → 等 Gateway 標 dead → 等 auto restart → 重新 alive
- [ ] `scripts/demo_status.sh` — 持續印 GET /status，視覺化節點狀態

### 建議錄影流程

```
視窗 A: ./scripts/start_all.sh          ← 看 container 一個一個起來
視窗 B: watch -n1 ./scripts/demo_status.sh  ← 持續顯示節點狀態
視窗 C: ./scripts/demo_normal.sh        ← 正常輪流回應

--- 切換 ---
視窗 C: ./scripts/demo_freeze.sh        ← freeze，流量繞開
視窗 C: ./scripts/demo_kill.sh          ← kill，等 auto restart
```

### 關鍵注意

- demo_kill.sh 要用 containerd Manager 的 /container/stop，不要直接 kill process
- 用顏色區分不同 server 的輸出（alive=綠、dead=紅、frozen=黃）
- 記錄每個步驟的時間戳，提供給實驗數據用
<!-- /SECTION:demo -->

---

## 8. 元件五：報告 / 簡報

<!-- SECTION:report -->
### 投影片架構

1. 問題背景：分散式服務維運的三個痛點
2. containerd 的設計邊界（它做了什麼、沒做什麼）
3. 系統架構與元件說明
4. containerd 應用：對應期中報告的技術
   - TaskExit event system（shim → daemon → subscriber）
   - task.Pause() 底層：cgroup freezer
   - containerd Go client API 使用方式
5. 分散式模式對應：Failure Detection / Auto Recovery
6. Demo 截圖 + 實驗數據
7. 結論

### 實驗數據（需填）

| 實驗               | 量測方式                              | 預期結果        |
|--------------------|---------------------------------------|-----------------|
| Recovery latency   | kill → alive 的時間差（毫秒）         | < 5 秒          |
| Detection latency  | 停止 heartbeat → Gateway 標 dead      | 約 10 秒        |
| Freeze 流量隔離    | freeze 期間送到 frozen server 的請求數 | 0               |
| Round-robin 均等   | 100 個請求，各 server 各拿幾個        | 約 33/33/33     |

### 關鍵注意

- 投影片要含 GitHub URL 和 Demo 影片連結，否則不計分
- 繳交格式：pdf 上傳 Moodle
<!-- /SECTION:report -->

---

## 9. 實驗設計

<!-- SECTION:experiment -->
### 實驗一：Auto Recovery（主要實驗）

**目的：** 證明 Failure Detection + Auto Recovery 有達成

**步驟：**
1. 系統正常跑，持續送訊息（每 0.5 秒一條）
2. `POST /container/stop {"id":"#2"}` — kill Chat Server #2
3. 記錄 t1：TaskExit event 抵達 containerd Manager 的時間
4. 記錄 t2：containerd Manager 發出 restart 指令的時間
5. 記錄 t3：新 container 啟動，Chat Server #2 重新 POST /register 回 Gateway 的時間
6. 記錄 t4：Gateway /status 顯示 #2 state = alive 的時間

**量測指標：**

```
event latency   = t2 - t1   （containerd event-driven 的即時性）
restart time    = t3 - t2   （containerd 拉起 container 的速度）
recovery time   = t4 - t1   （整體對外恢復的時間）
```

**對照組：** 不用 containerd Manager，手動 restart 需要多少時間（人工介入成本）

---

### 實驗二：Failure Detection 精準度

**目的：** 驗證 heartbeat 機制的偵測時間符合預期

**步驟：**
1. 讓 Chat Server #1 停止發送 heartbeat（但 process 還活著）
2. 記錄 Gateway 標 #1 為 dead 的時間
3. 驗證停止 heartbeat 到標 dead 的時間在 10-11 秒之間

**意義：** 區分「process 存活」和「服務健康」— 這是 containerd 本身做不到的

---

### 實驗三：Freeze 流量隔離

**目的：** 驗證 freeze 期間 Gateway 確實停止送流量給 frozen server

**步驟：**
1. 在 Chat Server #1 加計數器，記錄收到的請求數
2. `POST /container/freeze {"id":"#1"}`
3. 送 50 個請求
4. `POST /container/resume {"id":"#1"}`
5. 查 #1 的計數器，應為 0
<!-- /SECTION:experiment -->

---

## 10. 開發時程

<!-- SECTION:timeline -->
### Phase 1（Day 1-2）：並行開發，靠 mock

| 組別               | 人數 | 做什麼                                                      |
|--------------------|------|-------------------------------------------------------------|
| containerd Manager | 3 人 | 練習 Go client：pull / run nginx，確認 socket 權限與環境    |
| Gateway            | 2 人 | 先用 hardcode server list 測 round-robin，實作 /send /status|
| Chat Server        | 2 人 | 實作所有 endpoint + Dockerfile，最高優先                    |
| Heartbeat          | 3 人（與 Gateway 合作） | 設計 ServerRegistry，實作 /register /heartbeat + 掃描 goroutine |
| Demo / README      | 1 人 | 建目錄結構、寫 README 骨架、準備 demo_status.sh             |

**Day 1 必做：所有人對齊 API 規格（30 分鐘），確認 port / 路由 / JSON 格式。**

### Phase 2（Day 3-4）：Chat Server 交付，開始整合

| 組別               | 做什麼                                                       |
|--------------------|--------------------------------------------------------------|
| containerd Manager | 把 Chat Server image 用 Go client 跑起來，接全域 event 訂閱  |
| Gateway            | 接上真實 Chat Server，測 register + heartbeat + /notify      |
| Chat Server        | 交付可用版本 + Dockerfile，協助其他組測試                    |
| Heartbeat          | 與 Gateway 整合，驗證 dead 判斷正確、/notify 狀態同步        |
| Demo / README      | 寫 start_all.sh，確認一鍵啟動，開始寫 README 安裝步驟        |

### Phase 3（Day 5）：全系統跑通，Demo 整合

- freeze/resume + Gateway /notify 聯動全流程測試
- kill container → TaskExit event → auto restart → re-register → alive
- 跑三個實驗，記錄 recovery latency 數字
- Demo 人員完成所有腳本並試跑錄影流程

### Phase 4（Day 6）：錄影 + 收尾

- 填實驗數據到各自的投影片
- 錄影（按 demo 腳本順序）
- README 最終版確認安裝步驟可跑
- 投影片 → pdf → 上傳 Moodle
<!-- /SECTION:timeline -->

---

## 11. 目錄結構

```
final/
├── README.md
├── docs/
│   ├── final_demo_requirements.md
│   └── PLAN.md
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
