// Package hogs knows where a Mac usually loses disk space and explains,
// in Traditional Chinese, what each place is and how to clean it safely.
package hogs

// Risk tells how careful the user must be.
type Risk int

const (
	Safe    Risk = iota // 安全：可重新產生，刪了只是下次變慢
	Careful             // 小心：可能含有唯一的資料，或有正確的清法
	Never               // 不要刪：手動刪會弄壞 App 或資料
)

func (r Risk) String() string {
	switch r {
	case Safe:
		return "安全"
	case Careful:
		return "小心"
	}
	return "不要刪"
}

// Kind selects how a rule is measured.
type Kind int

const (
	KindDir         Kind = iota // sum of fixed paths
	KindNodeModules             // node_modules folders below project roots
	KindBigFiles                // large files inside the paths
	KindSnapshots               // tmutil listlocalsnapshots
)

// Rule describes one known space hog.
//
// Paths starting with "~/" are relative to the home directory; other
// absolute paths are relative to the system root (normally "/").
type Rule struct {
	ID    string
	Title string
	Kind  Kind
	Risk  Risk
	Paths []string
	// TopChildren, if > 0, also reports the largest children (KindDir).
	TopChildren int
	// Exclude lists paths (same syntax as Paths) that are already counted by
	// another rule and are subtracted from this one.
	Exclude   []string
	What      string // 這是什麼
	CanDelete string // 能不能刪
	HowClean  string // 怎麼安全清
}

// BigFileThreshold is the minimum size reported by KindBigFiles rules.
var BigFileThreshold int64 = 100_000_000

