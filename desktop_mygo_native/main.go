package main

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3"
	"github.com/egoist/mygo/ui"
	"github.com/guohuiyuan/go-music-dl/core"
	"github.com/guohuiyuan/music-lib/bilibili"
	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qq"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed icon.png
var appIconPNG []byte

const (
	appTitle      = "Music DL"
	appWindowKey  = "music-dl-apple-window"
	defaultWidth  = 1240
	defaultHeight = 800
	minWidth      = 960
	minHeight     = 640
	githubRepoURL = "https://github.com/guohuiyuan/go-music-dl"

	WindowControlsAuto       = "auto"
	WindowControlsMacOS      = "macos"
	WindowControlsWindows    = "windows"
	WindowControlsLinuxGNOME = "linux_gnome"
	WindowControlsLinuxKDE   = "linux_kde"

	ThemeModeDark  = "dark"
	ThemeModeLight = "light"
)

var sourceDisplayNames = map[string]string{
	"all":      "全部",
	"netease":  "网易云",
	"qq":       "QQ 音乐",
	"kugou":    "酷狗",
	"kuwo":     "酷我",
	"migu":     "咪咕",
	"bilibili": "哔哩哔哩",
	"soda":     "汽水",
	"apple":    "Apple Music",
	"local":    "本地音乐",
}

type DownloadItem struct {
	Song      model.Song
	Status    string // "正在下载…", "已下载", "下载失败"
	SavedPath string
	Error     string
	TimeStr   string
}

type CustomPlaylist struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	CreatedAt   string       `json:"created_at"`
	Songs       []model.Song `json:"songs"`
}

type LyricLine struct {
	TimeSec float64
	Text    string
}

type LocalMusicItem struct {
	Filename string
	Path     string
	Size     int64
	Ext      string
	ModTime  string
}

type QRLoginModalState struct {
	Active     bool
	Source     string
	SourceName string
	StatusText string
	ErrorText  string
	Bitmap     *ui.Bitmap
	Success    bool
	Key        string
}

type PlatformAuthStatus struct {
	Checked       bool
	Valid         bool
	Checking      bool
	Username      string
	IsVIP         bool
	PlaylistCount int
	Error         string
	CheckTime     string
}

type NativeApp struct {
	win *mygo.Window

	// 导航标签: "search", "recommend", "downloads", "my_playlists", "local_music", "settings"
	activeTab string

	// 外观主题: "dark", "light"
	themeMode string

	// 搜索状态与类型
	searchKeyword   string
	searchType      string // "song", "playlist", "album"
	selectedSource  string
	sourceOptions   []string
	searching       bool
	loadingMore     bool
	searchPage      int
	searchError     string
	searchResults   []model.Song
	playlistResults []model.Playlist

	// 歌单 / 专辑详情展开
	selectedPlaylist      *model.Playlist
	playlistSongs         []model.Song
	loadingPlaylistDetail bool
	playlistDetailError   string

	// 推荐歌单状态
	recommendPlaylists      []model.Playlist
	loadingRecommend        bool
	recommendError          string
	selectedRecommendSource string

	// 我的歌单状态
	myPlaylists        []model.Playlist
	selectedMySource   string
	loadingMyPlaylists bool
	myPlaylistsError   string

	// 本地自制歌单 (Custom Collections)
	customPlaylists     []CustomPlaylist
	selectedCustomColID string
	newColNameInput     string
	newColDescInput     string
	showCreateColDialog bool

	// 批量选择状态
	batchMode       bool
	selectedSongIDs map[string]bool // key: source + "_" + id -> true

	// 本地音乐库状态
	localMusicFiles   []LocalMusicItem
	loadingLocalMusic bool

	// 音频播放器状态与试听控制
	currentSong   *model.Song
	currentLyric  string
	showLyric     bool
	player        *audio.Player
	playerPlaying bool
	loadingAudio  bool
	audioError    string
	volume        float64
	playProgress  float64 // 0.0 ~ 1.0 比例
	playPosSec    int     // 当前播放位置(秒)
	playDurSec    int     // 曲目总时长(秒)
	playQueue     []model.Song
	playIndex     int
	singleLoop    bool

	// 下载管理与音质选择
	downloadDir     string
	downloadQuality string // "lossless" (无损优先), "exhigh" (320K 极高), "standard" (128K 标准)
	downloads       []DownloadItem

	// 基础与系统设置
	filenameTemplate string
	notifEnabled     bool
	webdavEnabled    bool
	webdavURL        string
	webdavUsername   string
	webdavPassword   string
	webdavDir        string
	cookieNetease    string
	cookieQQ         string
	cookieKugou      string
	cookieBilibili   string

	// Cookie 交互便捷支持与扫码模态
	expandedCookieEdit map[string]bool
	qrLoginPolling     map[string]bool
	qrModal            QRLoginModalState
	authStatus         map[string]PlatformAuthStatus

	// 界面字体配置
	fontFamily      string
	customFontInput string

	// 平台窗口控制风格: "auto", "macos", "windows", "linux_gnome", "linux_kde"
	windowControlsStyle string

	// 并发请求世代与状态
	playGen       uint64
	hasLoadedMore bool

	mu sync.Mutex
}

type DesktopSettings struct {
	ThemeMode           string  `json:"themeMode"`
	WindowControlsStyle string  `json:"windowControlsStyle"`
	FontFamily          string  `json:"fontFamily"`
	DownloadQuality     string  `json:"downloadQuality"`
	Volume              float64 `json:"volume"`
	NotifEnabled        bool    `json:"notifEnabled"`
	DownloadDir         string  `json:"downloadDir"`
	FilenameTemplate    string  `json:"filenameTemplate"`
}

func desktopSettingsPath() string {
	return filepath.Join("data", "desktop_settings.json")
}

func (a *NativeApp) loadDesktopSettings() {
	p := desktopSettingsPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var s DesktopSettings
	if err := json.Unmarshal(data, &s); err == nil {
		if s.ThemeMode != "" {
			a.themeMode = s.ThemeMode
		}
		if s.WindowControlsStyle != "" {
			a.windowControlsStyle = s.WindowControlsStyle
		}
		if s.FontFamily != "" {
			a.fontFamily = s.FontFamily
		}
		if s.DownloadQuality != "" {
			a.downloadQuality = s.DownloadQuality
		}
		if s.Volume > 0 {
			a.volume = s.Volume
		}
		a.notifEnabled = s.NotifEnabled
		if s.DownloadDir != "" {
			a.downloadDir = s.DownloadDir
		}
		if s.FilenameTemplate != "" {
			a.filenameTemplate = s.FilenameTemplate
		}
	}
}

func (a *NativeApp) saveDesktopSettings() {
	s := DesktopSettings{
		ThemeMode:           a.themeMode,
		WindowControlsStyle: a.windowControlsStyle,
		FontFamily:          a.fontFamily,
		DownloadQuality:     a.downloadQuality,
		Volume:              a.volume,
		NotifEnabled:        a.notifEnabled,
		DownloadDir:         a.downloadDir,
		FilenameTemplate:    a.filenameTemplate,
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		log.Printf("[saveDesktopSettings] json marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Printf("[saveDesktopSettings] failed to create data directory: %v", err)
		return
	}
	if err := os.WriteFile(desktopSettingsPath(), data, 0644); err != nil {
		log.Printf("[saveDesktopSettings] failed to write desktop settings: %v", err)
	}
}

func newNativeApp() *NativeApp {
	core.CM.Load()
	settings := core.GetWebSettings()
	downloadDir := settings.DownloadDir
	if downloadDir == "" {
		downloadDir = "data/downloads"
	}
	absDir, err := filepath.Abs(downloadDir)
	if err == nil {
		downloadDir = absDir
	}
	cookies := core.CM.GetAll()

	app := &NativeApp{
		activeTab:               "search",
		themeMode:               ThemeModeDark,
		fontFamily:              "",
		customFontInput:         "",
		expandedCookieEdit:      make(map[string]bool),
		qrLoginPolling:          make(map[string]bool),
		searchKeyword:           "",
		searchType:              "song",
		searchPage:              1,
		selectedSource:          "all",
		sourceOptions:           []string{"all", "netease", "qq", "kugou", "kuwo", "migu", "bilibili", "soda"},
		volume:                  0.8,
		playIndex:               -1,
		downloadQuality:         "lossless",
		downloadDir:             downloadDir,
		filenameTemplate:        core.DefaultDownloadFilenameTemplate,
		notifEnabled:            true,
		windowControlsStyle:     WindowControlsAuto,
		selectedRecommendSource: "netease",
		webdavEnabled:           settings.WebDAVEnabled,
		webdavURL:               settings.WebDAVURL,
		webdavUsername:          settings.WebDAVUsername,
		webdavPassword:          settings.WebDAVPassword,
		webdavDir:               settings.WebDAVDir,
		cookieNetease:           cookies["netease"],
		cookieQQ:                cookies["qq"],
		cookieKugou:             cookies["kugou"],
		cookieBilibili:          cookies["bilibili"],
		selectedSongIDs:         make(map[string]bool),
		authStatus:              make(map[string]PlatformAuthStatus),
		selectedMySource:        "netease",
	}

	app.loadDesktopSettings()
	app.loadCustomPlaylists()
	app.refreshLocalMusic()
	go app.startPlaybackTicker()
	for _, s := range []string{"netease", "qq", "kugou", "bilibili"} {
		if strings.TrimSpace(cookies[s]) != "" {
			app.verifyPlatformAuth(s, false)
		}
	}
	return app
}

func (a *NativeApp) isDark() bool {
	return a.themeMode != ThemeModeLight
}

func (a *NativeApp) effectiveWindowControlsStyle() string {
	if a.windowControlsStyle != "" && a.windowControlsStyle != WindowControlsAuto {
		return a.windowControlsStyle
	}
	switch runtime.GOOS {
	case "darwin":
		return WindowControlsMacOS
	case "windows":
		return WindowControlsWindows
	case "linux":
		desktop := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
		if strings.Contains(desktop, "kde") || strings.Contains(desktop, "plasma") {
			return WindowControlsLinuxKDE
		}
		return WindowControlsLinuxGNOME
	default:
		return WindowControlsWindows
	}
}

type PulseColors struct {
	IsDark          bool
	Background      ui.Color
	SidebarBg       ui.Color
	SidebarBorder   ui.Color
	Surface         ui.Color
	SurfaceHover    ui.Color
	SurfacePressed  ui.Color
	Border          ui.Color
	Text            ui.Color
	TextMuted       ui.Color
	Faint           ui.Color
	Accent          ui.Color
	AccentHover     ui.Color
	AccentPressed   ui.Color
	AccentText      ui.Color
	Danger          ui.Color
	Success         ui.Color
	Warning         ui.Color
	DockBg          ui.Color
	HeaderBorder    ui.Color
	CardMiniBg      ui.Color
	SongRowBg       ui.Color
	SongRowActiveBg ui.Color
	PillBg          ui.Color
	PillBorder      ui.Color
}

func (a *NativeApp) colors() PulseColors {
	if !a.isDark() {
		// Pulse Light Mode Palette (1:1 复刻 overview-light.jpg 浅色设计)
		return PulseColors{
			IsDark:          false,
			Background:      ui.Hex("#f5f5f7"),
			SidebarBg:       ui.Hex("#ffffff"),
			SidebarBorder:   ui.Hex("#e5e5eb"),
			Surface:         ui.Hex("#ffffff"),
			SurfaceHover:    ui.Hex("#f0f0f3"),
			SurfacePressed:  ui.Hex("#e5e5ea"),
			Border:          ui.Hex("#e2e2e7"),
			Text:            ui.Hex("#1d1d1f"),
			TextMuted:       ui.Hex("#6e6e73"),
			Faint:           ui.Hex("#86868b"),
			Accent:          ui.Hex("#007aff"), // Pulse Light 标志性蓝
			AccentHover:     ui.Hex("#0066d6"),
			AccentPressed:   ui.Hex("#0055b3"),
			AccentText:      ui.Hex("#ffffff"),
			Danger:          ui.Hex("#ff2d55"),
			Success:         ui.Hex("#28a745"),
			Warning:         ui.Hex("#d99a00"),
			DockBg:          ui.Hex("#ffffff"),
			HeaderBorder:    ui.Hex("#e5e5eb"),
			CardMiniBg:      ui.Hex("#f2f2f7"),
			SongRowBg:       ui.Hex("#fbfbfd"),
			SongRowActiveBg: ui.Hex("#e5f1ff"),
			PillBg:          ui.Hex("#ebebef"),
			PillBorder:      ui.Hex("#dadadf"),
		}
	}
	// Pulse Dark Mode Palette (1:1 复刻 overview-dark.jpg & processes-dark.jpg 深色设计)
	return PulseColors{
		IsDark:          true,
		Background:      ui.Hex("#0b0b0d"),
		SidebarBg:       ui.Hex("#0e0e12"),
		SidebarBorder:   ui.Hex("#1c1c22"),
		Surface:         ui.Hex("#16161a"),
		SurfaceHover:    ui.Hex("#1e1e24"),
		SurfacePressed:  ui.Hex("#26262e"),
		Border:          ui.Hex("#232328"),
		Text:            ui.Hex("#f5f5f7"),
		TextMuted:       ui.Hex("#a1a1a6"),
		Faint:           ui.Hex("#6e6e73"),
		Accent:          ui.Hex("#0a84ff"), // Pulse Dark 标志性电光蓝
		AccentHover:     ui.Hex("#0077e6"),
		AccentPressed:   ui.Hex("#0062c4"),
		AccentText:      ui.Hex("#ffffff"),
		Danger:          ui.Hex("#ff375f"),
		Success:         ui.Hex("#30d158"),
		Warning:         ui.Hex("#ffc93d"),
		DockBg:          ui.Hex("#0e0e12"),
		HeaderBorder:    ui.Hex("#18181f"),
		CardMiniBg:      ui.Hex("#131317"),
		SongRowBg:       ui.Hex("#121216"),
		SongRowActiveBg: ui.Hex("#142233"),
		PillBg:          ui.Hex("#141418"),
		PillBorder:      ui.Hex("#202026"),
	}
}

func main() {
	var flagVersion bool
	flag.BoolVar(&flagVersion, "v", false, "显示版本信息")
	flag.BoolVar(&flagVersion, "version", false, "显示版本信息")
	flag.Parse()

	if flagVersion {
		fmt.Printf("%s v%s (Apple Fluid Design Native UI)\n", appTitle, core.AppVersion)
		return
	}

	mygo.App.SetName(appTitle)
	mygo.App.SetVersion(core.AppVersion)

	if !mygo.App.RequestSingleInstanceLock() {
		log.Println("检测到另一个 Music DL 实例已在运行中，将唤醒已有窗口。")
		return
	}

	app := newNativeApp()

	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		if app.win != nil {
			app.win.Show()
			app.win.Restore()
			app.win.Focus()
		}
	})

	mygo.App.WhenReady(func() {
		// Frameless: true 无边框窗口，由 Apple Fluid Design 顶栏与原生组件完全接管渲染
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     appTitle,
			Width:     defaultWidth,
			Height:    defaultHeight,
			MinWidth:  minWidth,
			MinHeight: minHeight,
			StateKey:  appWindowKey,
			Frameless: true,
			Content:   ui.View(app.view),
		})

		if len(appIconPNG) > 0 {
			_ = app.win.SetIcon(appIconPNG)
		}

		app.setupAppMenu()

		app.win.OnClosed(func() {
			if app.player != nil {
				app.player.Stop()
				app.player.Close()
			}
			mygo.App.Quit()
		})
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatalf("Music DL Native UI 运行异常: %v", err)
	}
}

func (a *NativeApp) setupAppMenu() {
	items := []*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{
			Label: "文件",
			Submenu: []*mygo.MenuItem{
				{
					Label: "打开下载文件夹",
					Click: func(item *mygo.MenuItem, w *mygo.Window) {
						_ = mygo.Shell.OpenPath(a.downloadDir)
					},
				},
				mygo.Separator(),
				{
					Label:       "退出 " + appTitle,
					Role:        mygo.RoleQuit,
					Accelerator: "CmdOrCtrl+Q",
				},
			},
		},
		{
			Label: "视图",
			Submenu: []*mygo.MenuItem{
				{
					Label:       "全屏切换",
					Role:        mygo.RoleToggleFullScreen,
					Accelerator: "F11",
				},
			},
		},
		{
			Label: "帮助",
			Submenu: []*mygo.MenuItem{
				{
					Label: "项目主页…",
					Click: func(item *mygo.MenuItem, w *mygo.Window) {
						_ = mygo.Shell.OpenExternal(githubRepoURL)
					},
				},
				mygo.Separator(),
				{
					Label: "关于 " + appTitle,
					Click: func(item *mygo.MenuItem, w *mygo.Window) {
						_, _ = mygo.Dialog.Message(mygo.MessageOptions{
							Parent:  w,
							Title:   "关于 " + appTitle,
							Message: fmt.Sprintf("%s v%s\n基于 Apple Fluid Design 规范构建的原生桌面音乐客户端。\n\n项目主页: %s", appTitle, core.AppVersion, githubRepoURL),
							Buttons: []string{"好"},
						})
					},
				},
			},
		},
	}

	mygo.App.SetMenu(mygo.NewMenu(items))
}

