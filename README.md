# macdust
Find out what is eating your Mac's disk space: an ncdu-style terminal browser that understands macOS.

macdust 是一個終端機工具，用來找出 Mac 的儲存空間被誰吃掉。它像 ncdu 一樣可以逐層瀏覽資料夾大小，但額外「懂 macOS」：內建一份常見吃空間位置的清單（Xcode 快取、iPhone 備份、動態桌布快取、Docker、Homebrew……），用繁體中文告訴你每一項是什麼、能不能刪、怎麼安全清理。

macdust 預設是**唯讀**的。只有明確加上 `--allow-delete` 才能刪除，而且一律是「移到垃圾桶」，不會永久刪除。

## 功能

- 快速並行掃描：worker pool 平行讀目錄；大小用實際占用的磁碟區塊（`st_blocks * 512`）計算，不是檔案標示的大小，所以稀疏檔案不會被高估。
- 硬連結只算一次、不跨越檔案系統（不會掃進外接硬碟或其他 volume）、不跟隨符號連結、權限不足的資料夾會記錄下來但不會中斷掃描。
- 互動式 TUI：長條比例圖、百分比、人類可讀大小、排序（大小／名稱／項目數）、即時搜尋、在 Finder 顯示、視窗縮放自動重繪。支援中文檔名的寬度計算。
- `macdust hogs`：檢查 macOS 常見吃空間的地方，每項標示風險等級（安全／小心／不要刪）與繁中說明。
- 安全的刪除：預設唯讀；`--allow-delete` 才能按 `d`；必須輸入 `yes` 確認；只移到垃圾桶；拒絕家目錄以外的路徑、家目錄本身、`~/Library` 這類系統資料夾。
- 適合腳本使用：`--json` 輸出樹狀結果、`--top N` 列出最大的 N 個檔案、`--min-size` 過濾小項目。
- 只依賴 `golang.org/x/term` 與 `golang.org/x/sys`，TUI 直接輸出 ANSI 跳脫碼，沒有其他 UI 套件。

## 安裝

需要 macOS（主要目標）或 Linux。以下指令請在「終端機」App 裡貼上執行。

### 方法一：go install

先安裝 Go 1.22 以上（沒有的話：`brew install go`），然後：

```sh
go install github.com/useless-husband/macdust@latest
```

執行檔會放在 `~/go/bin/macdust`。如果輸入 `macdust` 顯示 command not found，把下面這行加進 `~/.zshrc`，再重開終端機：

```sh
export PATH="$HOME/go/bin:$PATH"
```

### 方法二：從 Releases 下載

1. 到本專案 GitHub 頁面的 Releases，依你的電腦下載檔案：
   - Apple Silicon（M1/M2/M3/M4）：`macdust-darwin-arm64`
   - Intel Mac：`macdust-darwin-amd64`
   - Linux x86_64：`macdust-linux-amd64`
2. 在終端機執行（以 Apple Silicon 為例，檔案預設在 Downloads）：

```sh
cd ~/Downloads
chmod +x macdust-darwin-arm64
xattr -d com.apple.quarantine macdust-darwin-arm64   # 解除 macOS 對下載檔的隔離標記
sudo mv macdust-darwin-arm64 /usr/local/bin/macdust
```

（看到 "No such xattr" 可以忽略。）

### 方法三：自己編譯

```sh
git clone https://github.com/useless-husband/macdust.git
cd macdust
make build        # 產生 ./macdust
./macdust version
```

## 使用方式

```sh
macdust                 # 瀏覽目前所在的資料夾
macdust ~               # 瀏覽整個家目錄
macdust ~/Library       # 只看 Library
macdust hogs            # 檢查 macOS 常見的吃空間位置（不進 TUI）
```

### TUI 按鍵

| 按鍵 | 功能 |
| --- | --- |
| `↑` `↓` / `k` `j` | 上下移動（`PgUp` `PgDn` `g` `G` 跳頁／頭尾） |
| `Enter` / `→` / `l` | 進入資料夾 |
| `←` / `Backspace` / `h` | 返回上一層 |
| `s` | 切換排序：大小 → 名稱 → 項目數 |
| `/` | 搜尋（只篩選目前這層，輸入即時生效；`Enter` 保留、`Esc` 清除） |
| `o` | 在 Finder 顯示（`open -R`；Linux 用 `xdg-open`） |
| `d` | 移到垃圾桶（只有加 `--allow-delete` 才能用） |
| `q` / `Ctrl+C` | 離開 |

畫面範例（掃描一個小資料夾）：