// Rules is the built-in catalogue.
var Rules = []Rule{
	{
		ID: "xcode-derived", Title: "Xcode DerivedData", Risk: Safe,
		Paths:     []string{"~/Library/Developer/Xcode/DerivedData"},
		What:      "Xcode 編譯專案時產生的中間檔與索引。",
		CanDelete: "可以。下次開專案會重新建置，第一次會比較慢。",
		HowClean:  "先關掉 Xcode，再把資料夾移到垃圾桶；或在 Xcode 的專案 Locations 設定裡點箭頭進去刪除。",
	},
	{
		ID: "xcode-devicesupport", Title: "Xcode 裝置支援檔 (DeviceSupport)", Risk: Safe,
		Paths: []string{
			"~/Library/Developer/Xcode/iOS DeviceSupport",
			"~/Library/Developer/Xcode/watchOS DeviceSupport",
			"~/Library/Developer/Xcode/tvOS DeviceSupport",
		},
		What:      "接上 iPhone / iPad / Apple Watch 除錯時，從裝置複製下來的系統符號，每個 iOS 版本一份，動輒數 GB。",
		CanDelete: "可以。下次接上該版本的裝置時會重新複製（需要等幾分鐘）。",
		HowClean:  "把不再使用的舊 iOS 版本資料夾移到垃圾桶即可，保留目前手上裝置的版本。",
	},
	{
		ID: "xcode-archives", Title: "Xcode Archives（封存檔）", Risk: Careful,
		Paths:     []string{"~/Library/Developer/Xcode/Archives"},
		What:      "Product > Archive 產生的封存，內含上架用的 App 與 dSYM 符號檔。",
		CanDelete: "小心。已上架的版本如果刪掉 dSYM，之後就無法還原當機報告的符號。",
		HowClean:  "在 Xcode 的 Window > Organizer 裡挑選舊封存刪除；確定不需要再分析當機的版本才刪。",
	},
	{
		ID: "coresimulator", Title: "iOS 模擬器 (CoreSimulator)", Risk: Careful,
		Paths:     []string{"~/Library/Developer/CoreSimulator"},
		What:      "各個模擬器裝置的系統與 App 資料。",
		CanDelete: "小心。不要直接刪這個資料夾，會讓 Xcode 找不到裝置；請用指令清。",
		HowClean:  "執行 xcrun simctl delete unavailable 移除已失效的模擬器；要全部重置用 xcrun simctl erase all（會清掉模擬器內的 App 資料）。",
	},
	{
		ID: "user-caches", Title: "使用者快取 (~/Library/Caches)", Risk: Careful, TopChildren: 5,
		Paths:     []string{"~/Library/Caches"},
		What:      "各個 App 與系統暫存的快取，通常是最大宗的可清理空間；Homebrew、pip、Go 編譯快取也常放在這裡。",
		CanDelete: "多半可以，App 會自己重建，但不要整個資料夾砍掉，有些 App 會因此登出或變慢。",
		HowClean:  "退出 App 後，只把下方列出的、明顯過大的子資料夾移到垃圾桶。",
	},
	{
		ID: "user-logs", Title: "使用者記錄檔 (~/Library/Logs)", Risk: Safe,
		Paths:     []string{"~/Library/Logs"},
		What:      "App 與系統寫下的診斷記錄。",
		CanDelete: "可以，通常很小；除非在追查問題，不需要保留。",
		HowClean:  "把舊的記錄檔移到垃圾桶。",
	},
	{
		ID: "aerial-system", Title: "動態桌布 Aerial 快取（系統層）", Risk: Careful,
		Paths:     []string{"/Library/Application Support/com.apple.idleassetsd/Customer"},
		What:      "macOS 下載的動態桌布／螢幕保護程式影片，由 idleassetsd 管理。如果桌布指向的影片不存在，系統可能一直重複下載，讓「系統資料」暴增。",
		CanDelete: "小心。需要管理員權限才能看到內容（讀不到時大小會顯示為 0），不建議手動刪系統資料夾。",
		HowClean:  "到 系統設定 > 桌布，把動態桌布改成靜態圖片，並在動態桌布的下載清單裡移除不用的；仍持續變大時重新開機再觀察。",
	},
	{
		ID: "aerial-user", Title: "動態桌布 Aerial 快取（使用者層）", Risk: Careful,
		Paths: []string{
			"~/Library/Application Support/com.apple.wallpaper/aerials",
			"~/Library/Application Support/com.apple.idleassetsd",
		},
		What:      "使用者帳號底下的動態桌布下載暫存與縮圖。",
		CanDelete: "小心。內容可以重新下載，但當前使用中的桌布被刪會造成再次下載；持續暴增通常代表桌布設定壞掉。",
		HowClean:  "先在 系統設定 > 桌布 換成靜態桌布，再把 aerials 裡的暫存（Data/tmp）移到垃圾桶。",
	},
	{
		ID: "iphone-backup", Title: "iPhone / iPad 備份", Risk: Careful,
		Paths:     []string{"~/Library/Application Support/MobileSync/Backup"},
		What:      "用 Finder 為 iPhone、iPad 做的本機備份，每台裝置一份，常有數十 GB。",
		CanDelete: "小心。刪了就沒有那份備份；資料夾裡是亂碼名稱，不能憑名字判斷是哪一台。",
		HowClean:  "接上裝置，在 Finder 側邊欄點該裝置 > 一般 > 管理備份，選舊的備份刪除。",
	},
	{
		ID: "docker", Title: "Docker Desktop 映像與容器", Risk: Careful,
		Paths:     []string{"~/Library/Containers/com.docker.docker", "~/.docker"},
		What:      "Docker Desktop 的虛擬磁碟（Docker.raw）包含所有映像、容器與 volume。",
		CanDelete: "小心。不要手動刪虛擬磁碟檔，會連 volume 裡的資料一起消失。",
		HowClean:  "先用 docker system df 看用量，再用 docker system prune（清停用的容器與懸空映像）；也可在 Docker Desktop 的 Settings > Resources 縮小磁碟上限。",
	},
	{
		ID: "homebrew-cache", Title: "Homebrew 下載快取", Risk: Safe,
		Paths:     []string{"~/Library/Caches/Homebrew"},
		What:      "brew 下載過的安裝包與舊版本。",
		CanDelete: "可以，需要時 brew 會重新下載。",
		HowClean:  "執行 brew cleanup --prune=all。",
	},
	{
		ID: "npm-cache", Title: "npm 快取", Risk: Safe,
		Paths:     []string{"~/.npm"},
		What:      "npm 下載過的套件壓縮檔。",
		CanDelete: "可以，下次 npm install 會重新下載。",
		HowClean:  "執行 npm cache clean --force。",
	},
	{
		ID: "pip-cache", Title: "pip 快取", Risk: Safe,
		Paths:     []string{"~/Library/Caches/pip", "~/.cache/pip"},
		What:      "pip 下載與編譯過的套件。",
		CanDelete: "可以。",
		HowClean:  "執行 pip cache purge（或 python3 -m pip cache purge）。",
	},
	{
		ID: "uv-cache", Title: "uv 快取", Risk: Safe,
		Paths:     []string{"~/.cache/uv", "~/Library/Caches/uv"},
		What:      "uv 下載的套件與建好的虛擬環境元件。",
		CanDelete: "可以，會自動重建。",
		HowClean:  "執行 uv cache clean。",
	},
	{
		ID: "go-cache", Title: "Go 模組與編譯快取", Risk: Safe,
		Paths:     []string{"~/go/pkg/mod", "~/Library/Caches/go-build", "~/.cache/go-build"},
		What:      "go 下載的相依套件（pkg/mod）與編譯快取（go-build）。",
		CanDelete: "可以，下次建置會重新下載與編譯。",
		HowClean:  "執行 go clean -modcache 與 go clean -cache。",
	},
	{
		ID: "node-modules", Title: "node_modules 資料夾", Kind: KindNodeModules, Risk: Safe,
		What:      "各個 JavaScript 專案下載的相依套件，一個專案常有幾百 MB。只在專案資料夾（預設為 ~ 底下的 Desktop、Documents、Developer 等常見位置，或用 --projects 指定）內尋找。",
		CanDelete: "可以，只要專案還有 package.json 與 lock 檔，執行 npm install 就能還原。",
		HowClean:  "對很久沒動的專案，把它的 node_modules 移到垃圾桶。",
	},
	{
		ID: "downloads-big", Title: "下載項目中的大檔案 (>= 100 MB)", Kind: KindBigFiles, Risk: Careful,
		Paths:     []string{"~/Downloads"},
		What:      "~/Downloads 裡超過 100 MB 的檔案，常是安裝檔 (.dmg/.pkg)、壓縮檔與影片。",
		CanDelete: "小心。安裝檔通常裝完就可以丟，但其他檔案請先確認內容。",
		HowClean:  "在 Finder 用「依大小排序」檢視，確認後移到垃圾桶。",
	},
	{
		ID: "trash", Title: "垃圾桶", Risk: Careful,
		Paths:     []string{"~/.Trash"},
		What:      "已丟進垃圾桶但還沒清空的項目，這些空間還沒真正釋放。",
		CanDelete: "小心。清空之後無法復原，請先確認裡面沒有要救回的東西。",
		HowClean:  "在 Finder 選單 > 清空垃圾桶。",
	},
	{
		ID: "tm-snapshots", Title: "Time Machine 本機快照", Kind: KindSnapshots, Risk: Careful,
		What:      "Time Machine 在本機磁碟保留的 APFS 快照，會佔用「系統資料」，空間不足時 macOS 會自動釋放。這裡只列出數量，無法得知確切大小。",
		CanDelete: "小心。快照是還原點，刪除後就無法回到那個時間點。",
		HowClean:  "一般不必手動處理；需要時執行 tmutil listlocalsnapshots / 查看，再用 sudo tmutil deletelocalsnapshots <日期> 刪除。",
	},
	{
		ID: "messages", Title: "訊息 App 的附件與資料庫", Risk: Never,
		Paths:     []string{"~/Library/Messages"},
		What:      "iMessage 的對話資料庫與附件（照片、影片）。",
		CanDelete: "不要刪。手動刪除會讓對話資料損毀或遺失，且無法復原。",
		HowClean:  "改在「訊息」App 裡刪除對話，或到 系統設定 > Apple 帳號 > iCloud 管理儲存空間。",
	},
	{
		ID: "icloud-drive", Title: "iCloud Drive 本機副本", Risk: Never,
		Paths:     []string{"~/Library/Mobile Documents"},
		What:      "iCloud Drive 與各 App 同步到本機的檔案。",
		CanDelete: "不要刪。直接刪除會同步刪掉雲端上的檔案。",
		HowClean:  "在 Finder 對檔案按右鍵 > 移除下載項目，只釋放本機空間、保留雲端。",
	},
	{
		ID: "app-containers", Title: "App 沙盒資料 (Containers)", Risk: Never,
		Paths:     []string{"~/Library/Containers", "~/Library/Group Containers"},
		Exclude:   []string{"~/Library/Containers/com.docker.docker"},
		What:      "各個 App 的設定與資料（Docker 除外，另列）。",
		CanDelete: "不要刪。手動刪除會讓 App 遺失資料或無法啟動。",
		HowClean:  "要清某個 App 的資料，請在該 App 內處理，或先確認它有備份再移除該 App 的資料夾。",
	},
}