// view 应用主视图，精确复刻 Pulse (https://pulse.egoist.dev) GPU 原生设计系统 (支持明暗双模与 WinUI 3 风格)
func (a *NativeApp) view(c *ui.Context) {
	pal := a.colors()

	var theme *ui.Theme
	if pal.IsDark {
		theme = ui.DarkTheme()
	} else {
		theme = ui.LightTheme()
	}
	theme.Background = pal.Background
	theme.Surface = pal.Surface
	theme.SurfaceHover = pal.SurfaceHover
	theme.SurfacePressed = pal.SurfacePressed
	theme.Border = pal.Border
	theme.Text = pal.Text
	theme.TextMuted = pal.TextMuted
	theme.Accent = pal.Accent
	theme.AccentHover = pal.AccentHover
	theme.AccentPressed = pal.AccentPressed
	theme.AccentText = pal.AccentText
	theme.Danger = pal.Danger
	theme.Success = pal.Success
	theme.Warning = pal.Warning
	if a.fontFamily != "" {
		theme.Font = a.fontFamily
	}
	theme.Radius = 10
	c.SetTheme(theme)

	t := c.Theme()

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		// 主工作区：左侧 Pulse 一体化侧边栏 + 右侧主内容区
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.pulseSidebar(c)

			ui.Column(c).Grow(1).Children(func() {
				// Pulse 顶部操作栏
				a.pulseMainHeader(c)

				// 主内容区域
				ui.Column(c).Grow(1).Padding(16, 24).Gap(14).Children(func() {
					switch a.activeTab {
					case "search":
						a.searchPageView(c)
					case "recommend":
						a.recommendPageView(c)
					case "downloads":
						a.downloadsPageView(c)
					case "custom_playlists":
						a.customPlaylistsPageView(c)
					case "my_playlists":
						a.myPlaylistsPageView(c)
					case "local_music":
						a.localMusicPageView(c)
					case "settings":
						a.settingsPageView(c)
					default:
						a.searchPageView(c)
					}

					// 悬浮实时歌词面板
					if a.showLyric && a.currentSong != nil {
						a.renderLyricsPanel(c)
					}
				})
			})
		})

		// Pulse 悬浮播放控制坞与实时状态指示器
		a.pulsePlayerDock(c)

		// 扫码授权模态弹窗浮层
		a.renderQRLoginModal(c)
	})
}

// pulseSidebar Pulse 风格一体化侧边栏 (复刻 Pulse 整体布局，支持明暗快速切换)
func (a *NativeApp) pulseSidebar(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()
	ctrlStyle := a.effectiveWindowControlsStyle()

	ui.Column(c).Width(216).Background(pal.SidebarBg).
		BorderWidth(0, 1, 0, 0).BorderColor(pal.SidebarBorder).Children(func() {

		// 顶栏 (macOS 下为 Traffic Lights 红黄绿；Windows / Linux 下为原生应用标识；右侧支持明暗快速切换)
		ui.Row(c).Height(50).Padding(0, 16).AlignItems(ui.Center).DragWindow().Children(func() {
			if ctrlStyle == WindowControlsMacOS {
				// macOS Traffic Lights (Close / Minimize / Zoom)
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					btnClose := ui.Row(c.Key("traffic-close")).Size(12, 12).Radius(6).Background(ui.Hex("#ff5f56")).Center()
					if btnClose.Clicked() && a.win != nil {
						a.win.Close()
					}

					btnMin := ui.Row(c.Key("traffic-min")).Size(12, 12).Radius(6).Background(ui.Hex("#ffbd2e")).Center()
					if btnMin.Clicked() && a.win != nil {
						a.win.Minimize()
					}

					btnZoom := ui.Row(c.Key("traffic-zoom")).Size(12, 12).Radius(6).Background(ui.Hex("#27c93f")).Center()
					if btnZoom.Clicked() && a.win != nil {
						a.win.ToggleMaximize()
					}
				})
			} else {
				// Windows / Linux: 左上角展示原生应用品牌与图标
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Row(c).Size(22, 22).Radius(6).Background(t.Accent).Center().Children(func() {
						ui.Icon(c, iconMusic).Size(12, 12).TextColor(t.AccentText)
					})
					ui.Text(c, appTitle).FontSize(13).Bold().TextColor(t.Text)
				})
			}

			ui.Spacer(c)

			// 快速明暗切换微按钮
			themeIcon := iconSun
			if !pal.IsDark {
				themeIcon = iconMoon
			}
			btnTheme := ui.Row(c.Key("btn-theme-toggle")).Size(26, 26).Radius(5).Background(pal.CardMiniBg).Center()
			if btnTheme.Clicked() {
				if a.isDark() {
					a.themeMode = ThemeModeLight
				} else {
					a.themeMode = ThemeModeDark
				}
				a.saveDesktopSettings()
			}
			btnTheme.Children(func() {
				ui.Icon(c, themeIcon).Size(13, 13).TextColor(pal.TextMuted)
			})
		})

		// 导航分组与项
		ui.Column(c).Padding(8, 12).Gap(16).Children(func() {
			// 分组 1: 发现 (DISCOVER)
			ui.Column(c).Gap(4).Children(func() {
				ui.Row(c).Padding(2, 10).Children(func() {
					ui.Text(c, "发现").FontSize(11).Bold().TextColor(pal.Faint)
				})
				a.pulseSidebarItem(c, "search", iconSearch, "搜索", 0)
				a.pulseSidebarItem(c, "recommend", iconRadio, "推荐歌单", 0)
			})

			// 分组 2: 资料库 (LIBRARY)
			ui.Column(c).Gap(4).Children(func() {
				ui.Row(c).Padding(2, 10).Children(func() {
					ui.Text(c, "资料库").FontSize(11).Bold().TextColor(pal.Faint)
				})
				a.pulseSidebarItem(c, "downloads", iconDownload, "已下载", len(a.downloads))
				a.pulseSidebarItem(c, "custom_playlists", iconHeart, "自制歌单", len(a.customPlaylists))
				a.pulseSidebarItem(c, "my_playlists", iconListMusic, "我的歌单", 0)
				a.pulseSidebarItem(c, "local_music", iconFolder, "本地音乐", len(a.localMusicFiles))
			})

			// 分组 3: 系统 (SYSTEM)
			ui.Column(c).Gap(4).Children(func() {
				ui.Row(c).Padding(2, 10).Children(func() {
					ui.Text(c, "系统").FontSize(11).Bold().TextColor(pal.Faint)
				})
				a.pulseSidebarItem(c, "settings", iconSettings, "设置", 0)
			})
		})

		ui.Spacer(c)

		// Pulse 底部设备/状态卡片 (复刻 Pulse overview-dark.jpg 与 overview-light.jpg 底部展示)
		ui.Column(c).Padding(14, 16).Gap(4).BorderWidth(1, 0, 0, 0).BorderColor(pal.SidebarBorder).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Row(c).Size(6, 6).Radius(3).Background(pal.Success) // Live Pulse LED
				ui.Text(c, "高解析度无损").FontSize(11).Bold().TextColor(t.Text)
			})
			ui.Text(c, appTitle+" v"+core.AppVersion).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "GPU 原生渲染 · 纯 Go 驱动").FontSize(10).TextColor(pal.Faint)
		})
	})
}

func (a *NativeApp) pulseSidebarItem(c *ui.Context, tabID string, icon *ui.SVG, label string, badgeCount int) {
	t := c.Theme()
	pal := a.colors()
	isSelected := a.activeTab == tabID

	btn := ui.Row(c.Key("nav-"+tabID)).Padding(8, 12).Radius(8).Gap(10).AlignItems(ui.Center)
	if isSelected {
		btn.Background(t.Accent).TextColor(t.AccentText)
	} else {
		btn.Background(pal.SidebarBg)
	}

	if btn.Clicked() {
		a.activeTab = tabID
	}

	btn.Children(func() {
		iconEl := ui.Icon(c, icon).Size(15, 15)
		if isSelected {
			iconEl.TextColor(t.AccentText)
		} else {
			iconEl.TextColor(pal.TextMuted)
		}

		textEl := ui.Text(c, label).FontSize(13)
		if isSelected {
			textEl.Bold()
		}

		if badgeCount > 0 {
			ui.Spacer(c)
			ui.Row(c).Padding(1, 6).Radius(10).Background(pal.CardMiniBg).Children(func() {
				ui.Text(c, fmt.Sprint(badgeCount)).FontSize(10).TextColor(pal.Text)
			})
		}
	})
}

// pulseMainHeader Pulse 风格顶部标题与操作栏 (支持跨平台右侧窗口控制按键)
func (a *NativeApp) pulseMainHeader(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	title := "搜索"
	subtitle := "全网无损音乐探索"
	switch a.activeTab {
	case "recommend":
		title = "推荐歌单"
		subtitle = "各大平台精选每日热门歌单"
	case "downloads":
		title = "已下载"
		subtitle = "下载位置: " + a.downloadDir
	case "custom_playlists":
		title = "自制歌单"
		subtitle = fmt.Sprintf("已创建 %d 个本地收藏夹，随时管理与离线聚合", len(a.customPlaylists))
	case "my_playlists":
		title = "我的歌单"
		subtitle = "已登录账号在云端创建与收藏的个人歌单"
	case "local_music":
		title = "本地音乐"
		subtitle = fmt.Sprintf("已扫描 %d 首本地音频文件", len(a.localMusicFiles))
	case "settings":
		title = "设置"
		subtitle = "应用偏好、账号 Cookie 与云端同步"
	default:
		if len(a.searchResults) > 0 {
			subtitle = fmt.Sprintf("共 %d 首歌曲 · %s", len(a.searchResults), sourceDisplayNames[a.selectedSource])
		} else if len(a.playlistResults) > 0 {
			subtitle = fmt.Sprintf("共 %d 个歌单/专辑", len(a.playlistResults))
		}
	}

	ui.Row(c).Height(50).Padding(0, 0, 0, 24).AlignItems(ui.Center).DragWindow().
		BorderWidth(0, 0, 1, 0).BorderColor(pal.HeaderBorder).Children(func() {

		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(18).Bold().TextColor(t.Text)
			ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted)
		})

		ui.Spacer(c)

		// 搜索栏与类型切换 (仅在搜索页展示)
		if a.activeTab == "search" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(0, 12, 0, 0).Children(func() {
				// 搜索类型分段器 (单曲 / 歌单 / 专辑)
				types := []struct{ id, name string }{
					{"song", "单曲"},
					{"playlist", "歌单"},
					{"album", "专辑"},
				}
				ui.Row(c).Padding(2).Radius(6).Background(pal.PillBg).Border(1, pal.PillBorder).Gap(2).Children(func() {
					for _, it := range types {
						opt := it
						isSelected := a.searchType == opt.id
						item := ui.Row(c.Key("type-"+opt.id)).Padding(3, 8).Radius(5).AlignItems(ui.Center)
						if isSelected {
							item.Background(t.Accent).TextColor(t.AccentText)
						} else {
							item.Background(pal.PillBg)
						}
						if item.Clicked() {
							a.searchType = opt.id
							if a.searchKeyword != "" {
								a.startSearch()
							}
						}
						item.Children(func() {
							lbl := ui.Text(c, opt.name).FontSize(11)
							if isSelected {
								lbl.Bold()
							}
						})
					}
				})

				searchField := ui.SearchField(c, &a.searchKeyword).
					Placeholder("搜索歌曲、歌单、专辑或粘贴分享链接").Width(240)
				if searchField.Submitted() {
					a.startSearch()
				}

				btn := ui.PrimaryButton(c, "搜索").Disabled(a.searching || strings.TrimSpace(a.searchKeyword) == "")
				if btn.Clicked() {
					a.startSearch()
				}
			})
		}

		// 右侧各平台窗口控制按钮 (WinUI 3 Caption Buttons / Linux GNOME / Linux KDE)
		a.pulseWindowControlsRight(c)
	})
}

// pulseWindowControlsRight 右端跨平台窗口控制按钮 (WinUI 3 标题栏贴边矩形规范)
func (a *NativeApp) pulseWindowControlsRight(c *ui.Context) {
	pal := a.colors()
	ctrlStyle := a.effectiveWindowControlsStyle()

	switch ctrlStyle {
	case WindowControlsWindows:
		// Windows 10/11 WinUI 3 规范贴边矩形 Caption Buttons (46px 宽，与顶栏等高，1px 纤细 Segoe 矢量图符)
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			// 1. 最小化 (WinUI 3: 10px 极细横线)
			btnMin := ui.Row(c.Key("win-min")).Size(46, 50).Center()
			if btnMin.Hovered() {
				if pal.IsDark {
					btnMin.Background(ui.Hex("#ffffff15"))
				} else {
					btnMin.Background(ui.Hex("#0000000d"))
				}
			}
			if btnMin.Clicked() && a.win != nil {
				a.win.Minimize()
			}
			btnMin.Children(func() {
				glyphColor := pal.TextMuted
				if btnMin.Hovered() {
					glyphColor = pal.Text
				}
				ui.Icon(c, iconWinMinimize).Size(10, 10).TextColor(glyphColor)
			})

			// 2. 最大化 / 还原 (WinUI 3: 单层方块 / 还原双层方块)
			btnMax := ui.Row(c.Key("win-max")).Size(46, 50).Center()
			if btnMax.Hovered() {
				if pal.IsDark {
					btnMax.Background(ui.Hex("#ffffff15"))
				} else {
					btnMax.Background(ui.Hex("#0000000d"))
				}
			}
			if btnMax.Clicked() && a.win != nil {
				a.win.ToggleMaximize()
			}
			btnMax.Children(func() {
				maxIcon := iconWinMaximize
				if a.win != nil && a.win.IsMaximized() {
					maxIcon = iconWinRestore
				}
				glyphColor := pal.TextMuted
				if btnMax.Hovered() {
					glyphColor = pal.Text
				}
				ui.Icon(c, maxIcon).Size(10, 10).TextColor(glyphColor)
			})

			// 3. 关闭 (WinUI 3: 悬停变 Critical Red #c42b1c，按压 #b82315，图符纯白)
			btnClose := ui.Row(c.Key("win-close")).Size(46, 50).Center()
			if btnClose.Pressed() {
				btnClose.Background(ui.Hex("#b82315"))
			} else if btnClose.Hovered() {
				btnClose.Background(ui.Hex("#c42b1c"))
			}
			if btnClose.Clicked() && a.win != nil {
				a.win.Close()
			}
			btnClose.Children(func() {
				closeColor := pal.TextMuted
				if btnClose.Hovered() || btnClose.Pressed() {
					closeColor = ui.Hex("#ffffff")
				}
				ui.Icon(c, iconWinClose).Size(10, 10).TextColor(closeColor)
			})
		})

	case WindowControlsLinuxGNOME:
		// GNOME / GTK (Adwaita / Libadwaita) 经典圆形药丸按钮
		ui.Row(c).Padding(0, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			btnMin := ui.Row(c.Key("gnome-min")).Size(26, 26).Radius(13).Background(pal.CardMiniBg).Center()
			if btnMin.Hovered() {
				btnMin.Background(pal.SurfaceHover)
			}
			if btnMin.Clicked() && a.win != nil {
				a.win.Minimize()
			}
			btnMin.Children(func() {
				ui.Icon(c, iconMinimize).Size(10, 10).TextColor(pal.Text)
			})

			btnMax := ui.Row(c.Key("gnome-max")).Size(26, 26).Radius(13).Background(pal.CardMiniBg).Center()
			if btnMax.Hovered() {
				btnMax.Background(pal.SurfaceHover)
			}
			if btnMax.Clicked() && a.win != nil {
				a.win.ToggleMaximize()
			}
			btnMax.Children(func() {
				ui.Icon(c, iconMaximize).Size(10, 10).TextColor(pal.Text)
			})

			btnClose := ui.Row(c.Key("gnome-close")).Size(26, 26).Radius(13).Background(pal.CardMiniBg).Center()
			if btnClose.Hovered() {
				btnClose.Background(ui.Hex("#e01b24"))
			}
			if btnClose.Clicked() && a.win != nil {
				a.win.Close()
			}
			btnClose.Children(func() {
				closeColor := pal.Text
				if btnClose.Hovered() {
					closeColor = ui.Hex("#ffffff")
				}
				ui.Icon(c, iconClose).Size(10, 10).TextColor(closeColor)
			})
		})

	case WindowControlsLinuxKDE:
		// KDE Plasma / Qt (Breeze) 几何方块微图标
		ui.Row(c).Padding(0, 12).Gap(4).AlignItems(ui.Center).Children(func() {
			btnMin := ui.Row(c.Key("kde-min")).Size(28, 28).Radius(4).Center()
			if btnMin.Hovered() {
				btnMin.Background(ui.Hex("#3daee924"))
			}
			if btnMin.Clicked() && a.win != nil {
				a.win.Minimize()
			}
			btnMin.Children(func() {
				iconColor := pal.Text
				if btnMin.Hovered() {
					iconColor = ui.Hex("#3daee9")
				}
				ui.Icon(c, iconMinimize).Size(12, 12).TextColor(iconColor)
			})

			btnMax := ui.Row(c.Key("kde-max")).Size(28, 28).Radius(4).Center()
			if btnMax.Hovered() {
				btnMax.Background(ui.Hex("#3daee924"))
			}
			if btnMax.Clicked() && a.win != nil {
				a.win.ToggleMaximize()
			}
			btnMax.Children(func() {
				iconColor := pal.Text
				if btnMax.Hovered() {
					iconColor = ui.Hex("#3daee9")
				}
				ui.Icon(c, iconMaximize).Size(12, 12).TextColor(iconColor)
			})

			btnClose := ui.Row(c.Key("kde-close")).Size(28, 28).Radius(4).Center()
			if btnClose.Hovered() {
				btnClose.Background(ui.Hex("#da4453"))
			}
			if btnClose.Clicked() && a.win != nil {
				a.win.Close()
			}
			btnClose.Children(func() {
				iconColor := pal.Text
				if btnClose.Hovered() {
					iconColor = ui.Hex("#ffffff")
				}
				ui.Icon(c, iconClose).Size(12, 12).TextColor(iconColor)
			})
		})

	default:
		// macOS: 控制按钮位于左上角侧边栏内，右端留白
	}
}