```
 macdust │ ~/Downloads/example
 合計 204.8 KB · 3 項 · 排序：大小 · 磁碟可用 628.4 GB / 994.6 GB
──────────────────────────────────────────────────────────────────────────────────────────
>  200.7 KB ██████████████████  98.0% victim.bin
     4.1 KB █░░░░░░░░░░░░░░░░░   2.0% keep/

 ↑↓/jk 移動  Enter/→ 進入  ←/⌫ 返回  s 排序  / 搜尋  o Finder  d 刪除  q 離開
```

### 選項

| 選項 | 說明 |
| --- | --- |
| `--allow-delete` | 允許在 TUI 按 `d` 移到垃圾桶（預設唯讀） |
| `--json` | 輸出 JSON，不進 TUI（`--depth N` 控制層數，預設 2） |
| `--top N` | 列出最大的 N 個檔案，不進 TUI |
| `--min-size SIZE` | 只顯示不小於 SIZE 的項目，例如 `100MB`、`1.5G`、`500k` |
| `--cross-fs` | 允許跨越檔案系統（預設不跨越） |
| `--workers N` | 平行讀取目錄的數量 |

大小用十進位單位（1 GB = 1,000,000,000 bytes），和 Finder 與「關於本機」顯示的一致。

### 範例：最大的檔案

```
$ macdust --top 5 ~
掃描 387744 個檔案、121002 個資料夾，共 71.6 GB，耗時 6.21 秒；0 個項目無法讀取
   10.7 GB  ~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw
  679.5 MB  ~/Library/Developer/CoreSimulator/Devices/750D9CEA-.../data/private/var/MobileAsset/.../UC_SIRI_ASR_ASSISTANT_EN_US_EN_US_H18P_Cryptex.dmg
  551.6 MB  ~/Library/Developer/CoreSimulator/Devices/CD63C5A8-.../data/private/var/MobileAsset/.../UC_SIRI_ASR_ASSISTANT_EN_US_EN_US_M11_Cryptex.dmg
  ...
```

（路徑中間以 `...` 節錄。第一行的統計輸出到 stderr，不會混進要處理的結果。）

### 範例：`macdust hogs`

```
$ macdust hogs
[小心]      7.6 GB  iOS 模擬器 (CoreSimulator)
         這是什麼：各個模擬器裝置的系統與 App 資料。
         能不能刪：小心。不要直接刪這個資料夾，會讓 Xcode 找不到裝置；請用指令清。
         怎麼清理：執行 xcrun simctl delete unavailable 移除已失效的模擬器；……
           ~/Library/Developer/CoreSimulator

[安全]      7.1 GB  Xcode 裝置支援檔 (DeviceSupport)
         這是什麼：接上 iPhone / iPad / Apple Watch 除錯時，從裝置複製下來的系統符號……
         能不能刪：可以。下次接上該版本的裝置時會重新複製（需要等幾分鐘）。
         怎麼清理：把不再使用的舊 iOS 版本資料夾移到垃圾桶即可，保留目前手上裝置的版本。
           7.1 GB  ~/Library/Developer/Xcode/iOS DeviceSupport

[不要刪]    6.3 GB  App 沙盒資料 (Containers)
         能不能刪：不要刪。手動刪除會讓 App 遺失資料或無法啟動。
         ...
```

想快速看一眼可以加 `--brief`，每項只印一行：

```
[小心]      7.6 GB  iOS 模擬器 (CoreSimulator)
[安全]      7.1 GB  Xcode 裝置支援檔 (DeviceSupport)
[不要刪]    6.3 GB  App 沙盒資料 (Containers)
[小心]      3.8 GB  使用者快取 (~/Library/Caches)
[安全]      1.2 GB  Xcode DerivedData
[安全]    957.1 MB  uv 快取
...
標示「安全」的項目合計約 10.6 GB（可重新產生，清掉不會遺失資料）。
```

`hogs` 檢查的項目：Xcode DerivedData／DeviceSupport／Archives、iOS 模擬器、`~/Library/Caches`、動態桌布 Aerial 快取（系統層與使用者層）、iPhone/iPad 備份、Docker Desktop、Homebrew／npm／pip／uv／Go 的快取、`node_modules`（預設在 Desktop、Documents、Developer 等常見位置尋找，可用 `--projects 資料夾1,資料夾2` 指定）、`~/Downloads` 裡超過 100 MB 的大檔、垃圾桶、Time Machine 本機快照（只用 `tmutil listlocalsnapshots` 列出數量），以及幾個「不要手動刪」的地方（訊息、iCloud Drive、App 沙盒）。也可用 `--json` 取得機器可讀的結果。

### 效能

在一台 Apple Silicon Mac（內建 SSD，約 72 GB 的家目錄）上實測：

| 指令 | 結果 | 耗時 |
| --- | --- | --- |
| `macdust --top 10 ~` | 387,744 個檔案、121,002 個資料夾、71.6 GB | 約 6.2 秒 |
| `macdust hogs` | 檢查 20 多個位置 | 約 5.5 秒 |
| `du -sk ~`（對照） | 69,873,616 KiB ≈ 71.55 GB | 約 9.1 秒 |