// searchPageView 音乐搜索主页面 (支持单曲/歌单/专辑/链接直达解析)
func (a *NativeApp) searchPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	// 若当前处于歌单/专辑详情展开视图
	if a.selectedPlaylist != nil {
		a.playlistDetailPageView(c)
		return
	}

	// 1. Pulse 风格音源分段选择器 (仅单曲搜索时展示细分音源)
	ui.Row(c).Padding(3).Radius(8).Background(pal.PillBg).Border(1, pal.PillBorder).Gap(3).AlignItems(ui.Center).Children(func() {
		for _, srcKey := range a.sourceOptions {
			key := srcKey
			name := sourceDisplayNames[key]
			isSelected := a.selectedSource == key

			item := ui.Row(c.Key("seg-"+key)).Padding(5, 12).Radius(6).AlignItems(ui.Center)
			if isSelected {
				item.Background(t.Accent).TextColor(t.AccentText)
			} else {
				item.Background(pal.PillBg)
			}

			if item.Clicked() {
				a.selectedSource = key
				if a.searchKeyword != "" {
					a.startSearch()
				}
			}

			item.Children(func() {
				lbl := ui.Text(c, name).FontSize(12)
				if isSelected {
					lbl.Bold()
				}
			})
		}
	})

	// 2. 搜索状态处理
	if a.searching {
		ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
			ui.Spinner(c)
			ui.Text(c, "正在搜索…").TextColor(t.TextMuted).FontSize(14)
		})
		return
	}

	if a.searchError != "" {
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			ui.Row(c).Size(44, 44).Radius(22).Background(pal.CardMiniBg).Center().Children(func() {
				ui.Icon(c, iconClose).Size(20, 20).TextColor(t.Danger)
			})
			ui.Text(c, "未能完成搜索: "+a.searchError).TextColor(t.Danger).FontSize(14)
			if ui.Button(c, "重试").Clicked() {
				a.startSearch()
			}
		})
		return
	}

	// 3. 空状态展示
	if len(a.searchResults) == 0 && len(a.playlistResults) == 0 {
		ui.Column(c).Grow(1).Center().Gap(20).Children(func() {
			ui.Column(c).Padding(28).Radius(14).Background(t.Surface).Border(1, t.Border).Gap(18).Width(640).Center().Children(func() {
				ui.Row(c).Size(56, 56).Radius(16).Background(pal.CardMiniBg).Border(1, pal.Border).Center().Children(func() {
					ui.Icon(c, iconMusic).Size(28, 28).TextColor(t.Accent)
				})
				ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Text(c, "开始探索").FontSize(20).Bold().TextColor(t.Text)
					ui.Text(c, "搜索全网音乐，支持即时试听与无损下载。").FontSize(13).TextColor(t.TextMuted)
				})

				// Pulse 3个特性迷你指示卡片 (LED dot 指示灯风格)
				ui.Row(c).Gap(12).Padding(8, 0, 0).Children(func() {
					pulseMiniFeature(c, pal, pal.Accent, "多源聚合", "网易云/QQ/酷狗等")
					pulseMiniFeature(c, pal, pal.Success, "高解析度无损", "FLAC / 320K 原生流")
					pulseMiniFeature(c, pal, pal.Warning, "GPU 加速", "MyGo 原生渲染引擎")
				})
			})
		})
		return
	}

	// 4. 歌单 / 专辑搜索结果展示网格
	if a.searchType != "song" && len(a.playlistResults) > 0 {
		ui.Column(c).Grow(1).Gap(10).Children(func() {
			ui.Text(c, fmt.Sprintf("已找到 %d 个%s", len(a.playlistResults), sourceDisplayNames[a.searchType])).FontSize(13).Bold().TextColor(t.Text)
			ui.Scroll(c).Grow(1).Gap(10).Children(func() {
				ui.Row(c).Wrap().Gap(12).Children(func() {
					for _, item := range a.playlistResults {
						pl := item
						card := ui.Column(c.Key("pl-res-"+pl.ID)).
							Width(280).Padding(14).Radius(12).Background(t.Surface).Border(1, t.Border).Gap(8)
						if card.Clicked() {
							a.openPlaylistDetail(pl, a.searchType == "album")
						}
						card.Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
								ui.Row(c).Size(38, 38).Radius(8).Background(pal.CardMiniBg).Center().Children(func() {
									ui.Icon(c, iconDisc).Size(18, 18).TextColor(t.Accent)
								})
								ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
									ui.Text(c, pl.Name).FontSize(13).Bold().TextColor(t.Text)
									ui.Text(c, pl.Creator).FontSize(11).TextColor(t.TextMuted)
								})
							})
							if pl.Description != "" {
								ui.Text(c, pl.Description).FontSize(11).TextColor(pal.Faint)
							}
							ui.Row(c).AlignItems(ui.Center).Children(func() {
								if pl.TrackCount > 0 {
									ui.Text(c, fmt.Sprintf("%d 首", pl.TrackCount)).FontSize(10).TextColor(pal.Faint)
								}
								ui.Spacer(c)
								ui.Row(c).Padding(1, 6).Radius(4).Background(pal.CardMiniBg).Children(func() {
									ui.Text(c, sourceDisplayNames[pl.Source]).FontSize(10).TextColor(t.TextMuted)
								})
							})
						})
					}
				})
			})
		})
		return
	}

	// 5. 单曲搜索结果列表 - Pulse 进程表风格
	ui.Column(c).Grow(1).Gap(6).Children(func() {
		a.renderSongTable(c, a.searchResults)

		// 底部加载扩展音源结果操作栏
		if len(a.searchResults) > 0 {
			ui.Row(c).Height(46).Padding(6, 0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				if a.loadingMore {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c)
						ui.Text(c, "正在检索扩展音源…").FontSize(12).TextColor(pal.Accent)
					})
				} else if a.hasLoadedMore {
					ui.Text(c, fmt.Sprintf("已聚合全部可用音源 (共 %d 首)", len(a.searchResults))).FontSize(12).TextColor(pal.Faint)
				} else {
					loadBtnText := fmt.Sprintf("加载更多音源 (已载入 %d 首)", len(a.searchResults))
					if ui.Button(c.Key("btn-load-more"), loadBtnText).Clicked() {
						a.loadMoreSearchResults()
					}
				}
			})
		}
	})
}

// playlistDetailPageView 歌单与专辑曲目详情展开视图
func (a *NativeApp) playlistDetailPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()
	pl := a.selectedPlaylist
	if pl == nil {
		return
	}

	ui.Column(c).Grow(1).Gap(12).Children(func() {
		// 顶部导航与歌单元数据头
		ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
			if ui.Button(c, "返回").Clicked() {
				a.selectedPlaylist = nil
				a.playlistSongs = nil
			}

			ui.Row(c).Size(40, 40).Radius(10).Background(pal.CardMiniBg).Center().Children(func() {
				ui.Icon(c, iconDisc).Size(20, 20).TextColor(t.Accent)
			})

			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, pl.Name).FontSize(15).Bold().TextColor(t.Text)
					ui.Row(c).Padding(1, 5).Radius(4).Background(pal.CardMiniBg).Children(func() {
						ui.Text(c, sourceDisplayNames[pl.Source]).FontSize(10).TextColor(t.TextMuted)
					})
				})
				metaStr := pl.Creator
				if len(a.playlistSongs) > 0 {
					metaStr += fmt.Sprintf(" · 共 %d 首曲目", len(a.playlistSongs))
				}
				ui.Text(c, metaStr).FontSize(11).TextColor(t.TextMuted)
			})

			// 批量操作
			if len(a.playlistSongs) > 0 {
				if ui.PrimaryButton(c, "全部下载").Clicked() {
					a.batchDownloadSongs(a.playlistSongs)
					c.Toast(fmt.Sprintf("已将 %d 首曲目加入下载队列", len(a.playlistSongs)))
				}
				if ui.Button(c, "播放全部").Clicked() {
					a.playSong(a.playlistSongs[0])
				}
			}
		})

		if a.loadingPlaylistDetail {
			ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在解析歌单曲目…").TextColor(t.TextMuted).FontSize(14)
			})
			return
		}

		if a.playlistDetailError != "" {
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Text(c, "解析遇到问题: "+a.playlistDetailError).TextColor(t.Danger)
			})
			return
		}

		// 曲目列表
		a.renderSongTable(c, a.playlistSongs)
	})
}

// recommendPageView 推荐歌单页面 (调用平台热门与每日精选接口)
func (a *NativeApp) recommendPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	if a.selectedPlaylist != nil {
		a.playlistDetailPageView(c)
		return
	}

	ui.Column(c).Grow(1).Gap(14).Children(func() {
		// 顶部音源选择与刷新按钮
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "平台精选:").FontSize(13).Bold().TextColor(t.Text)

			sources := []struct{ id, name string }{
				{"netease", "网易云音乐"},
				{"qq", "QQ 音乐"},
				{"kugou", "酷狗音乐"},
				{"kuwo", "酷我音乐"},
			}

			ui.Row(c).Padding(2).Radius(8).Background(pal.PillBg).Border(1, pal.PillBorder).Gap(2).Children(func() {
				for _, it := range sources {
					opt := it
					isSelected := a.selectedRecommendSource == opt.id
					item := ui.Row(c.Key("rec-src-"+opt.id)).Padding(4, 10).Radius(6).AlignItems(ui.Center)
					if isSelected {
						item.Background(t.Accent).TextColor(t.AccentText)
					} else {
						item.Background(pal.PillBg)
					}
					if item.Clicked() {
						a.selectedRecommendSource = opt.id
						a.loadRecommendPlaylists()
					}
					item.Children(func() {
						lbl := ui.Text(c, opt.name).FontSize(11)
						if isSelected {
							lbl.Bold()
						}
					})
				}
			})

			ui.Spacer(c)

			if ui.Button(c, "刷新推荐").Clicked() {
				a.loadRecommendPlaylists()
			}
		})

		if a.loadingRecommend {
			ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在获取精选推荐歌单…").TextColor(t.TextMuted).FontSize(14)
			})
			return
		}

		if a.recommendError != "" {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				ui.Row(c).Size(44, 44).Radius(22).Background(pal.CardMiniBg).Center().Children(func() {
					ui.Icon(c, iconClose).Size(20, 20).TextColor(pal.Danger)
				})
				ui.Text(c, "未能获取推荐歌单: "+a.recommendError).TextColor(pal.Danger).FontSize(13)
				if ui.Button(c, "重新尝试").Clicked() {
					a.loadRecommendPlaylists()
				}
			})
			return
		}

		if len(a.recommendPlaylists) == 0 {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				ui.Text(c, "暂未加载到推荐歌单").TextColor(t.TextMuted)
				if ui.Button(c, "获取推荐").Clicked() {
					a.loadRecommendPlaylists()
				}
			})
			return
		}

		// 歌单网格卡片
		ui.Scroll(c).Grow(1).Gap(10).Children(func() {
			ui.Row(c).Wrap().Gap(12).Children(func() {
				for _, item := range a.recommendPlaylists {
					pl := item
					card := ui.Column(c.Key("rec-pl-"+pl.ID)).
						Width(280).Padding(14).Radius(12).Background(t.Surface).Border(1, t.Border).Gap(8)
					if card.Clicked() {
						a.openPlaylistDetail(pl, false)
					}
					card.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
							ui.Row(c).Size(40, 40).Radius(8).Background(pal.CardMiniBg).Center().Children(func() {
								ui.Icon(c, iconDisc).Size(20, 20).TextColor(t.Accent)
							})
							ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, pl.Name).FontSize(13).Bold().TextColor(t.Text)
								ui.Text(c, pl.Creator).FontSize(11).TextColor(t.TextMuted)
							})
						})
						if pl.Description != "" {
							ui.Text(c, pl.Description).FontSize(11).TextColor(pal.Faint)
						}
						ui.Row(c).AlignItems(ui.Center).Children(func() {
							if pl.PlayCount > 0 {
								ui.Text(c, fmt.Sprintf("播放: %d", pl.PlayCount)).FontSize(10).TextColor(pal.Faint)
							}
							ui.Spacer(c)
							ui.Row(c).Padding(1, 6).Radius(4).Background(pal.CardMiniBg).Children(func() {
								ui.Text(c, sourceDisplayNames[pl.Source]).FontSize(10).TextColor(t.TextMuted)
							})
						})
					})
				}
			})
		})
	})
}

// customPlaylistsPageView 本地自制歌单与收藏夹管理
func (a *NativeApp) customPlaylistsPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	// 1. 如果选中了某个自制歌单，展开其曲目详情
	if a.selectedCustomColID != "" {
		var activeCol *CustomPlaylist
		for i := range a.customPlaylists {
			if a.customPlaylists[i].ID == a.selectedCustomColID {
				activeCol = &a.customPlaylists[i]
				break
			}
		}

		if activeCol == nil {
			a.selectedCustomColID = ""
			return
		}

		ui.Column(c).Grow(1).Gap(12).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				if ui.Button(c, "返回").Clicked() {
					a.selectedCustomColID = ""
				}

				ui.Row(c).Size(40, 40).Radius(10).Background(pal.CardMiniBg).Center().Children(func() {
					ui.Icon(c, iconHeart).Size(20, 20).TextColor(pal.Danger)
				})

				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, activeCol.Name).FontSize(15).Bold().TextColor(t.Text)
						ui.Row(c).Padding(1, 5).Radius(4).Background(pal.CardMiniBg).Children(func() {
							ui.Text(c, "本地自制").FontSize(10).TextColor(t.TextMuted)
						})
					})
					meta := fmt.Sprintf("创建于 %s · 共 %d 首歌曲", activeCol.CreatedAt, len(activeCol.Songs))
					if activeCol.Description != "" {
						meta += " · " + activeCol.Description
					}
					ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted)
				})

				if len(activeCol.Songs) > 0 {
					if ui.PrimaryButton(c, "全部下载").Clicked() {
						a.batchDownloadSongs(activeCol.Songs)
						c.Toast(fmt.Sprintf("已将 %d 首自制歌单歌曲加入下载队列", len(activeCol.Songs)))
					}
					if ui.Button(c, "播放全部").Clicked() {
						a.playSongFromList(activeCol.Songs[0], activeCol.Songs, 0)
					}
				}

				if activeCol.ID != "fav_01" {
					if ui.Button(c.Key("del-col-"+activeCol.ID), "删除歌单").Clicked() {
						a.deleteCustomPlaylist(activeCol.ID)
						a.selectedCustomColID = ""
						c.Toast("已删除歌单")
					}
				}
			})

			if len(activeCol.Songs) == 0 {
				ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
					ui.Column(c).Padding(28).Radius(14).Background(t.Surface).Border(1, t.Border).Gap(12).Width(480).Center().Children(func() {
						ui.Row(c).Size(52, 52).Radius(16).Background(pal.CardMiniBg).Border(1, pal.Border).Center().Children(func() {
							ui.Icon(c, iconHeart).Size(26, 26).TextColor(pal.Danger)
						})
						ui.Text(c, "歌单暂无歌曲").FontSize(16).Bold().TextColor(t.Text)
						ui.Text(c, "在音乐搜索中点击“收藏”即可将心仪曲目收录至此。").FontSize(12).TextColor(t.TextMuted)
					})
				})
				return
			}

			// 歌曲表格
			a.renderSongTable(c, activeCol.Songs)
		})
		return
	}

	// 2. 自制歌单总览与管理
	ui.Column(c).Grow(1).Gap(14).Children(func() {
		// 顶部工具栏
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, fmt.Sprintf("我的收藏夹 (%d 个歌单)", len(a.customPlaylists))).FontSize(14).Bold().TextColor(t.Text)
			ui.Spacer(c)

			if ui.PrimaryButton(c.Key("btn-show-create-col"), "新建歌单").Clicked() {
				a.showCreateColDialog = true
				a.newColNameInput = ""
				a.newColDescInput = ""
			}
		})

		// 新建歌单对话框
		if a.showCreateColDialog {
			ui.Row(c).Padding(12, 16).Radius(10).Background(pal.CardMiniBg).Border(1, pal.Accent).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, "歌单名称:").FontSize(12).Bold().TextColor(t.Text)
				ui.TextInput(c, &a.newColNameInput).Width(160).Placeholder("输入新歌单名称")
				ui.Text(c, "描述:").FontSize(12).TextColor(pal.Faint)
				ui.TextInput(c, &a.newColDescInput).Width(220).Placeholder("描述或标签 (可选)")

				if ui.PrimaryButton(c, "保存创建").Clicked() {
					name := strings.TrimSpace(a.newColNameInput)
					if name != "" {
						a.createCustomPlaylist(name, a.newColDescInput)
						a.showCreateColDialog = false
						c.Toast("已创建自制歌单: " + name)
					}
				}
				if ui.Button(c, "取消").Clicked() {
					a.showCreateColDialog = false
				}
			})
		}

		// 歌单卡片网格
		ui.Scroll(c).Grow(1).Gap(10).Children(func() {
			ui.Row(c).Wrap().Gap(12).Children(func() {
				for _, item := range a.customPlaylists {
					col := item
					card := ui.Column(c.Key("custom-col-"+col.ID)).
						Width(280).Padding(14).Radius(12).Background(t.Surface).Border(1, t.Border).Gap(8)
					if card.Clicked() {
						a.selectedCustomColID = col.ID
					}
					card.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
							ui.Row(c).Size(40, 40).Radius(8).Background(pal.CardMiniBg).Center().Children(func() {
								ui.Icon(c, iconHeart).Size(20, 20).TextColor(pal.Danger)
							})
							ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, col.Name).FontSize(13).Bold().TextColor(t.Text)
								ui.Text(c, fmt.Sprintf("%d 首歌曲 · %s", len(col.Songs), col.CreatedAt)).FontSize(11).TextColor(t.TextMuted)
							})
						})
						if col.Description != "" {
							ui.Text(c, col.Description).FontSize(11).TextColor(pal.Faint)
						}
						ui.Row(c).AlignItems(ui.Center).Children(func() {
							ui.Row(c).Padding(1, 6).Radius(4).Background(pal.CardMiniBg).Children(func() {
								ui.Text(c, "本地自建").FontSize(10).TextColor(t.TextMuted)
							})
							ui.Spacer(c)
							if ui.Button(c.Key("open-col-"+col.ID), "打开").Clicked() {
								a.selectedCustomColID = col.ID
							}
						})
					})
				}
			})
		})
	})
}

// myPlaylistsPageView 我的歌单页面 (展示已登录账号的个人歌单与收藏)
func (a *NativeApp) myPlaylistsPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	if a.selectedPlaylist != nil {
		a.playlistDetailPageView(c)
		return
	}

	ui.Column(c).Grow(1).Gap(14).Children(func() {
		// 顶部选择已登录平台
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "账号来源:").FontSize(13).Bold().TextColor(t.Text)

			sources := []struct{ id, name string }{
				{"netease", "网易云音乐"},
				{"qq", "QQ 音乐"},
				{"kugou", "酷狗音乐"},
				{"bilibili", "哔哩哔哩"},
			}

			ui.Row(c).Padding(2).Radius(8).Background(pal.PillBg).Border(1, pal.PillBorder).Gap(2).Children(func() {
				for _, it := range sources {
					opt := it
					isSelected := a.selectedMySource == opt.id
					item := ui.Row(c.Key("my-src-"+opt.id)).Padding(4, 10).Radius(6).AlignItems(ui.Center)
					if isSelected {
						item.Background(t.Accent).TextColor(t.AccentText)
					} else {
						item.Background(pal.PillBg)
					}
					if item.Clicked() {
						a.selectedMySource = opt.id
						a.loadMyPlaylists()
					}
					item.Children(func() {
						lbl := ui.Text(c, opt.name).FontSize(11)
						if isSelected {
							lbl.Bold()
						}
					})
				}
			})

			ui.Spacer(c)

			if ui.Button(c, "同步歌单").Clicked() {
				a.loadMyPlaylists()
			}
		})

		if a.loadingMyPlaylists {
			ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在同步你的云端歌单…").TextColor(t.TextMuted).FontSize(14)
			})
			return
		}

		// 检查当前所选源是否配置了 Cookie
		cookie := a.getPlatformCookie(a.selectedMySource)
		if strings.TrimSpace(cookie) == "" {
			ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
				ui.Column(c).Padding(28).Radius(14).Background(t.Surface).Border(1, t.Border).Gap(12).Width(520).Center().Children(func() {
					ui.Row(c).Size(52, 52).Radius(16).Background(pal.CardMiniBg).Border(1, pal.Border).Center().Children(func() {
						ui.Icon(c, iconQrCode).Size(26, 26).TextColor(t.Accent)
					})
					ui.Text(c, sourceDisplayNames[a.selectedMySource]+" 未登录").FontSize(16).Bold().TextColor(t.Text)
					ui.Text(c, "前往设置扫码登录或配置 Cookie，即可在此同步你的个人歌单与收藏。").FontSize(12).TextColor(t.TextMuted)
					if ui.PrimaryButton(c, "去扫码登录").Clicked() {
						a.activeTab = "settings"
						a.startQRLogin(a.selectedMySource)
					}
				})
			})
			return
		}

		if a.myPlaylistsError != "" {
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Text(c, "同步遇到问题: "+a.myPlaylistsError).TextColor(t.Danger)
				if ui.Button(c, "重新同步").Clicked() {
					a.loadMyPlaylists()
				}
			})
			return
		}

		if len(a.myPlaylists) == 0 {
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Text(c, "暂未同步到个人歌单").TextColor(t.TextMuted)
				if ui.Button(c, "拉取歌单").Clicked() {
					a.loadMyPlaylists()
				}
			})
			return
		}

		// 个人歌单网格卡片
		ui.Scroll(c).Grow(1).Gap(10).Children(func() {
			ui.Row(c).Wrap().Gap(12).Children(func() {
				for _, item := range a.myPlaylists {
					pl := item
					card := ui.Column(c.Key("my-pl-"+pl.ID)).
						Width(280).Padding(14).Radius(12).Background(t.Surface).Border(1, t.Border).Gap(8)
					if card.Clicked() {
						a.openPlaylistDetail(pl, false)
					}
					card.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
							ui.Row(c).Size(40, 40).Radius(8).Background(pal.CardMiniBg).Center().Children(func() {
								ui.Icon(c, iconDisc).Size(20, 20).TextColor(t.Accent)
							})
							ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, pl.Name).FontSize(13).Bold().TextColor(t.Text)
								ui.Text(c, pl.Creator).FontSize(11).TextColor(t.TextMuted)
							})
						})
						if pl.Description != "" {
							ui.Text(c, pl.Description).FontSize(11).TextColor(pal.Faint)
						}
						ui.Row(c).AlignItems(ui.Center).Children(func() {
							if pl.TrackCount > 0 {
								ui.Text(c, fmt.Sprintf("%d 首歌曲", pl.TrackCount)).FontSize(10).TextColor(pal.Faint)
							}
							ui.Spacer(c)
							ui.Row(c).Padding(1, 6).Radius(4).Background(pal.CardMiniBg).Children(func() {
								ui.Text(c, sourceDisplayNames[pl.Source]).FontSize(10).TextColor(t.TextMuted)
							})
						})
					})
				}
			})
		})
	})
}

// localMusicPageView 本地音乐库页面 (扫描已下载音频文件，支持纯 Go 离线播放)
func (a *NativeApp) localMusicPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	ui.Column(c).Grow(1).Gap(14).Children(func() {
		// 顶部工具栏
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Icon(c, iconFolder).Size(16, 16).TextColor(t.Accent)
			ui.Text(c, "本地目录:").FontSize(13).Bold().TextColor(t.Text)
			ui.Text(c, a.downloadDir).FontSize(12).TextColor(t.TextMuted).Grow(1)

			if ui.Button(c, "打开文件夹").Clicked() {
				_ = mygo.Shell.OpenPath(a.downloadDir)
			}

			if ui.Button(c, "重新扫描").Clicked() {
				a.refreshLocalMusic()
				c.Toast("已重新扫描本地音乐")
			}
		})

		if a.loadingLocalMusic {
			ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在扫描本地音乐文件…").TextColor(t.TextMuted).FontSize(14)
			})
			return
		}

		if len(a.localMusicFiles) == 0 {
			ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
				ui.Column(c).Padding(28).Radius(14).Background(t.Surface).Border(1, t.Border).Gap(12).Width(480).Center().Children(func() {
					ui.Row(c).Size(52, 52).Radius(16).Background(pal.CardMiniBg).Border(1, pal.Border).Center().Children(func() {
						ui.Icon(c, iconFolder).Size(26, 26).TextColor(t.Accent)
					})
					ui.Text(c, "本地未找到音乐文件").FontSize(16).Bold().TextColor(t.Text)
					ui.Text(c, "下载或放置音频文件至下载目录后，将在此自动罗列。").FontSize(12).TextColor(t.TextMuted)
				})
			})
			return
		}

		// 表头
		ui.Row(c).Height(30).Padding(0, 12).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(pal.HeaderBorder).Children(func() {
			ui.Text(c, "#").FontSize(11).Bold().TextColor(pal.Faint).Width(32)
			ui.Text(c, "音频文件名").FontSize(11).Bold().TextColor(pal.Faint).Grow(1)
			ui.Text(c, "格式").FontSize(11).Bold().TextColor(pal.Faint).Width(60)
			ui.Row(c).Width(80).Justify(ui.End).Children(func() {
				ui.Text(c, "大小").FontSize(11).Bold().TextColor(pal.Faint)
			})
			ui.Row(c).Width(140).Justify(ui.End).Children(func() {
				ui.Text(c, "修改时间").FontSize(11).Bold().TextColor(pal.Faint)
			})
			ui.Row(c).Width(160).Justify(ui.End).Children(func() {
				ui.Text(c, "操作").FontSize(11).Bold().TextColor(pal.Faint)
			})
		})

		// 列表区
		ui.Scroll(c).Grow(1).Gap(4).Children(func() {
			for i, item := range a.localMusicFiles {
				file := item
				isCurrentPlaying := a.currentSong != nil && a.currentSong.ID == file.Path && a.playerPlaying

				row := ui.Row(c.Key(fmt.Sprintf("local-%d", i))).
					Padding(6, 12).Radius(8).AlignItems(ui.Center)

				if isCurrentPlaying {
					row.Background(pal.SongRowActiveBg).Border(1, pal.Accent)
				} else {
					row.Background(pal.SongRowBg).Border(1, pal.Border)
				}

				row.Children(func() {
					ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(11).TextColor(pal.Faint).Width(32)

					ui.Row(c).Grow(1).AlignItems(ui.Center).Gap(10).MinWidth(0).Children(func() {
						ui.Row(c).Size(28, 28).Radius(6).Background(pal.CardMiniBg).Center().Children(func() {
							ui.Icon(c, iconMusic).Size(14, 14).TextColor(t.Accent)
						})
						ui.Text(c, file.Filename).FontSize(13).Bold().TextColor(t.Text)
					})

					ui.Row(c).Width(60).Children(func() {
						ui.Row(c).Padding(1, 5).Radius(3).Background(pal.CardMiniBg).Children(func() {
							ui.Text(c, strings.ToUpper(file.Ext)).FontSize(9).TextColor(pal.TextMuted)
						})
					})

					ui.Row(c).Width(80).Justify(ui.End).Children(func() {
						ui.Text(c, core.FormatSize(file.Size)).FontSize(11).TextColor(t.TextMuted)
					})

					ui.Row(c).Width(140).Justify(ui.End).Children(func() {
						ui.Text(c, file.ModTime).FontSize(11).TextColor(pal.Faint)
					})

					ui.Row(c).Width(160).Justify(ui.End).Gap(6).Children(func() {
						playLabel := "播放"
						if isCurrentPlaying {
							playLabel = "暂停"
						}
						btnPlay := ui.Button(c.Key("play-local-"+file.Filename), playLabel)
						if btnPlay.Clicked() {
							if isCurrentPlaying {
								a.pauseAudio()
							} else {
								a.playLocalFile(file)
							}
						}

						if ui.Button(c.Key("loc-file-"+file.Filename), "定位").Clicked() {
							mygo.Shell.ShowItemInFolder(file.Path)
						}
					})
				})
			}
		})
	})
}

// renderSongTable 通用歌曲表格呈现组件 (支持批量选择、一键收藏/移出自制歌单与换源)
func (a *NativeApp) renderSongTable(c *ui.Context, songs []model.Song) {
	t := c.Theme()
	pal := a.colors()

	// 表头
	ui.Row(c).Height(32).Padding(0, 12).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.HeaderBorder).Children(func() {

		// 批量选择开关 / 全选
		batchBtnText := "批量"
		if a.batchMode {
			batchBtnText = "取消批量"
		}
		btnBatchToggle := ui.Button(c.Key("btn-toggle-batch"), batchBtnText)
		if btnBatchToggle.Clicked() {
			a.batchMode = !a.batchMode
			if !a.batchMode {
				a.selectedSongIDs = make(map[string]bool)
			}
		}

		if a.batchMode {
			ui.Row(c).Padding(0, 6).Children(func() {
				btnSelectAll := ui.Button(c.Key("btn-select-all"), "全选")
				if btnSelectAll.Clicked() {
					for _, s := range songs {
						a.selectedSongIDs[s.Source+"_"+s.ID] = true
					}
				}
			})
		} else {
			ui.Text(c, "#").FontSize(11).Bold().TextColor(pal.Faint).Width(28)
		}

		ui.Text(c, "歌曲 / 艺人").FontSize(11).Bold().TextColor(pal.Faint).Grow(1)
		ui.Text(c, "音源").FontSize(11).Bold().TextColor(pal.Faint).Width(75)
		ui.Text(c, "格式").FontSize(11).Bold().TextColor(pal.Faint).Width(54)
		ui.Row(c).Width(60).Justify(ui.End).Children(func() {
			ui.Text(c, "时长").FontSize(11).Bold().TextColor(pal.Faint)
		})
		ui.Row(c).Width(70).Justify(ui.End).Children(func() {
			ui.Text(c, "大小").FontSize(11).Bold().TextColor(pal.Faint)
		})
		ui.Row(c).Width(280).Justify(ui.End).Children(func() {
			ui.Text(c, "操作").FontSize(11).Bold().TextColor(pal.Faint)
		})
	})

	// 列表滚动区
	ui.Scroll(c).Grow(1).Gap(4).Children(func() {
		for i, item := range songs {
			song := item
			songKey := song.Source + "_" + song.ID
			isSelectedInBatch := a.selectedSongIDs[songKey]
			isCurrentPlaying := a.currentSong != nil && a.currentSong.ID == song.ID && a.currentSong.Source == song.Source && a.playerPlaying
			isFav := a.isSongInCustomPlaylist("fav_01", song.ID, song.Source)

			row := ui.Row(c.Key(fmt.Sprintf("song-%s-%s-%d", song.Source, song.ID, i))).
				Padding(6, 12).Radius(8).AlignItems(ui.Center)

			if isSelectedInBatch {
				row.Background(pal.CardMiniBg).Border(1, pal.Accent)
			} else if isCurrentPlaying {
				row.Background(pal.SongRowActiveBg).Border(1, pal.Accent)
			} else {
				row.Background(pal.SongRowBg).Border(1, pal.Border)
			}

			row.Children(func() {
				// 批量选择勾选框或序号
				if a.batchMode {
					checkText := "[  ]"
					if isSelectedInBatch {
						checkText = "[✓]"
					}
					btnCheck := ui.Button(c.Key("chk-"+songKey), checkText)
					if btnCheck.Clicked() {
						if a.selectedSongIDs[songKey] {
							delete(a.selectedSongIDs, songKey)
						} else {
							a.selectedSongIDs[songKey] = true
						}
					}
				} else {
					ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(11).TextColor(pal.Faint).Width(28)
				}

				// 唱片小图标 + 歌曲与艺人
				ui.Row(c).Grow(1).AlignItems(ui.Center).Gap(10).MinWidth(0).Children(func() {
					ui.Row(c).Size(28, 28).Radius(6).Background(pal.CardMiniBg).Center().Children(func() {
						ui.Icon(c, iconDisc).Size(14, 14).TextColor(t.Accent)
					})
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(c, song.Name).FontSize(13).Bold().TextColor(t.Text)
						ui.Text(c, "—").FontSize(12).TextColor(pal.Faint)
						artistStr := song.Artist
						if song.Album != "" {
							artistStr += " (" + song.Album + ")"
						}
						ui.Text(c, artistStr).FontSize(12).TextColor(t.TextMuted)
					})
				})

				// 音源微胶囊
				ui.Row(c).Width(75).Children(func() {
					ui.Row(c).Padding(2, 6).Radius(4).Background(pal.CardMiniBg).Border(1, pal.Border).Children(func() {
						ui.Text(c, sourceDisplayNames[song.Source]).FontSize(10).TextColor(t.TextMuted)
					})
				})

				// 格式
				ui.Row(c).Width(54).Children(func() {
					ext := "MP3"
					if song.Ext != "" {
						ext = strings.ToUpper(song.Ext)
					}
					ui.Row(c).Padding(1, 4).Radius(3).Background(pal.CardMiniBg).Children(func() {
						ui.Text(c, ext).FontSize(9).TextColor(pal.TextMuted)
					})
				})

				// 时长
				ui.Row(c).Width(60).Justify(ui.End).Children(func() {
					ui.Text(c, formatDuration(song.Duration)).FontSize(11).TextColor(t.TextMuted)
				})

				// 大小
				ui.Row(c).Width(70).Justify(ui.End).Children(func() {
					sizeStr := "--"
					if song.Size > 0 {
						sizeStr = core.FormatSize(song.Size)
					}
					ui.Text(c, sizeStr).FontSize(11).TextColor(t.TextMuted)
				})

				// 操作按钮组 (播放, 收藏/移出, 下载, 换源, 复制链接)
				ui.Row(c).Width(280).Justify(ui.End).Gap(6).Children(func() {
					playLabel := "播放"
					if isCurrentPlaying {
						playLabel = "暂停"
					}
					btnPlay := ui.Button(c.Key("play-"+song.ID), playLabel)
					if btnPlay.Clicked() {
						if isCurrentPlaying {
							a.pauseAudio()
						} else {
							a.playSongFromList(song, songs, i)
						}
					}

					// 收藏到自制歌单 或 从当前自制歌单移除
					if a.selectedCustomColID != "" {
						if ui.Button(c.Key("rem-"+song.ID), "移出").Clicked() {
							a.removeSongFromCustomPlaylist(a.selectedCustomColID, song.ID, song.Source)
							c.Toast("已从歌单移出: " + song.Name)
						}
					} else {
						favLabel := "收藏"
						if isFav {
							favLabel = "已收藏"
						}
						btnFav := ui.Button(c.Key("fav-"+song.ID), favLabel)
						if btnFav.Clicked() {
							if isFav {
								a.removeSongFromCustomPlaylist("fav_01", song.ID, song.Source)
								c.Toast("已取消收藏: " + song.Name)
							} else {
								a.addSongToCustomPlaylist("fav_01", song)
								c.Toast("已收藏到 [我喜欢的音乐]: " + song.Name)
							}
						}
					}

					btnDl := ui.Button(c.Key("dl-"+song.ID), "下载")
					if btnDl.Clicked() {
						a.downloadSong(song)
						qualityLabel := "无损"
						if a.downloadQuality == "exhigh" {
							qualityLabel = "320K"
						} else if a.downloadQuality == "standard" {
							qualityLabel = "标准"
						}
						c.Toast(fmt.Sprintf("已以 [%s] 音质加入下载: %s", qualityLabel, song.Name))
					}

					btnSwitch := ui.Button(c.Key("switch-"+song.ID), "换源")
					if btnSwitch.Clicked() {
						a.switchSongSource(song)
						c.Toast("正在匹配最佳音源…")
					}

					btnCopy := ui.Button(c.Key("copy-"+song.ID), "复制链接")
					if btnCopy.Clicked() {
						link := song.Link
						if link == "" {
							link = song.URL
						}
						c.WriteClipboard(link)
						c.Toast("链接已复制")
					}
				})
			})
		}
	})

	// 批量操作浮动工具条
	if a.batchMode && len(a.selectedSongIDs) > 0 {
		var selectedList []model.Song
		for _, s := range songs {
			if a.selectedSongIDs[s.Source+"_"+s.ID] {
				selectedList = append(selectedList, s)
			}
		}

		ui.Row(c).Height(46).Padding(0, 16).Radius(10).
			Background(pal.Surface).Border(1, pal.Accent).AlignItems(ui.Center).Gap(14).Children(func() {
			ui.Text(c, fmt.Sprintf("已勾选 %d 首歌曲", len(selectedList))).FontSize(13).Bold().TextColor(pal.Accent)

			ui.Spacer(c)

			if ui.PrimaryButton(c.Key("btn-batch-dl"), fmt.Sprintf("批量下载 (%d)", len(selectedList))).Clicked() {
				a.batchDownloadSongs(selectedList)
				c.Toast(fmt.Sprintf("已将选中的 %d 首歌曲加入下载队列", len(selectedList)))
				a.batchMode = false
				a.selectedSongIDs = make(map[string]bool)
			}

			if ui.Button(c.Key("btn-batch-fav"), fmt.Sprintf("批量收藏 (%d)", len(selectedList))).Clicked() {
				count := 0
				for _, s := range selectedList {
					if a.addSongToCustomPlaylist("fav_01", s) {
						count++
					}
				}
				c.Toast(fmt.Sprintf("已将 %d 首歌曲收藏到 [我喜欢的音乐]", count))
				a.batchMode = false
				a.selectedSongIDs = make(map[string]bool)
			}

			if ui.Button(c.Key("btn-exit-batch"), "退出批量").Clicked() {
				a.batchMode = false
				a.selectedSongIDs = make(map[string]bool)
			}
		})
	}
}