macdust 算出的總量與 `du` 相符。第一次掃描通常比較慢，之後檔案系統快取熱了會更快；實際時間視檔案數量與磁碟而定。

## 刪除功能的安全設計

- 預設完全唯讀。要刪除必須用 `macdust --allow-delete` 啟動。
- 按 `d` 後要輸入 `yes` 再按 Enter，其他輸入（包含 `y`、`YES`）一律取消。
- 刪除 = 移到垃圾桶：macOS 透過 `osascript` 請 Finder 刪除（在 Finder 可以「放回原處」），Finder 無法使用時退回移動到 `~/.Trash`；Linux 依 freedesktop.org XDG 垃圾桶規範。不會呼叫 `rm`。
- 拒絕的目標：家目錄以外的任何路徑（包含透過 `..` 或符號連結繞出去的）、家目錄本身、`~/Desktop`／`~/Documents`／`~/Downloads`／`~/Library` 等標準資料夾本身、`~/Library` 的直接子資料夾（例如 `~/Library/Application Support`）、已經在垃圾桶裡的項目。
- `macdust hogs` 只讀取，不會修改任何東西；它列出的清理方法都需要你自己動手。

## 專案結構

```
main.go                     進入點
internal/
  scan/                     並行掃描、大小樹、排序／篩選／TopFiles、JSON 轉換
  hogs/                     macOS 常見吃空間位置的規則與檢查、報告輸出
  trash/                    刪除保護規則（Guard）與移到垃圾桶
  tui/                      按鍵解析、狀態（Model）、畫面繪製（Render）、終端機主迴圈
  humanize/                 大小格式化與解析
  cli/                      命令列參數與各子指令
scripts/release.sh          交叉編譯（等同 make release）
Makefile                    build / test / release
.github/workflows/ci.yml    CI（macOS 與 Ubuntu）
```

## 測試

```sh
go vet ./...
go test ./...
```

或 `make test`。測試都在 `t.TempDir()` 建立的假目錄裡進行，不會碰到你真實的檔案，內容包含：

- 掃描：實際區塊大小、硬連結只算一次、符號連結不跟隨、稀疏檔案、沒有權限的資料夾、取消掃描。
- `hogs`：用假的家目錄與假的系統根目錄驗證每條規則的比對、排除重複計算、`tmutil` 輸出解析、報告不外洩家目錄路徑。
- 刪除保護：家目錄外、家目錄本身、`..` 與符號連結逃逸、系統資料夾都必須被拒絕，且被拒絕的目標不能被動到。
- TUI：按鍵解析、導覽、排序、搜尋、刪除確認流程、縮放與捲動、以及「render 函式只吃狀態、輸出字串」的畫面檢查（不需要真的終端機）。檔名裡的跳脫字元會被過濾，避免惡意檔名控制你的終端機。

## 原理簡介

- **大小**：對每個項目做 `lstat`，取 `st_blocks * 512`，也就是檔案系統實際配置的空間。這和 `du` 的算法一致，所以稀疏檔案、壓縮檔案不會被「標示大小」誤導。
- **並行**：每個資料夾一個 goroutine，但真正讀目錄（`ReadDir` 加 `lstat`）時要先取得 semaphore 的名額，限制同時進行的磁碟讀取數量；等待子資料夾時已釋放名額，所以不會死結。
- **硬連結**：`nlink > 1` 的檔案用 `(dev, inode)` 記錄，第二次以後遇到的不再累計大小。
- **邊界**：根目錄的裝置編號 (`st_dev`) 與子目錄不同就視為另一個檔案系統，標示為「其他磁碟」而不進入；符號連結只計符號連結本身。
- **TUI**：`Model` 保存狀態並處理按鍵，`Render(model)` 是純函式，輸出整個畫面的字串；主迴圈用 raw mode 讀按鍵、接 `SIGWINCH` 視窗縮放訊號後重繪。

## 已知限制

- 系統受保護的位置（例如 `~/Library/Mail`、部分 `~/Library/Containers`、`MobileSync/Backup`）需要在「系統設定 > 隱私權與安全性 > 完全取用磁碟」允許你的終端機，否則會顯示權限不足、大小可能偏小。
- 整個磁碟的「系統資料」以及 APFS 快照占用的空間，一般使用者權限看不到，macdust 只能列出快照數量。
- 掃描結果全部放在記憶體，數千萬個檔案的磁碟會用掉較多記憶體。
- 掃描完成後不會即時偵測檔案變動；在別處刪除檔案後需重新啟動。

## 授權

MIT，詳見 [LICENSE](LICENSE)。