// renderLyricsPanel 悬浮歌词面板 (支持 LRC 时间戳实时同步逐行高亮)
func (a *NativeApp) renderLyricsPanel(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()
	if a.currentSong == nil {
		return
	}

	ui.Column(c.Key("lyrics-panel")).Padding(16).Radius(12).
		Background(pal.Surface).Border(1, pal.Border).Gap(10).Children(func() {

		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Icon(c, iconMusic).Size(14, 14).TextColor(pal.Accent)
				ui.Text(c, a.currentSong.Name+" — 实时歌词").FontSize(13).Bold().TextColor(t.Text)
			})
			ui.Spacer(c)
			if ui.Button(c.Key("btn-close-lrc"), "收起歌词").Clicked() {
				a.showLyric = false
			}
		})

		parsed := parseLRC(a.currentLyric)
		if len(parsed) == 0 {
			rawText := a.currentLyric
			if strings.TrimSpace(rawText) == "" {
				rawText = "暂无可用歌词"
			}
			ui.Scroll(c).Height(140).Children(func() {
				ui.Text(c, rawText).FontSize(12).TextColor(pal.TextMuted)
			})
			return
		}

		// 找到当前正在唱的那一行 (最后一条 TimeSec <= playPosSec)
		currentIdx := -1
		curSec := float64(a.playPosSec)
		for i, line := range parsed {
			if curSec >= line.TimeSec {
				currentIdx = i
			} else {
				break
			}
		}

		ui.Scroll(c).Height(140).Gap(6).Children(func() {
			for i, line := range parsed {
				l := line
				isCurrent := i == currentIdx
				row := ui.Row(c.Key(fmt.Sprintf("lrc-%d", i))).Padding(3, 8).Radius(6).AlignItems(ui.Center).Gap(8)
				if isCurrent {
					row.Background(pal.CardMiniBg).Border(1, pal.Accent.Alpha(0.4))
				}
				row.Children(func() {
					txt := ui.Text(c, l.Text).FontSize(12)
					if isCurrent {
						txt.Bold().TextColor(pal.Accent).FontSize(13)
					} else {
						txt.TextColor(pal.Faint)
					}
				})
			}
		})
	})
}

// downloadsPageView 下载管理页面
func (a *NativeApp) downloadsPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	// 顶部操作栏
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Icon(c, iconFolder).Size(16, 16).TextColor(t.Accent)
		ui.Text(c, "下载位置").FontSize(13).Bold()
		ui.Text(c, a.downloadDir).FontSize(12).TextColor(t.TextMuted).Grow(1)

		if ui.Button(c, "打开文件夹").Clicked() {
			_ = mygo.Shell.OpenPath(a.downloadDir)
		}

		if ui.Button(c, "更改…").Clicked() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent:    a.win,
				Title:     "选择下载位置",
				Directory: true,
			})
			if err == nil && len(paths) > 0 && paths[0] != "" {
				a.downloadDir = paths[0]
				a.saveDesktopSettings()
				c.Toast("下载位置已更新")
			}
		}

		if len(a.downloads) > 0 {
			if ui.Button(c, "清除全部记录").Clicked() {
				a.downloads = nil
				c.Toast("记录已清除")
			}
		}
	})

	if len(a.downloads) == 0 {
		ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
			ui.Column(c).Padding(28).Radius(14).Background(t.Surface).Border(1, t.Border).Gap(12).Width(480).Center().Children(func() {
				ui.Row(c).Size(52, 52).Radius(16).Background(pal.CardMiniBg).Border(1, pal.Border).Center().Children(func() {
					ui.Icon(c, iconDownload).Size(26, 26).TextColor(t.Accent)
				})
				ui.Text(c, "无下载内容").FontSize(16).Bold().TextColor(t.Text)
				ui.Text(c, "你下载的歌曲将显示在这里。").FontSize(12).TextColor(t.TextMuted)
			})
		})
		return
	}

	ui.Scroll(c).Grow(1).Gap(6).Children(func() {
		for i, item := range a.downloads {
			dl := item
			row := ui.Row(c.Key(fmt.Sprintf("dl-%d", i))).
				Padding(10, 14).Radius(8).Background(pal.SongRowBg).Border(1, pal.Border).AlignItems(ui.Center).Gap(12)

			row.Children(func() {
				ui.Row(c).Size(32, 32).Radius(6).Background(pal.CardMiniBg).Center().Children(func() {
					ui.Icon(c, iconMusic).Size(16, 16).TextColor(t.Accent)
				})

				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(c, dl.Song.Name).FontSize(13).Bold().TextColor(t.Text)
						ui.Row(c).Padding(1, 5).Radius(4).Background(pal.CardMiniBg).Children(func() {
							ui.Text(c, sourceDisplayNames[dl.Song.Source]).FontSize(10).TextColor(t.TextMuted)
						})
					})
					ui.Text(c, dl.Song.Artist).FontSize(11).TextColor(t.TextMuted)
					if dl.SavedPath != "" {
						ui.Text(c, "文件: "+filepath.Base(dl.SavedPath)).FontSize(10).TextColor(pal.Faint)
					}
					if dl.Error != "" {
						ui.Text(c, "错误: "+dl.Error).FontSize(10).TextColor(t.Danger)
					}
				})

				// Pulse 状态指示器
				switch dl.Status {
				case "已下载":
					ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
						ui.Row(c).Size(6, 6).Radius(3).Background(pal.Success)
						ui.Text(c, "已下载").FontSize(12).TextColor(pal.Success).Bold()
					})
				case "下载失败":
					ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
						ui.Row(c).Size(6, 6).Radius(3).Background(pal.Danger)
						ui.Text(c, "下载失败").FontSize(12).TextColor(pal.Danger).Bold()
					})
				default:
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c)
						ui.Text(c, dl.Status).FontSize(12).TextColor(t.TextMuted)
					})
				}

				ui.Text(c, dl.TimeStr).FontSize(11).TextColor(pal.Faint)

				// 定位文件
				if dl.SavedPath != "" {
					if ui.Button(c.Key("loc-"+fmt.Sprint(i)), "在文件夹中显示").Clicked() {
						mygo.Shell.ShowItemInFolder(dl.SavedPath)
					}
				}
			})
		}
	})
}

// settingsPageView 偏好设置页面 (精确复刻 Pulse system-dark.jpg 与 overview-light.jpg 风格)
func (a *NativeApp) settingsPageView(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	ui.Scroll(c).Grow(1).Gap(16).Children(func() {
		// Pulse Hero Card (复刻 system-dark.jpg 顶部的 MacBook Air 大卡片)
		ui.Row(c).Padding(20).Radius(14).Background(t.Surface).Border(1, t.Border).AlignItems(ui.Center).Gap(18).Children(func() {
			ui.Row(c).Size(52, 52).Radius(14).Background(pal.CardMiniBg).Border(1, pal.Border).Center().Children(func() {
				ui.Icon(c, iconMusic).Size(26, 26).TextColor(t.Accent)
			})
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				ui.Text(c, appTitle).FontSize(20).Bold().TextColor(t.Text)
				ui.Text(c, "v"+core.AppVersion+" · Pulse 设计风格原生客户端").FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Padding(4, 0, 0).Children(func() {
					pulseHeroBadge(c, pal, pal.Accent, "GPU 硬件加速")
					pulseHeroBadge(c, pal, pal.Success, "纯 Go 原生引擎")
				})
			})
		})

		// Pulse 分组卡片: 外观与主题 (明暗双模切换)
		pulseGroupCard(c, iconSun, ui.Hex("#ffc93d"), "外观与主题", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "显示模式").Width(90).FontSize(13).TextColor(t.TextMuted)

				themeModes := []struct {
					id, name string
					icon     *ui.SVG
				}{
					{ThemeModeDark, "深色模式 (Pulse Dark)", iconMoon},
					{ThemeModeLight, "浅色模式 (Pulse Light)", iconSun},
				}

				ui.Row(c).Padding(2).Radius(8).Background(pal.PillBg).Border(1, pal.PillBorder).Gap(2).Children(func() {
					for _, it := range themeModes {
						opt := it
						isSelected := a.themeMode == opt.id
						item := ui.Row(c.Key("theme-"+opt.id)).Padding(4, 12).Radius(6).AlignItems(ui.Center).Gap(6)
						if isSelected {
							item.Background(t.Accent).TextColor(t.AccentText)
						} else {
							item.Background(pal.PillBg)
						}

						if item.Clicked() {
							a.themeMode = opt.id
							a.saveDesktopSettings()
							c.Toast("已切换为" + opt.name)
						}

						item.Children(func() {
							ui.Icon(c, opt.icon).Size(12, 12)
							lbl := ui.Text(c, opt.name).FontSize(11)
							if isSelected {
								lbl.Bold()
							}
						})
					}
				})
			})
		})

		// Pulse 分组卡片: 界面字体 (支持预设精选字体与自定义本地字体实时切换)
		pulseGroupCard(c, iconSliders, ui.Hex("#30d158"), "界面字体", func() {
			ui.Column(c).Gap(12).Children(func() {
				// 1. 当前生效字体展示
				ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
					ui.Text(c, "当前字体").Width(90).FontSize(13).TextColor(t.TextMuted)
					currentFontLabel := a.fontFamily
					if currentFontLabel == "" {
						currentFontLabel = "系统默认"
					}
					ui.Text(c, currentFontLabel).FontSize(12).Bold().TextColor(t.Text)
				})

				// 2. 预设精选字体胶囊
				ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
					ui.Text(c, "精选字体").Width(90).FontSize(13).TextColor(t.TextMuted)

					fonts := []struct{ family, name string }{
						{"", "系统默认"},
						{"Microsoft YaHei", "微软雅黑"},
						{"Segoe UI", "Segoe UI"},
						{"PingFang SC", "苹方"},
						{"SF Pro Text", "SF Pro"},
						{"KaiTi", "楷体"},
						{"SimSun", "宋体"},
						{"Consolas", "等宽代码"},
					}

					ui.Row(c).Wrap().Gap(6).Children(func() {
						for _, it := range fonts {
							f := it
							isSelected := a.fontFamily == f.family
							item := ui.Row(c.Key("font-"+f.family)).Padding(4, 10).Radius(6).AlignItems(ui.Center)
							if isSelected {
								item.Background(t.Accent).TextColor(t.AccentText)
							} else {
								item.Background(pal.CardMiniBg).Border(1, pal.Border)
							}

							if item.Clicked() {
								a.fontFamily = f.family
								a.saveDesktopSettings()
								c.Toast("已切换界面字体: " + f.name)
							}

							item.Children(func() {
								lbl := ui.Text(c, f.name).FontSize(11)
								if f.family != "" {
									lbl.Font(f.family)
								}
								if isSelected {
									lbl.Bold()
								}
							})
						}
					})
				})

				// 3. 自定义本机字体输入
				ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
					ui.Text(c, "自定义字体").Width(90).FontSize(13).TextColor(t.TextMuted)
					ui.TextInput(c, &a.customFontInput).Width(240).Placeholder("输入本机已安装字体名称，如 Fira Code")
					if ui.Button(c, "应用字体").Clicked() {
						customName := strings.TrimSpace(a.customFontInput)
						if customName != "" {
							a.fontFamily = customName
							a.saveDesktopSettings()
							c.Toast("已应用自定义字体: " + customName)
						}
					}
				})

				// 4. 实时字形预览区
				ui.Column(c).Padding(12, 16).Radius(8).Background(pal.CardMiniBg).Border(1, pal.Border).Gap(4).Children(func() {
					ui.Text(c, "字形即时预览:").FontSize(11).Bold().TextColor(pal.Faint)
					previewText := ui.Text(c, "春日影 - 迷迹波 · Go Music DL 原生桌面端 0123456789 ABCabc").FontSize(13).TextColor(t.Text)
					if a.fontFamily != "" {
						previewText.Font(a.fontFamily)
					}
				})
			})
		})

		// Pulse 分组卡片: 窗口控制与平台交互 (支持根据平台自动适配或手动体验各平台规范)
		pulseGroupCard(c, iconSliders, ui.Hex("#40c8c0"), "窗口控制风格", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "当前运行系统").Width(90).FontSize(13).TextColor(t.TextMuted)
				platformLabel := fmt.Sprintf("%s (%s)", runtime.GOOS, runtime.GOARCH)
				ui.Text(c, platformLabel).FontSize(12).Bold().TextColor(t.Text)
			})

			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "控制按钮风格").Width(90).FontSize(13).TextColor(t.TextMuted)

				styles := []struct{ id, name string }{
					{WindowControlsAuto, "自动 (跟随系统)"},
					{WindowControlsMacOS, "macOS (交通灯)"},
					{WindowControlsWindows, "Windows (WinUI 3 标题栏)"},
					{WindowControlsLinuxGNOME, "Linux GNOME"},
					{WindowControlsLinuxKDE, "Linux KDE"},
				}

				ui.Row(c).Padding(2).Radius(8).Background(pal.PillBg).Border(1, pal.PillBorder).Gap(2).Children(func() {
					currentStyle := a.windowControlsStyle
					if currentStyle == "" {
						currentStyle = WindowControlsAuto
					}
					for _, it := range styles {
						opt := it
						isSelected := currentStyle == opt.id
						item := ui.Row(c.Key("style-"+opt.id)).Padding(4, 9).Radius(6).AlignItems(ui.Center)
						if isSelected {
							item.Background(t.Accent).TextColor(t.AccentText)
						} else {
							item.Background(pal.PillBg)
						}

						if item.Clicked() {
							a.windowControlsStyle = opt.id
							a.saveDesktopSettings()
							c.Toast("已切换窗口控制风格: " + opt.name)
						}

						item.Children(func() {
							lbl := ui.Text(c, opt.name).FontSize(11)
							if isSelected {
								lbl.Bold()
							}
						})
					}
				})
			})
		})

		// Pulse 分组卡片: 存储与下载设置
		pulseGroupCard(c, iconFolder, ui.Hex("#ff9500"), "下载与存储", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "下载位置").Width(90).FontSize(13).TextColor(t.TextMuted)
				ui.Text(c, a.downloadDir).FontSize(12).TextColor(t.Text).Grow(1)
				if ui.Button(c, "选取…").Clicked() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
						Parent:    a.win,
						Title:     "选择下载位置",
						Directory: true,
					})
					if err == nil && len(paths) > 0 && paths[0] != "" {
						a.downloadDir = paths[0]
						a.saveDesktopSettings()
						c.Toast("下载位置已更新")
					}
				}
			})

			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "文件命名格式").Width(90).FontSize(13).TextColor(t.TextMuted)
				if ti := ui.TextInput(c, &a.filenameTemplate).Width(280); ti.Changed() {
					a.saveDesktopSettings()
				}
				ui.Text(c, "变量：{artist} 歌手，{name} 歌名，{album} 专辑").FontSize(12).TextColor(pal.Faint)
			})

			// 下载音质偏好选择
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "下载音质").Width(90).FontSize(13).TextColor(t.TextMuted)

				qualities := []struct{ id, name string }{
					{"lossless", "无损优先 (FLAC)"},
					{"exhigh", "极高 (320Kbps)"},
					{"standard", "标准 (128Kbps)"},
				}

				ui.Row(c).Padding(2).Radius(8).Background(pal.PillBg).Border(1, pal.PillBorder).Gap(2).Children(func() {
					for _, it := range qualities {
						opt := it
						isSelected := a.downloadQuality == opt.id
						item := ui.Row(c.Key("quality-"+opt.id)).Padding(4, 10).Radius(6).AlignItems(ui.Center)
						if isSelected {
							item.Background(t.Accent).TextColor(t.AccentText)
						} else {
							item.Background(pal.PillBg)
						}
						if item.Clicked() {
							a.downloadQuality = opt.id
							a.saveDesktopSettings()
							c.Toast("已设置下载音质偏好: " + opt.name)
						}
						item.Children(func() {
							lbl := ui.Text(c, opt.name).FontSize(11)
							if isSelected {
								lbl.Bold()
							}
						})
					}
				})
			})
		})

		// Pulse 分组卡片: 账号与 Cookie 授权 (极简卡片化 + 一键扫码 + 剪贴板智能识别)
		pulseGroupCard(c, iconQrCode, ui.Hex("#ff375f"), "平台 Cookie 授权", func() {
			ui.Column(c).Gap(12).Children(func() {
				// 顶部智能导入提示与按钮
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "支持一键扫码自动捕获，或从剪贴板一键智能识别写入。").FontSize(12).TextColor(pal.Faint).Grow(1)

					if ui.PrimaryButton(c, "从剪贴板智能识别导入").Clicked() {
						clip := c.ReadClipboard()
						src, cookie, ok := detectAndApplyCookie(clip)
						if ok {
							switch src {
							case "netease":
								a.cookieNetease = cookie
							case "qq":
								a.cookieQQ = cookie
							case "kugou":
								a.cookieKugou = cookie
							case "bilibili":
								a.cookieBilibili = cookie
							}
							core.CM.SetAll(map[string]string{src: cookie})
							core.CM.Save()
							c.Toast("已智能识别并保存: " + sourceDisplayNames[src] + " Cookie！")
						} else if strings.TrimSpace(clip) != "" {
							c.Toast("未能识别剪贴板内容的音源平台，请点击下方对应平台的“从剪贴板填入”")
						} else {
							c.Toast("剪贴板为空，请先在浏览器中复制 Cookie 文本")
						}
					}
				})

				// 各平台授权卡片列表
				platforms := []struct {
					source string
					name   string
					cookie *string
				}{
					{"netease", "网易云音乐", &a.cookieNetease},
					{"qq", "QQ 音乐", &a.cookieQQ},
					{"kugou", "酷狗音乐", &a.cookieKugou},
					{"bilibili", "哔哩哔哩", &a.cookieBilibili},
				}

				for _, item := range platforms {
					p := item
					hasCookie := strings.TrimSpace(*p.cookie) != ""
					isPolling := a.qrLoginPolling[p.source]
					isExpanded := a.expandedCookieEdit[p.source]
					auth := a.authStatus[p.source]

					ui.Column(c.Key("cookie-card-"+p.source)).Padding(10, 14).Radius(8).
						Background(pal.CardMiniBg).Border(1, pal.Border).Gap(6).Children(func() {

						// 平台信息与状态行
						ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
							// 平台名称
							ui.Text(c, p.name).Width(80).FontSize(13).Bold().TextColor(t.Text)

							// 授权与登录状态徽章
							if auth.Checking {
								ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
									ui.Spinner(c)
									ui.Text(c, "验证中…").FontSize(11).TextColor(pal.Accent)
								})
							} else if auth.Checked && auth.Valid {
								ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
									ui.Row(c).Size(6, 6).Radius(3).Background(pal.Success)
									ui.Text(c, "已登录: "+auth.Username).FontSize(11).TextColor(pal.Success).Bold()
									if auth.IsVIP {
										ui.Row(c).Padding(1, 5).Radius(3).Background(ui.Hex("#ffc93d26")).Children(func() {
											ui.Text(c, "VIP").FontSize(9).TextColor(pal.Warning).Bold()
										})
									}
									if auth.PlaylistCount > 0 {
										ui.Text(c, fmt.Sprintf("(%d个歌单)", auth.PlaylistCount)).FontSize(10).TextColor(pal.Faint)
									}
								})
							} else if auth.Checked && !auth.Valid && hasCookie {
								ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
									ui.Row(c).Size(6, 6).Radius(3).Background(pal.Danger)
									ui.Text(c, "授权失效/已过期").FontSize(11).TextColor(pal.Danger).Bold()
								})
							} else if hasCookie {
								ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
									ui.Row(c).Size(6, 6).Radius(3).Background(pal.Success)
									ui.Text(c, fmt.Sprintf("已授权 (%d 字符)", len(*p.cookie))).FontSize(11).TextColor(pal.Success).Bold()
								})
							} else {
								ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
									ui.Row(c).Size(6, 6).Radius(3).Background(pal.Faint)
									ui.Text(c, "未授权").FontSize(11).TextColor(pal.Faint)
								})
							}

							ui.Spacer(c)

							// 动作按钮组
							// 1. 扫码登录
							if isPolling {
								ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
									ui.Spinner(c)
									ui.Text(c, "等待扫码…").FontSize(11).TextColor(pal.Accent)
								})
							} else {
								btnQR := ui.Button(c.Key("qr-"+p.source), "扫码授权")
								if btnQR.Clicked() {
									a.startQRLogin(p.source)
								}
							}

							// 2. 状态验证按钮 (检测登录是否依然有效)
							if hasCookie && !isPolling {
								btnCheck := ui.Button(c.Key("check-"+p.source), "验证状态")
								if btnCheck.Clicked() {
									a.verifyPlatformAuth(p.source, true)
								}
							}

							// 3. 从剪贴板填入
							btnPaste := ui.Button(c.Key("paste-"+p.source), "从剪贴板填入")
							if btnPaste.Clicked() {
								clip := strings.TrimSpace(c.ReadClipboard())
								if clip != "" {
									*p.cookie = clip
									core.CM.SetAll(map[string]string{p.source: clip})
									core.CM.Save()
									go a.verifyPlatformAuth(p.source, true)
									c.Toast("已填入并保存 " + p.name + " Cookie！")
								} else {
									c.Toast("剪贴板为空")
								}
							}

							// 4. 手动编辑折叠开关
							editLabel := "手动编辑"
							if isExpanded {
								editLabel = "收起"
							}
							btnToggle := ui.Button(c.Key("edit-"+p.source), editLabel)
							if btnToggle.Clicked() {
								a.expandedCookieEdit[p.source] = !a.expandedCookieEdit[p.source]
							}

							// 5. 清除凭据
							if hasCookie {
								btnClear := ui.Button(c.Key("clear-"+p.source), "清除")
								if btnClear.Clicked() {
									*p.cookie = ""
									core.CM.SetAll(map[string]string{p.source: ""})
									core.CM.Save()
									delete(a.authStatus, p.source)
									c.Toast("已清除 " + p.name + " 授权凭据")
								}
							}
						})

						// 手动编辑展开框 (按需展示)
						if isExpanded {
							ui.Row(c).Padding(4, 0, 0).AlignItems(ui.Center).Gap(8).Children(func() {
								ui.TextInput(c, p.cookie).Grow(1).Placeholder("输入或粘贴该平台完整 Cookie")
								if ui.PrimaryButton(c.Key("save-"+p.source), "保存").Clicked() {
									core.CM.SetAll(map[string]string{p.source: *p.cookie})
									core.CM.Save()
									go a.verifyPlatformAuth(p.source, true)
									c.Toast("已保存 " + p.name + " Cookie")
									a.expandedCookieEdit[p.source] = false
								}
							})
						}
					})
				}
			})
		})

		// Pulse 分组卡片: WebDAV 云盘同步
		pulseGroupCard(c, iconFolder, ui.Hex("#40c8c0"), "WebDAV 云端同步", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Switch(c, &a.webdavEnabled)
				ui.Text(c, "下载完成后自动同步上传至 WebDAV 网盘").FontSize(13).TextColor(t.Text)
			})
			if a.webdavEnabled {
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "服务地址").Width(90).FontSize(13).TextColor(t.TextMuted)
					ui.TextInput(c, &a.webdavURL).Width(360).Placeholder("http://127.0.0.1:18081/dav")
				})
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "账号").Width(90).FontSize(13).TextColor(t.TextMuted)
					ui.TextInput(c, &a.webdavUsername).Width(180)
					ui.Text(c, "密码").Width(50).FontSize(13).TextColor(t.TextMuted)
					ui.TextInput(c, &a.webdavPassword).Width(180)
				})
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "远端目录").Width(90).FontSize(13).TextColor(t.TextMuted)
					ui.TextInput(c, &a.webdavDir).Width(180).Placeholder("/music")
				})
				ui.Row(c).Padding(4, 0, 0, 100).Children(func() {
					if ui.Button(c, "保存 WebDAV 设置").Clicked() {
						settings := core.GetWebSettings()
						settings.WebDAVEnabled = a.webdavEnabled
						settings.WebDAVURL = a.webdavURL
						settings.WebDAVUsername = a.webdavUsername
						settings.WebDAVPassword = a.webdavPassword
						settings.WebDAVDir = a.webdavDir
						if err := core.SaveWebSettings(settings); err != nil {
							c.Toast("保存 WebDAV 失败: " + err.Error())
						} else {
							c.Toast("已保存 WebDAV 配置")
						}
					}
				})
			}
		})

		// Pulse 分组卡片: 通知
		pulseGroupCard(c, iconBell, ui.Hex("#ffc93d"), "通知", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				if sw := ui.Switch(c, &a.notifEnabled); sw.Changed() {
					a.saveDesktopSettings()
				}
				ui.Text(c, "下载完成时发送通知").FontSize(13).TextColor(t.Text)
			})
		})

		// Pulse 分组卡片: 关于与系统信息
		pulseGroupCard(c, iconInfo, ui.Hex("#0a84ff"), "关于", func() {
			items := []struct{ k, v string }{
				{"应用版本", "v" + core.AppVersion},
				{"视觉规范", "Pulse (https://pulse.egoist.dev)"},
				{"渲染引擎", "MyGo UI 原生渲染引擎 (GPU 加速)"},
				{"音频核心", "MyGo 原生音频解码核心"},
				{"矢量图标", "Lucide Icons"},
				{"开源主页", githubRepoURL},
			}

			for _, it := range items {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, it.k).Width(110).FontSize(13).TextColor(t.TextMuted)
					ui.Spacer(c)
					ui.Text(c, it.v).FontSize(13).Bold().TextColor(t.Text)
				})
			}

			ui.Row(c).Padding(10, 0, 0).Children(func() {
				if ui.PrimaryButton(c, "访问 GitHub 源码仓库").Clicked() {
					_ = mygo.Shell.OpenExternal(githubRepoURL)
				}
			})
		})
	})
}

// pulsePlayerDock Pulse 悬浮播放控制坞与专业试听体验 (支持进度条拖拽Seek、上一首/下一首切歌与自动连播)
func (a *NativeApp) pulsePlayerDock(c *ui.Context) {
	t := c.Theme()
	pal := a.colors()

	ui.Row(c).Height(78).Padding(0, 20).Background(pal.DockBg).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.SidebarBorder).AlignItems(ui.Center).Children(func() {

		// 左侧：当前曲目唱片封面与信息 (250px)
		if a.currentSong != nil {
			ui.Row(c).Width(250).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Row(c).Size(42, 42).Radius(8).Background(pal.CardMiniBg).Center().Children(func() {
					ui.Icon(c, iconDisc).Size(22, 22).TextColor(t.Accent)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(c, a.currentSong.Name).FontSize(13).Bold().TextColor(t.Text)
						ui.Row(c).Padding(1, 4).Radius(3).Background(pal.CardMiniBg).Children(func() {
							ui.Text(c, sourceDisplayNames[a.currentSong.Source]).FontSize(9).TextColor(t.TextMuted)
						})
					})
					ui.Text(c, a.currentSong.Artist).FontSize(11).TextColor(t.TextMuted)
				})
			})
		} else {
			ui.Row(c).Width(250).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Row(c).Size(42, 42).Radius(8).Background(pal.CardMiniBg).Center().Children(func() {
					ui.Icon(c, iconMusic).Size(20, 20).TextColor(pal.Faint)
				})
				ui.Text(c, "未在播放").FontSize(12).TextColor(t.TextMuted)
			})
		}

		ui.Spacer(c)

		// 中间核心控制区：按键排 + 播放进度条 (弹性自适应宽度，居中)
		ui.Column(c).Grow(1).MaxWidth(520).AlignItems(ui.Center).Gap(6).Children(func() {
			// 第一行：播放控制按钮排
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				// 单曲/列表循环切换
				loopTitle := "列表"
				if a.singleLoop {
					loopTitle = "单曲"
				}
				btnLoop := ui.Button(c.Key("btn-loop-mode"), loopTitle)
				if btnLoop.Clicked() {
					a.singleLoop = !a.singleLoop
					if a.singleLoop {
						c.Toast("已开启单曲循环")
					} else {
						c.Toast("已开启列表连续播放")
					}
				}

				// 上一首
				btnPrev := ui.Button(c.Key("btn-skip-prev"), "上一首")
				if btnPrev.Clicked() {
					a.playPrevSong()
				}

				// 播放 / 暂停 主控制
				playLabel := "播放"
				if a.playerPlaying {
					playLabel = "暂停"
				}
				btnPlay := ui.PrimaryButton(c.Key("btn-dock-play"), playLabel).Disabled(a.currentSong == nil && !a.loadingAudio)
				if btnPlay.Clicked() {
					if a.playerPlaying {
						a.pauseAudio()
					} else {
						a.resumeAudio()
					}
				}

				// 下一首
				btnNext := ui.Button(c.Key("btn-skip-next"), "下一首")
				if btnNext.Clicked() {
					a.playNextSong()
				}

				// 停止按钮
				if ui.Button(c.Key("btn-dock-stop"), "停止").Clicked() {
					a.stopAudio()
				}

				// 加载与报错提示
				if a.loadingAudio {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c)
						ui.Text(c, "载入中…").FontSize(11).TextColor(pal.Accent)
					})
				} else if a.audioError != "" {
					ui.Text(c, a.audioError).FontSize(11).TextColor(t.Danger)
				}
			})

			// 第二行：播放进度条与时间显示 (可拖拽 Seek)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
				// 当前播放时间
				curTimeStr := fmt.Sprintf("%02d:%02d", a.playPosSec/60, a.playPosSec%60)
				ui.Text(c, curTimeStr).FontSize(11).TextColor(pal.Faint).Width(34)

				// 进度条滑动器 (支持拖动快进/选段试听)
				progressSlider := ui.Slider(c.Key("seek-slider"), &a.playProgress, 0, 1).Grow(1)
				if progressSlider.Changed() && a.player != nil && a.playDurSec > 0 {
					targetSec := int(a.playProgress * float64(a.playDurSec))
					a.playPosSec = targetSec
					a.player.Seek(time.Duration(targetSec) * time.Second)
				}

				// 曲目总时长
				totalTimeStr := formatDuration(a.playDurSec)
				ui.Text(c, totalTimeStr).FontSize(11).TextColor(pal.Faint).Width(34)
			})
		})

		ui.Spacer(c)

		// 右侧：歌词展开、音量调节滑块与 Pulse 实时指示器 (280px)
		ui.Row(c).Width(280).Justify(ui.End).AlignItems(ui.Center).Gap(14).Children(func() {
			// 歌词展开按钮
			if a.currentSong != nil {
				lrcLabel := "歌词"
				if a.showLyric {
					lrcLabel = "收起歌词"
				}
				btnLrc := ui.Button(c.Key("btn-lrc"), lrcLabel)
				if btnLrc.Clicked() {
					a.showLyric = !a.showLyric
				}
			}

			// 音量调节滑块
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				volIcon := iconVolume
				if a.volume <= 0.05 {
					volIcon = iconVolumeMute
				}
				ui.Icon(c, volIcon).Size(15, 15).TextColor(t.TextMuted)

				slider := ui.Slider(c.Key("vol-slider"), &a.volume, 0, 1).Width(70)
				if slider.Changed() {
					if a.player != nil {
						a.player.SetVolume(a.volume)
					}
					a.saveDesktopSettings()
				}
				ui.Text(c, fmt.Sprintf("%d%%", int(a.volume*100))).FontSize(11).TextColor(t.TextMuted).Width(30)
			})

			// Pulse Live LED Status Indicators
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Row(c).Size(6, 6).Radius(3).Background(pal.Success)
					ui.Text(c, "60fps").FontSize(10).TextColor(pal.Faint)
				})
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Row(c).Size(6, 6).Radius(3).Background(pal.Accent)
					ui.Text(c, "原生").FontSize(10).TextColor(pal.Faint)
				})
			})
		})
	})
}

func (a *NativeApp) startSearch() {
	kw := strings.TrimSpace(a.searchKeyword)
	if kw == "" || a.searching {
		return
	}

	a.searching = true
	a.searchError = ""
	a.searchResults = nil
	a.playlistResults = nil
	a.selectedPlaylist = nil
	a.playlistSongs = nil
	a.hasLoadedMore = false

	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
		// 1. 链接直达解析 (支持粘贴网易云/QQ/酷狗等歌曲/歌单/专辑网页分享链接)
		if strings.HasPrefix(kw, "http://") || strings.HasPrefix(kw, "https://") {
			source := core.DetectSource(kw)

			// 尝试解析歌单链接
			if parsePL := core.GetParsePlaylistFunc(source); parsePL != nil {
				pl, songs, err := parsePL(kw)
				if err == nil && pl != nil && len(songs) > 0 {
					if a.win != nil {
						a.win.Update(func() {
							a.searching = false
							a.selectedPlaylist = pl
							a.playlistSongs = songs
						})
					}
					return
				}
			}

			// 尝试解析专辑链接
			if parseAlbum := core.GetParseAlbumFunc(source); parseAlbum != nil {
				pl, songs, err := parseAlbum(kw)
				if err == nil && pl != nil && len(songs) > 0 {
					if a.win != nil {
						a.win.Update(func() {
							a.searching = false
							a.selectedPlaylist = pl
							a.playlistSongs = songs
						})
					}
					return
				}
			}

			// 尝试解析单曲链接
			if parseSong := core.GetParseFunc(source); parseSong != nil {
				song, err := parseSong(kw)
				if err == nil && song != nil {
					if a.win != nil {
						a.win.Update(func() {
							a.searching = false
							a.searchResults = []model.Song{*song}
						})
					}
					return
				}
			}
		}

		// 2. 分类搜索
		switch a.searchType {
		case "playlist":
			src := a.selectedSource
			if src == "all" {
				src = "netease"
			}
			plFn := core.GetPlaylistSearchFunc(src)
			if plFn == nil {
				if a.win != nil {
					a.win.Update(func() {
						a.searching = false
						a.searchError = "当前音源不支持歌单搜索"
					})
				}
				return
			}
			pls, err := plFn(kw)
			if a.win != nil {
				a.win.Update(func() {
					a.searching = false
					if err != nil {
						a.searchError = err.Error()
					} else {
						a.playlistResults = pls
					}
				})
			}

		case "album":
			src := a.selectedSource
			if src == "all" {
				src = "netease"
			}
			alFn := core.GetAlbumSearchFunc(src)
			if alFn == nil {
				if a.win != nil {
					a.win.Update(func() {
						a.searching = false
						a.searchError = "当前音源不支持专辑搜索"
					})
				}
				return
			}
			als, err := alFn(kw)
			if a.win != nil {
				a.win.Update(func() {
					a.searching = false
					if err != nil {
						a.searchError = err.Error()
					} else {
						a.playlistResults = als
					}
				})
			}

		default:
			// 单曲搜索
			results, err := a.performSearch(kw, a.selectedSource)
			if a.win != nil {
				a.win.Update(func() {
					a.searching = false
					if err != nil {
						a.searchError = err.Error()
					} else {
						a.searchResults = results
					}
				})
			}
		}
	}()
}

func (a *NativeApp) openPlaylistDetail(pl model.Playlist, isAlbum bool) {
	a.selectedPlaylist = &pl
	a.loadingPlaylistDetail = true
	a.playlistDetailError = ""
	a.playlistSongs = nil

	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
		var songs []model.Song
		var err error
		if isAlbum {
			if fn := core.GetAlbumDetailFunc(pl.Source); fn != nil {
				songs, err = fn(pl.ID)
			} else {
				err = fmt.Errorf("该音源暂不支持专辑详情解析")
			}
		} else {
			if fn := core.GetPlaylistDetailFunc(pl.Source); fn != nil {
				songs, err = fn(pl.ID)
			} else {
				err = fmt.Errorf("该音源暂不支持歌单详情解析")
			}
		}

		if a.win != nil {
			a.win.Update(func() {
				a.loadingPlaylistDetail = false
				if err != nil {
					a.playlistDetailError = err.Error()
				} else {
					a.playlistSongs = songs
				}
			})
		}
	}()
}

func (a *NativeApp) loadRecommendPlaylists() {
	a.loadingRecommend = true
	a.recommendError = ""
	a.recommendPlaylists = nil

	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
		src := a.selectedRecommendSource
		if src == "" {
			src = "netease"
		}
		fn := core.GetRecommendFunc(src)
		var pls []model.Playlist
		var err error
		if fn != nil {
			pls, err = fn()
		} else {
			err = fmt.Errorf("该音源暂不支持推荐歌单")
		}

		if a.win != nil {
			a.win.Update(func() {
				a.loadingRecommend = false
				if err != nil {
					a.recommendError = err.Error()
				} else {
					a.recommendPlaylists = pls
				}
			})
		}
	}()
}

func (a *NativeApp) refreshLocalMusic() {
	a.loadingLocalMusic = true
	go func() {
		entries, err := os.ReadDir(a.downloadDir)
		var files []LocalMusicItem
		if err == nil {
			audioExts := map[string]bool{
				".mp3": true, ".flac": true, ".m4a": true,
				".ogg": true, ".wav": true, ".aac": true,
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if audioExts[ext] {
					info, err := entry.Info()
					if err == nil {
						files = append(files, LocalMusicItem{
							Filename: entry.Name(),
							Path:     filepath.Join(a.downloadDir, entry.Name()),
							Size:     info.Size(),
							Ext:      strings.TrimPrefix(ext, "."),
							ModTime:  info.ModTime().Format("2006-01-02 15:04"),
						})
					}
				}
			}
			sort.Slice(files, func(i, j int) bool {
				return files[i].ModTime > files[j].ModTime
			})
		}

		if a.win != nil {
			a.win.Update(func() {
				a.localMusicFiles = files
				a.loadingLocalMusic = false
			})
		} else {
			a.localMusicFiles = files
			a.loadingLocalMusic = false
		}
	}()
}

func (a *NativeApp) playLocalFile(item LocalMusicItem) {
	ext := strings.ToLower(item.Ext)
	if ext != "mp3" && ext != "wav" {
		a.mu.Lock()
		a.currentSong = &model.Song{
			ID:     item.Path,
			Name:   strings.TrimSuffix(item.Filename, filepath.Ext(item.Filename)),
			Artist: "本地音乐",
			Source: "local",
			Ext:    item.Ext,
			Size:   item.Size,
		}
		a.audioError = fmt.Sprintf("暂不支持播放 .%s 格式（原生播放器当前内置纯 Go MP3 与 WAV 解码）", item.Ext)
		a.loadingAudio = false
		a.playerPlaying = false
		a.mu.Unlock()
		if a.win != nil {
			a.win.Update(func() {
				a.win.Invalidate()
			})
		}
		return
	}

	a.mu.Lock()
	a.playGen++
	gen := a.playGen
	if a.player != nil {
		a.player.Stop()
		a.player.Close()
		a.player = nil
	}
	a.loadingAudio = true
	a.playerPlaying = false
	a.audioError = ""
	a.currentSong = &model.Song{
		ID:     item.Path,
		Name:   strings.TrimSuffix(item.Filename, filepath.Ext(item.Filename)),
		Artist: "本地音乐",
		Source: "local",
		Ext:    item.Ext,
		Size:   item.Size,
	}
	a.currentLyric = ""
	a.playProgress = 0
	a.playPosSec = 0
	a.playDurSec = 0
	a.mu.Unlock()

	if a.win != nil {
		a.win.Update(func() {
			a.win.Invalidate()
		})
	}

	go func(gen uint64) {
		data, err := os.ReadFile(item.Path)
		if err != nil {
			if a.win != nil {
				a.win.Update(func() {
					a.mu.Lock()
					defer a.mu.Unlock()
					if a.playGen == gen {
						a.loadingAudio = false
						a.audioError = "读取本地音频失败: " + err.Error()
					}
				})
			}
			return
		}
		p, err := audio.OpenStream(bytes.NewReader(data))
		if a.win != nil {
			a.win.Update(func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				if a.playGen != gen {
					if p != nil {
						p.Close()
					}
					return
				}
				a.loadingAudio = false
				if err != nil {
					a.audioError = "本地音频解码失败: " + err.Error()
					return
				}
				if a.player != nil {
					a.player.Stop()
					a.player.Close()
				}
				a.player = p
				a.player.SetVolume(a.volume)
				a.player.Play()
				a.playerPlaying = true
			})
		} else if p != nil {
			p.Close()
		}
	}(gen)
}

func (a *NativeApp) switchSongSource(target model.Song) {
	go func() {
		sources := []string{"netease", "qq", "kugou", "kuwo", "migu"}
		var bestMatch *model.Song
		var bestScore float64
		for _, s := range sources {
			if s == target.Source {
				continue
			}
			searchFn := core.GetSearchFunc(s)
			if searchFn == nil {
				continue
			}
			cands, err := searchFn(target.Name)
			if err == nil {
				for _, cand := range cands {
					score := core.CalcSongSimilarity(target.Name, target.Artist, cand.Name, cand.Artist)
					if score > 0.75 && score > bestScore {
						bestScore = score
						item := cand
						bestMatch = &item
					}
				}
			}
		}
		if bestMatch != nil {
			if a.win != nil {
				a.win.Update(func() {
					a.playSong(*bestMatch)
				})
			}
		}
	}()
}

func (a *NativeApp) batchDownloadSongs(songs []model.Song) {
	for _, song := range songs {
		a.downloadSong(song)
	}
}

func (a *NativeApp) performSearch(keyword, source string) ([]model.Song, error) {
	if source == "all" {
		sources := []string{"netease", "qq", "kugou", "kuwo", "migu", "bilibili", "soda"}
		var wg sync.WaitGroup
		var mu sync.Mutex
		var allSongs []model.Song
		var lastErr error

		for _, src := range sources {
			searchFn := core.GetSearchFunc(src)
			if searchFn == nil {
				continue
			}

			wg.Add(1)
			go func(s string, fn core.SearchFunc) {
				defer wg.Done()
				songs, err := fn(keyword)
				if err != nil {
					mu.Lock()
					lastErr = err
					mu.Unlock()
					return
				}
				mu.Lock()
				allSongs = append(allSongs, songs...)
				mu.Unlock()
			}(src, searchFn)
		}

		wg.Wait()
		if len(allSongs) == 0 && lastErr != nil {
			return nil, lastErr
		}
		return allSongs, nil
	}

	searchFn := core.GetSearchFunc(source)
	if searchFn == nil {
		return nil, fmt.Errorf("不支持的搜索源: %s", source)
	}
	return searchFn(keyword)
}

func (a *NativeApp) playSong(song model.Song) {
	a.mu.Lock()
	a.playGen++
	gen := a.playGen
	if a.player != nil {
		a.player.Stop()
		a.player.Close()
		a.player = nil
	}
	a.currentSong = &song
	a.currentLyric = ""
	a.loadingAudio = true
	a.playerPlaying = false
	a.audioError = ""
	a.playProgress = 0
	a.playPosSec = 0
	a.playDurSec = song.Duration
	a.mu.Unlock()

	if a.win != nil {
		a.win.Invalidate()
	}

	// 异步拉取歌词 (带世代防串歌校验)
	go func(gen uint64, s model.Song) {
		lyricFn := core.GetLyricFunc(s.Source)
		if lyricFn != nil {
			lrc, err := lyricFn(&s)
			if err == nil && lrc != "" {
				if a.win != nil {
					a.win.Update(func() {
						a.mu.Lock()
						if a.playGen == gen {
							a.currentLyric = lrc
						}
						a.mu.Unlock()
					})
				}
			}
		}
	}(gen, song)

	// 异步拉取音频流 (带世代防竞态与句柄泄漏防护)
	go func(gen uint64, s model.Song) {
		dlData, err := core.DownloadSongData(&s, false, false)
		if err != nil {
			if a.win != nil {
				a.win.Update(func() {
					a.mu.Lock()
					defer a.mu.Unlock()
					if a.playGen == gen {
						a.loadingAudio = false
						a.audioError = "获取音频失败: " + err.Error()
					}
				})
			}
			return
		}

		p, err := audio.OpenStream(bytes.NewReader(dlData.Data))
		if a.win != nil {
			a.win.Update(func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				if a.playGen != gen {
					if p != nil {
						p.Close()
					}
					return
				}
				a.loadingAudio = false
				if err != nil {
					a.audioError = "音频解码失败: " + err.Error()
					log.Printf("音频解码异常: %v", err)
				} else {
					if a.player != nil {
						a.player.Stop()
						a.player.Close()
					}
					a.player = p
					a.player.SetVolume(a.volume)
					a.player.Play()
					a.playerPlaying = true
				}
			})
		} else if p != nil {
			p.Close()
		}
	}(gen, song)
}

func (a *NativeApp) pauseAudio() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.player != nil && a.player.Playing() {
		a.player.Pause()
		a.playerPlaying = false
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *NativeApp) resumeAudio() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.player != nil {
		a.player.Play()
		a.playerPlaying = true
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *NativeApp) stopAudio() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.playGen++
	if a.player != nil {
		a.player.Stop()
		a.player.Close()
		a.player = nil
	}
	a.playerPlaying = false
	a.loadingAudio = false
	a.currentSong = nil
	a.currentLyric = ""
	a.playProgress = 0
	a.playPosSec = 0
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *NativeApp) downloadSong(song model.Song) {
	if song.Extra == nil {
		song.Extra = make(map[string]string)
	}
	q := a.downloadQuality
	if q == "" {
		q = "lossless"
	}
	song.Extra["level"] = q
	song.Extra["netease_level"] = q
	song.Extra["quality"] = q
	switch q {
	case "lossless":
		song.Extra["br"] = "999"
		song.Extra["format"] = "flac"
	case "exhigh":
		song.Extra["br"] = "320"
		song.Extra["format"] = "mp3"
	case "standard":
		song.Extra["br"] = "128"
		song.Extra["format"] = "mp3"
	}

	item := DownloadItem{
		Song:    song,
		Status:  "正在下载…",
		TimeStr: time.Now().Format("15:04:05"),
	}

	a.mu.Lock()
	a.downloads = append([]DownloadItem{item}, a.downloads...)
	a.mu.Unlock()

	if a.win != nil {
		a.win.Update(func() {
			a.win.Invalidate()
		})
	}

	go func() {
		saved, err := core.SaveSongToFileWithTemplate(&song, a.downloadDir, true, true, a.filenameTemplate)

		if a.win != nil {
			a.win.Update(func() {
				for i := range a.downloads {
					if a.downloads[i].Song.ID == song.ID && a.downloads[i].Song.Source == song.Source {
						if err != nil {
							a.downloads[i].Status = "下载失败"
							a.downloads[i].Error = err.Error()
						} else {
							a.downloads[i].Status = "已下载"
							if saved != nil {
								a.downloads[i].SavedPath = saved.SavedPath
							}
						}
						break
					}
				}
			})
		}

		// WebDAV 自动同步上传
		if err == nil && a.webdavEnabled && a.webdavURL != "" && saved != nil && saved.SavedPath != "" {
			settings := core.WebSettings{
				WebDAVEnabled:  a.webdavEnabled,
				WebDAVURL:      a.webdavURL,
				WebDAVUsername: a.webdavUsername,
				WebDAVPassword: a.webdavPassword,
				WebDAVDir:      a.webdavDir,
			}
			if core.WebDAVConfigured(settings) {
				data, readErr := os.ReadFile(saved.SavedPath)
				if readErr == nil {
					if upErr := core.UploadSongToWebDAV(settings, filepath.Base(saved.SavedPath), data); upErr != nil {
						log.Printf("[WebDAV] failed to upload %s: %v", filepath.Base(saved.SavedPath), upErr)
					}
				}
			}
		}

		if err == nil && a.notifEnabled && mygo.NotificationsSupported() {
			n := mygo.NewNotification(mygo.NotificationOptions{
				Title: "音乐已下载",
				Body:  fmt.Sprintf("%s - %s", song.Artist, song.Name),
			})
			if saved != nil && saved.SavedPath != "" {
				n.OnClick(func() { mygo.Shell.ShowItemInFolder(saved.SavedPath) })
			}
			_ = n.Show()
		}
	}()
}

func pulseGroupCard(c *ui.Context, icon *ui.SVG, iconColor ui.Color, title string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(18).Gap(14).Radius(14).Background(t.Surface).Border(1, t.Border).Children(func() {
		if title != "" {
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				if icon != nil {
					ui.Row(c).Size(24, 24).Radius(6).Background(iconColor.Alpha(0.12)).Center().Children(func() {
						ui.Icon(c, icon).Size(14, 14).TextColor(iconColor)
					})
				}
				ui.Text(c, title).FontSize(14).Bold().TextColor(iconColor)
			})
		}
		body()
	})
}

func pulseHeroBadge(c *ui.Context, pal PulseColors, ledColor ui.Color, label string) {
	ui.Row(c).Padding(3, 8).Radius(6).Background(pal.CardMiniBg).Border(1, pal.Border).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Row(c).Size(6, 6).Radius(3).Background(ledColor)
		ui.Text(c, label).FontSize(11).TextColor(pal.Text)
	})
}

func pulseMiniFeature(c *ui.Context, pal PulseColors, ledColor ui.Color, title, desc string) {
	ui.Column(c).Padding(10, 14).Radius(8).Background(pal.CardMiniBg).Border(1, pal.Border).Gap(4).Width(170).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Row(c).Size(6, 6).Radius(3).Background(ledColor)
			ui.Text(c, title).FontSize(11).Bold().TextColor(pal.Text)
		})
		ui.Text(c, desc).FontSize(10).TextColor(pal.TextMuted)
	})
}

func formatDuration(sec int) string {
	if sec <= 0 {
		return "--:--"
	}
	m := sec / 60
	s := sec % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

func detectAndApplyCookie(raw string) (platform string, cookie string, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", false
	}
	// 网易云音乐: MUSIC_U 或 __csrf
	if strings.Contains(s, "MUSIC_U") || strings.Contains(s, "__csrf") {
		return "netease", s, true
	}
	// QQ 音乐: qm_keyst, uin, p_skey, pskey
	if strings.Contains(s, "qm_keyst") || strings.Contains(s, "p_skey") || strings.Contains(s, "pskey") {
		return "qq", s, true
	}
	// 酷狗音乐: kg_mid, kg_dfid, KuGou, token
	if strings.Contains(s, "kg_mid") || strings.Contains(s, "kg_dfid") || strings.Contains(s, "KuGou") {
		return "kugou", s, true
	}
	// 哔哩哔哩: SESSDATA, bili_jct, DedeUserID
	if strings.Contains(s, "SESSDATA") || strings.Contains(s, "bili_jct") || strings.Contains(s, "DedeUserID") {
		return "bilibili", s, true
	}
	return "", "", false
}

func qrLoginCookieString(result *model.QRLoginResult) string {
	if result == nil {
		return ""
	}
	if cookie := strings.TrimSpace(result.Cookie); cookie != "" {
		return cookie
	}
	if len(result.Cookies) == 0 {
		return ""
	}
	keys := make([]string, 0, len(result.Cookies))
	for k := range result.Cookies {
		if strings.TrimSpace(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(result.Cookies[k])
		if v == "" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func generateQRCodeBitmap(session *model.QRLoginSession) (*ui.Bitmap, error) {
	if session == nil {
		return nil, fmt.Errorf("二维码会话为空")
	}

	// 1. 如果有 ImageURL (例如 QQ 音乐直接返回 data:image/png;base64,...)
	if session.ImageURL != "" {
		raw := strings.TrimSpace(session.ImageURL)
		if idx := strings.Index(raw, ","); idx != -1 && strings.HasPrefix(raw, "data:") {
			raw = raw[idx+1:]
		}
		data, err := base64.StdEncoding.DecodeString(raw)
		if err == nil && len(data) > 0 {
			return ui.DecodeBitmap(data)
		}
	}

	// 2. 如果有 URL (例如网易云/酷狗/B站等返回用于扫码的 URL 链接)
	loginURL := strings.TrimSpace(session.URL)
	if loginURL == "" {
		return nil, fmt.Errorf("二维码链接为空")
	}

	pngData, err := qrcode.Encode(loginURL, qrcode.Medium, 220)
	if err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(pngData)
}

func (a *NativeApp) updateQRModalError(msg string) {
	if a.win != nil {
		a.win.Update(func() {
			a.mu.Lock()
			a.qrModal.ErrorText = msg
			a.mu.Unlock()
			a.win.Invalidate()
		})
	} else {
		a.mu.Lock()
		a.qrModal.ErrorText = msg
		a.mu.Unlock()
	}
}

func (a *NativeApp) startQRLogin(source string) {
	name := sourceDisplayNames[source]
	if name == "" {
		name = source
	}

	a.mu.Lock()
	a.qrModal = QRLoginModalState{
		Active:     true,
		Source:     source,
		SourceName: name,
		StatusText: "正在生成二维码…",
		Bitmap:     nil,
	}
	a.qrLoginPolling[source] = true
	a.mu.Unlock()

	if a.win != nil {
		a.win.Update(func() {
			a.win.Invalidate()
		})
	}

	go func() {
		defer func() {
			if a.win != nil {
				a.win.Update(func() {
					a.mu.Lock()
					a.qrLoginPolling[source] = false
					a.mu.Unlock()
					a.win.Invalidate()
				})
			} else {
				a.mu.Lock()
				a.qrLoginPolling[source] = false
				a.mu.Unlock()
			}
		}()

		fn := core.GetQRLoginCreateFunc(source)
		if fn == nil {
			a.updateQRModalError("该音源暂不支持扫码授权")
			return
		}
		session, err := fn()
		if err != nil {
			a.updateQRModalError("创建二维码失败: " + err.Error())
			return
		}

		bmp, err := generateQRCodeBitmap(session)
		if err != nil {
			a.updateQRModalError("解析二维码图像失败: " + err.Error())
			return
		}

		appTip := "请使用 " + name + " 手机 App 扫描"
		if source == "qq" {
			appTip = "请使用 手机 QQ 扫描二维码"
		}

		if a.win != nil {
			a.win.Update(func() {
				a.mu.Lock()
				a.qrModal.Bitmap = bmp
				a.qrModal.StatusText = appTip
				a.qrModal.Key = session.Key
				a.mu.Unlock()
				a.win.Invalidate()
			})
		} else {
			a.mu.Lock()
			a.qrModal.Bitmap = bmp
			a.qrModal.StatusText = appTip
			a.qrModal.Key = session.Key
			a.mu.Unlock()
		}

		checkFn := core.GetQRLoginCheckFunc(source)
		if checkFn == nil {
			return
		}

		// 轮询 45 次，每次 2.2 秒，近 100 秒有效时间
		for i := 0; i < 45; i++ {
			time.Sleep(2200 * time.Millisecond)

			// 用户手动关闭了模态弹窗则退出
			a.mu.Lock()
			if !a.qrModal.Active || a.qrModal.Source != source {
				a.mu.Unlock()
				return
			}
			a.mu.Unlock()

			res, err := checkFn(session.Key)
			if err != nil {
				continue
			}
			if res == nil {
				continue
			}

			switch res.Status {
			case model.QRLoginStatusScanned:
				if a.win != nil {
					a.win.Update(func() {
						a.mu.Lock()
						a.qrModal.StatusText = "已扫描，请在手机端点击确认授权…"
						a.mu.Unlock()
						a.win.Invalidate()
					})
				} else {
					a.mu.Lock()
					a.qrModal.StatusText = "已扫描，请在手机端点击确认授权…"
					a.mu.Unlock()
				}
			case model.QRLoginStatusSuccess:
				cookie := qrLoginCookieString(res)
				if cookie != "" {
					if a.win != nil {
						a.win.Update(func() {
							a.mu.Lock()
							a.qrModal.Success = true
							a.qrModal.StatusText = "✅ 授权成功，正在同步账号身份…"
							switch source {
							case "netease":
								a.cookieNetease = cookie
							case "qq":
								a.cookieQQ = cookie
							case "kugou":
								a.cookieKugou = cookie
							case "bilibili":
								a.cookieBilibili = cookie
							}
							core.CM.SetAll(map[string]string{source: cookie})
							core.CM.Save()
							a.mu.Unlock()
							a.win.Invalidate()
						})
					} else {
						a.mu.Lock()
						a.qrModal.Success = true
						a.qrModal.StatusText = "✅ 授权成功，正在同步账号身份…"
						switch source {
						case "netease":
							a.cookieNetease = cookie
						case "qq":
							a.cookieQQ = cookie
						case "kugou":
							a.cookieKugou = cookie
						case "bilibili":
							a.cookieBilibili = cookie
						}
						core.CM.SetAll(map[string]string{source: cookie})
						core.CM.Save()
						a.mu.Unlock()
					}

					// 立即异步验证获取真实昵称
					a.verifyPlatformAuth(source, false)

					// 稍作等待以展示验证出的昵称
					time.Sleep(800 * time.Millisecond)
					if a.win != nil {
						a.win.Update(func() {
							a.mu.Lock()
							st := a.authStatus[source]
							if st.Valid && st.Username != "" {
								vipBadge := ""
								if st.IsVIP {
									vipBadge = " (VIP)"
								}
								a.qrModal.StatusText = fmt.Sprintf("✅ 欢迎，%s%s！登录成功", st.Username, vipBadge)
							} else {
								a.qrModal.StatusText = "✅ 授权成功，已自动保存！"
							}
							a.mu.Unlock()
							a.win.Invalidate()
						})
					} else {
						a.mu.Lock()
						st := a.authStatus[source]
						if st.Valid && st.Username != "" {
							vipBadge := ""
							if st.IsVIP {
								vipBadge = " (VIP)"
							}
							a.qrModal.StatusText = fmt.Sprintf("✅ 欢迎，%s%s！登录成功", st.Username, vipBadge)
						} else {
							a.qrModal.StatusText = "✅ 授权成功，已自动保存！"
						}
						a.mu.Unlock()
					}

					time.Sleep(1500 * time.Millisecond)
					if a.win != nil {
						a.win.Update(func() {
							a.mu.Lock()
							if a.qrModal.Source == source {
								a.qrModal.Active = false
							}
							a.mu.Unlock()
							a.win.Invalidate()
						})
					} else {
						a.mu.Lock()
						if a.qrModal.Source == source {
							a.qrModal.Active = false
						}
						a.mu.Unlock()
					}
					return
				}
			case model.QRLoginStatusExpired:
				a.updateQRModalError("二维码已过期，请重新点击扫码")
				return
			case model.QRLoginStatusFailed:
				a.updateQRModalError("扫码登录已取消或失败")
				return
			}
		}

		a.updateQRModalError("扫码超时，请重新点击扫码")
	}()
}

// renderQRLoginModal 渲染 Pulse 原生二维码扫码弹窗
func (a *NativeApp) renderQRLoginModal(c *ui.Context) {
	if !a.qrModal.Active {
		return
	}
	pal := a.colors()

	// 遮罩层 (全屏绝对定位半透明黑)
	ui.Column(c.Key("qr-modal-backdrop")).Absolute().Fill().Background(ui.Hex("#000000a6")).Center().Children(func() {
		// 模态弹窗主体卡片 (340px 宽度)
		ui.Column(c.Key("qr-modal-card")).Width(340).Padding(22).Radius(16).
			Background(pal.Surface).Border(1, pal.Border).Gap(14).AlignItems(ui.Center).Children(func() {

			// 1. 顶部标题栏 + 关闭按钮
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Icon(c, iconQrCode).Size(16, 16).TextColor(pal.Accent)
					ui.Text(c, a.qrModal.SourceName+" 扫码授权").FontSize(14).Bold().TextColor(pal.Text)
				})
				ui.Spacer(c)
				btnClose := ui.Button(c.Key("btn-close-qr-modal"), "✕")
				if btnClose.Clicked() {
					a.qrModal.Active = false
				}
			})

			// 2. 二维码容器 (白色背景底框，高反差)
			ui.Row(c).Size(200, 200).Radius(12).Background(ui.Hex("#ffffff")).Center().Children(func() {
				if a.qrModal.Bitmap != nil {
					ui.Image(c, a.qrModal.Bitmap).Size(180, 180)
				} else if a.qrModal.ErrorText != "" {
					ui.Column(c).Center().Gap(6).Children(func() {
						ui.Icon(c, iconClose).Size(24, 24).TextColor(pal.Danger)
						ui.Text(c, "二维码不可用").FontSize(11).TextColor(ui.Hex("#444444"))
					})
				} else {
					ui.Column(c).Center().Gap(8).Children(func() {
						ui.Spinner(c)
						ui.Text(c, "生成中…").FontSize(11).TextColor(ui.Hex("#666666"))
					})
				}
			})

			// 3. 动态状态提示
			ui.Column(c).AlignItems(ui.Center).Gap(4).Children(func() {
				if a.qrModal.ErrorText != "" {
					ui.Text(c, a.qrModal.ErrorText).FontSize(12).TextColor(pal.Danger)
				} else {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						dotColor := pal.Accent
						if a.qrModal.Success {
							dotColor = pal.Success
						}
						ui.Row(c).Size(6, 6).Radius(3).Background(dotColor)
						ui.Text(c, a.qrModal.StatusText).FontSize(12).TextColor(pal.Text)
					})
				}
			})

			// 4. 操作按钮组
			ui.Row(c).Gap(10).Children(func() {
				if a.qrModal.ErrorText != "" {
					if ui.PrimaryButton(c.Key("btn-qr-retry"), "重新生成").Clicked() {
						a.startQRLogin(a.qrModal.Source)
					}
				}
				if ui.Button(c.Key("btn-qr-cancel"), "关闭").Clicked() {
					a.qrModal.Active = false
				}
			})
		})
	})
}

func (a *NativeApp) getPlatformCookie(source string) string {
	switch source {
	case "netease":
		return a.cookieNetease
	case "qq":
		return a.cookieQQ
	case "kugou":
		return a.cookieKugou
	case "bilibili":
		return a.cookieBilibili
	default:
		return ""
	}
}

func (a *NativeApp) loadMyPlaylists() {
	src := a.selectedMySource
	if src == "" {
		src = "netease"
	}
	cookie := a.getPlatformCookie(src)
	if strings.TrimSpace(cookie) == "" {
		a.myPlaylists = nil
		a.myPlaylistsError = ""
		return
	}

	a.loadingMyPlaylists = true
	a.myPlaylistsError = ""
	a.myPlaylists = nil

	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
		var pls []model.Playlist
		var err error
		switch src {
		case "netease":
			pls, err = netease.New(cookie).GetUserPlaylists(1, 20)
		case "qq":
			pls, err = qq.New(cookie).GetUserPlaylists(1, 20)
		case "kugou":
			pls, err = kugou.New(cookie).GetUserPlaylists(1, 20)
		case "bilibili":
			pls, err = bilibili.New(cookie).GetUserPlaylists(1, 20)
		}

		if a.win != nil {
			a.win.Update(func() {
				a.loadingMyPlaylists = false
				if err != nil {
					a.myPlaylistsError = err.Error()
				} else {
					a.myPlaylists = pls
				}
			})
		}
	}()
}

func (a *NativeApp) verifyPlatformAuth(source string, showToast bool) {
	cookie := a.getPlatformCookie(source)

	if strings.TrimSpace(cookie) == "" {
		a.mu.Lock()
		a.authStatus[source] = PlatformAuthStatus{
			Checked:   true,
			Valid:     false,
			Error:     "未配置 Cookie",
			CheckTime: time.Now().Format("15:04"),
		}
		a.mu.Unlock()
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}

	a.mu.Lock()
	st := a.authStatus[source]
	st.Checking = true
	a.authStatus[source] = st
	a.mu.Unlock()
	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
		valid := false
		username := ""
		isVip := false
		plCount := 0
		var err error

		switch source {
		case "netease":
			client := netease.New(cookie)
			isVip, _ = client.IsVipAccount()
			pls, e := client.GetUserPlaylists(1, 5)
			if e == nil {
				valid = true
				plCount = len(pls)
				if len(pls) > 0 && pls[0].Creator != "" {
					username = pls[0].Creator
				} else {
					username = "网易云用户"
				}
			} else {
				err = e
			}

		case "qq":
			client := qq.New(cookie)
			isVip, _ = client.IsVipAccount()
			pls, e := client.GetUserPlaylists(1, 5)
			if e == nil {
				valid = true
				plCount = len(pls)
				if len(pls) > 0 && pls[0].Creator != "" {
					username = pls[0].Creator
				} else {
					username = "QQ 音乐用户"
				}
			} else {
				err = e
			}

		case "kugou":
			client := kugou.New(cookie)
			pls, e := client.GetUserPlaylists(1, 5)
			if e == nil {
				valid = true
				plCount = len(pls)
				if len(pls) > 0 && pls[0].Creator != "" {
					username = pls[0].Creator
				} else {
					username = "酷狗用户"
				}
			} else {
				err = e
			}

		case "bilibili":
			client := bilibili.New(cookie)
			isVip, _ = client.IsVipAccount()
			pls, e := client.GetUserPlaylists(1, 5)
			if e == nil {
				valid = true
				plCount = len(pls)
				if len(pls) > 0 && pls[0].Creator != "" {
					username = pls[0].Creator
				} else {
					username = "B站用户"
				}
			} else {
				err = e
			}
		}

		a.mu.Lock()
		newSt := PlatformAuthStatus{
			Checked:       true,
			Valid:         valid,
			Checking:      false,
			Username:      username,
			IsVIP:         isVip,
			PlaylistCount: plCount,
			CheckTime:     time.Now().Format("15:04"),
		}
		if err != nil {
			newSt.Error = err.Error()
		}
		a.authStatus[source] = newSt
		a.mu.Unlock()

		if a.win != nil {
			a.win.Update(func() {
				// 状态更新后触发重绘
			})
		}
	}()
}

func (a *NativeApp) startPlaybackTicker() {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		a.mu.Lock()
		if a.player != nil && a.playerPlaying {
			pos := a.player.Position()
			dur := a.player.Duration()
			posSec := int(pos.Seconds())
			durSec := int(dur.Seconds())
			if durSec == 0 && a.currentSong != nil && a.currentSong.Duration > 0 {
				durSec = a.currentSong.Duration
			}
			a.playPosSec = posSec
			a.playDurSec = durSec
			if durSec > 0 {
				a.playProgress = float64(posSec) / float64(durSec)
				if a.playProgress > 1.0 {
					a.playProgress = 1.0
				}
			}

			// 检查是否播放完毕 (允许 1 秒缓冲容差)
			if durSec > 0 && posSec >= durSec {
				a.mu.Unlock()
				a.playNextSong()
				continue
			}
			a.mu.Unlock()

			if a.win != nil {
				a.win.Update(func() {
					a.win.Invalidate()
				})
			}
		} else {
			a.mu.Unlock()
		}
	}
}

func (a *NativeApp) playSongFromList(song model.Song, list []model.Song, idx int) {
	a.mu.Lock()
	a.playQueue = list
	a.playIndex = idx
	a.mu.Unlock()
	a.playSong(song)
}

func (a *NativeApp) playNextSong() {
	a.mu.Lock()
	if len(a.playQueue) == 0 {
		a.mu.Unlock()
		return
	}
	if a.singleLoop && a.currentSong != nil {
		s := *a.currentSong
		a.mu.Unlock()
		a.playSong(s)
		return
	}
	nextIdx := a.playIndex + 1
	if nextIdx >= len(a.playQueue) {
		nextIdx = 0
	}
	nextSong := a.playQueue[nextIdx]
	a.playIndex = nextIdx
	a.mu.Unlock()
	a.playSong(nextSong)
}

func (a *NativeApp) playPrevSong() {
	a.mu.Lock()
	if len(a.playQueue) == 0 {
		a.mu.Unlock()
		return
	}
	prevIdx := a.playIndex - 1
	if prevIdx < 0 {
		prevIdx = len(a.playQueue) - 1
	}
	prevSong := a.playQueue[prevIdx]
	a.playIndex = prevIdx
	a.mu.Unlock()
	a.playSong(prevSong)
}

func (a *NativeApp) loadMoreSearchResults() {
	if a.loadingMore || strings.TrimSpace(a.searchKeyword) == "" || a.hasLoadedMore {
		return
	}
	a.loadingMore = true

	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
		kw := a.searchKeyword
		extraSources := []string{"kugou", "kuwo", "migu", "bilibili", "jamendo", "fivesing"}
		var moreSongs []model.Song
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, src := range extraSources {
			fn := core.GetSearchFunc(src)
			if fn == nil {
				continue
			}
			wg.Add(1)
			go func(s string, searchFn core.SearchFunc) {
				defer wg.Done()
				res, err := searchFn(kw)
				if err == nil && len(res) > 0 {
					mu.Lock()
					moreSongs = append(moreSongs, res...)
					mu.Unlock()
				}
			}(src, fn)
		}
		wg.Wait()

		updateFunc := func() {
			a.loadingMore = false
			a.hasLoadedMore = true
			if len(moreSongs) > 0 {
				existing := make(map[string]bool)
				for _, s := range a.searchResults {
					key := s.Source + "_" + s.ID
					existing[key] = true
					existing[s.Name+"_"+s.Artist] = true
				}
				for _, s := range moreSongs {
					key := s.Source + "_" + s.ID
					nameKey := s.Name + "_" + s.Artist
					if !existing[key] && !existing[nameKey] {
						existing[key] = true
						existing[nameKey] = true
						a.searchResults = append(a.searchResults, s)
					}
				}
			}
		}

		if a.win != nil {
			a.win.Update(updateFunc)
		} else {
			updateFunc()
		}
	}()
}

func (a *NativeApp) loadCustomPlaylists() {
	p := filepath.Join(a.downloadDir, "collections.json")
	data, err := os.ReadFile(p)
	if err == nil {
		var list []CustomPlaylist
		if json.Unmarshal(data, &list) == nil && len(list) > 0 {
			a.customPlaylists = list
			return
		}
	}
	a.customPlaylists = []CustomPlaylist{
		{
			ID:          "fav_01",
			Name:        "我喜欢的音乐",
			Description: "默认自制本地收藏歌单",
			CreatedAt:   time.Now().Format("2006-01-02"),
			Songs:       nil,
		},
	}
	a.saveCustomPlaylists()
}

func (a *NativeApp) saveCustomPlaylists() {
	p := filepath.Join(a.downloadDir, "collections.json")
	data, err := json.MarshalIndent(a.customPlaylists, "", "  ")
	if err != nil {
		log.Printf("[saveCustomPlaylists] json marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(a.downloadDir, 0755); err != nil {
		log.Printf("[saveCustomPlaylists] mkdir failed: %v", err)
		return
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		log.Printf("[saveCustomPlaylists] failed to write %s: %v", p, err)
	}
}

func (a *NativeApp) createCustomPlaylist(name, desc string) {
	id := fmt.Sprintf("col_%d", time.Now().Unix())
	newCol := CustomPlaylist{
		ID:          id,
		Name:        name,
		Description: desc,
		CreatedAt:   time.Now().Format("2006-01-02"),
		Songs:       nil,
	}
	a.customPlaylists = append(a.customPlaylists, newCol)
	a.saveCustomPlaylists()
}

func (a *NativeApp) deleteCustomPlaylist(colID string) {
	var newList []CustomPlaylist
	for _, c := range a.customPlaylists {
		if c.ID != colID {
			newList = append(newList, c)
		}
	}
	a.customPlaylists = newList
	a.saveCustomPlaylists()
}

func (a *NativeApp) addSongToCustomPlaylist(colID string, song model.Song) bool {
	for i := range a.customPlaylists {
		if a.customPlaylists[i].ID == colID {
			for _, s := range a.customPlaylists[i].Songs {
				if s.ID == song.ID && s.Source == song.Source {
					return false
				}
			}
			a.customPlaylists[i].Songs = append(a.customPlaylists[i].Songs, song)
			a.saveCustomPlaylists()
			return true
		}
	}
	return false
}

func (a *NativeApp) removeSongFromCustomPlaylist(colID string, songID string, source string) {
	for i := range a.customPlaylists {
		if a.customPlaylists[i].ID == colID {
			var newSongs []model.Song
			for _, s := range a.customPlaylists[i].Songs {
				if !(s.ID == songID && s.Source == source) {
					newSongs = append(newSongs, s)
				}
			}
			a.customPlaylists[i].Songs = newSongs
			a.saveCustomPlaylists()
			return
		}
	}
}

func (a *NativeApp) isSongInCustomPlaylist(colID string, songID, source string) bool {
	for _, col := range a.customPlaylists {
		if col.ID == colID {
			for _, s := range col.Songs {
				if s.ID == songID && s.Source == source {
					return true
				}
			}
		}
	}
	return false
}

func parseLRC(lrc string) []LyricLine {
	lines := strings.Split(lrc, "\n")
	var result []LyricLine
	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if !strings.HasPrefix(raw, "[") {
			continue
		}
		idx := strings.Index(raw, "]")
		if idx == -1 {
			continue
		}
		timeStr := raw[1:idx]
		textStr := strings.TrimSpace(raw[idx+1:])
		parts := strings.Split(timeStr, ":")
		if len(parts) == 2 {
			m, e1 := strconv.ParseFloat(parts[0], 64)
			s, e2 := strconv.ParseFloat(parts[1], 64)
			if e1 == nil && e2 == nil {
				result = append(result, LyricLine{
					TimeSec: m*60 + s,
					Text:    textStr,
				})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].TimeSec < result[j].TimeSec
	})
	return result
}
