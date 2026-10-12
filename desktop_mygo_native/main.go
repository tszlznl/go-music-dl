package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3"
	"github.com/egoist/mygo/ui"
	"github.com/guohuiyuan/go-music-dl/core"
	"github.com/guohuiyuan/music-lib/model"
)

//go:embed icon.png
var appIconPNG []byte

const (
	appTitle      = "Go Music DL (Native UI)"
	appWindowKey  = "music-dl-native-window"
	defaultWidth  = 1180
	defaultHeight = 760
	minWidth      = 900
	minHeight     = 600
	githubRepoURL = "https://github.com/guohuiyuan/go-music-dl"
)

var sourceDisplayNames = map[string]string{
	"all":      "全部聚合",
	"netease":  "网易云音乐",
	"qq":       "QQ音乐",
	"kugou":    "酷狗音乐",
	"kuwo":     "酷我音乐",
	"migu":     "咪咕音乐",
	"bilibili": "Bilibili",
	"soda":     "汽水音乐",
	"apple":    "Apple Music",
}

type DownloadItem struct {
	Song      model.Song
	Status    string // "下载中...", "已完成", "下载失败"
	SavedPath string
	Error     string
	TimeStr   string
}

type NativeApp struct {
	win *mygo.Window

	// 导航标签: "search", "downloads", "settings"
	activeTab string

	// 搜索状态
	searchKeyword  string
	selectedSource string
	sourceOptions  []string
	searching      bool
	searchError    string
	searchResults  []model.Song

	// 音频播放器状态
	currentSong   *model.Song
	player        *audio.Player
	playerPlaying bool
	loadingAudio  bool
	audioError    string
	volume        float64

	// 下载管理
	downloadDir string
	downloads   []DownloadItem

	// 基础设置
	filenameTemplate string
	notifEnabled     bool

	mu sync.Mutex
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

	return &NativeApp{
		activeTab:        "search",
		searchKeyword:    "",
		selectedSource:   "all",
		sourceOptions:    []string{"all", "netease", "qq", "kugou", "kuwo", "migu", "bilibili", "soda"},
		volume:           0.8,
		downloadDir:      downloadDir,
		filenameTemplate: core.DefaultDownloadFilenameTemplate,
		notifEnabled:     true,
	}
}

func main() {
	var (
		flagVersion bool
	)
	flag.BoolVar(&flagVersion, "v", false, "显示版本信息")
	flag.BoolVar(&flagVersion, "version", false, "显示版本信息")
	flag.Parse()

	if flagVersion {
		fmt.Printf("%s v%s (Pure Go GPU Native UI)\n", appTitle, core.AppVersion)
		return
	}

	mygo.App.SetName(appTitle)
	mygo.App.SetVersion(core.AppVersion)

	if !mygo.App.RequestSingleInstanceLock() {
		log.Println("检测到另一个 Music DL Native 实例已在运行中，将唤醒已有窗口。")
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
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     appTitle,
			Width:     defaultWidth,
			Height:    defaultHeight,
			MinWidth:  minWidth,
			MinHeight: minHeight,
			StateKey:  appWindowKey,
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
					Label: "打开下载目录",
					Click: func(item *mygo.MenuItem, w *mygo.Window) {
						_ = mygo.Shell.OpenPath(a.downloadDir)
					},
				},
				mygo.Separator(),
				{
					Label:       "退出",
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
					Label: "访问 GitHub 项目主页",
					Click: func(item *mygo.MenuItem, w *mygo.Window) {
						_ = mygo.Shell.OpenExternal(githubRepoURL)
					},
				},
				mygo.Separator(),
				{
					Label: "关于 Music DL (Native UI)",
					Click: func(item *mygo.MenuItem, w *mygo.Window) {
						_, _ = mygo.Dialog.Message(mygo.MessageOptions{
							Parent:  w,
							Title:   "关于 " + appTitle,
							Message: fmt.Sprintf("%s v%s\n纯 Go 编写，GPU 原生渲染，无 Webview，无 CGO 依赖。\n\n项目主页: %s", appTitle, core.AppVersion, githubRepoURL),
							Buttons: []string{"确定"},
						})
					},
				},
			},
		},
	}

	mygo.App.SetMenu(mygo.NewMenu(items))
}

func (a *NativeApp) view(c *ui.Context) {
	t := c.Theme()

	ui.Column(c).Fill().Background(t.Background).Children(func() {
		// 主工作区：左侧侧边栏 + 右侧主内容
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			ui.Column(c).Grow(1).Padding(16, 20).Gap(14).Children(func() {
				switch a.activeTab {
				case "search":
					a.searchPageView(c)
				case "downloads":
					a.downloadsPageView(c)
				case "settings":
					a.settingsPageView(c)
				default:
					a.searchPageView(c)
				}
			})
		})

		// 底部常驻播放器栏
		a.playerBar(c)
	})
}

func (a *NativeApp) sidebar(c *ui.Context) {
	t := c.Theme()

	ui.Column(c).Width(190).Padding(16, 12).Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		// 标题
		ui.Row(c).Padding(4, 8, 16).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "🎵").FontSize(20)
			ui.Column(c).Children(func() {
				ui.Text(c, "Music DL").FontSize(15).Bold()
				ui.Text(c, "Native UI 版").FontSize(11).TextColor(t.TextMuted)
			})
		})

		// 导航菜单项
		tabs := []struct {
			id    string
			icon  string
			label string
		}{
			{"search", "🔍", "音乐搜索"},
			{"downloads", "📥", "下载管理"},
			{"settings", "⚙️", "偏好设置"},
		}

		ui.Column(c).Gap(6).Grow(1).Children(func() {
			for _, tab := range tabs {
				tabID := tab.id
				isSelected := a.activeTab == tabID

				btn := ui.Row(c.Key(tabID)).Padding(8, 12).Radius(8).Gap(10).AlignItems(ui.Center)
				if isSelected {
					btn.Background(t.Accent).TextColor(t.AccentText)
				} else {
					btn.Background(t.Surface)
				}

				if btn.Clicked() {
					a.activeTab = tabID
				}

				btn.Children(func() {
					ui.Text(c, tab.icon).FontSize(14)
					label := ui.Text(c, tab.label).FontSize(13)
					if isSelected {
						label.Bold()
					}
					if tabID == "downloads" && len(a.downloads) > 0 {
						ui.Spacer(c)
						ui.Badge(c, fmt.Sprint(len(a.downloads)))
					}
				})
			}
		})

		// 底部版本标识
		ui.Row(c).Padding(8, 8, 0).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "版本: v"+core.AppVersion).FontSize(11).TextColor(t.TextMuted)
		})
	})
}

func (a *NativeApp) searchPageView(c *ui.Context) {
	t := c.Theme()

	// 顶部搜索控制栏
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		searchField := ui.SearchField(c, &a.searchKeyword).Placeholder("输入歌曲名、歌手名或关键词回车搜索...").Grow(1)
		if searchField.Submitted() {
			a.startSearch()
		}

		// 平台选择
		displayOptions := make([]string, len(a.sourceOptions))
		for i, s := range a.sourceOptions {
			displayOptions[i] = sourceDisplayNames[s]
		}
		selectedLabel := sourceDisplayNames[a.selectedSource]
		if ui.Select(c, &selectedLabel, displayOptions).Width(130).Changed() {
			for src, label := range sourceDisplayNames {
				if label == selectedLabel {
					a.selectedSource = src
					break
				}
			}
		}

		btn := ui.PrimaryButton(c, "搜索").Disabled(a.searching || strings.TrimSpace(a.searchKeyword) == "")
		if btn.Clicked() {
			a.startSearch()
		}
	})

	// 搜索状态展示
	if a.searching {
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			ui.Spinner(c)
			ui.Text(c, "正在全网并发搜索音乐中，请稍候...").TextColor(t.TextMuted).FontSize(14)
		})
		return
	}

	if a.searchError != "" {
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			ui.Text(c, "⚠️ 搜索遇到错误: "+a.searchError).TextColor(t.Danger).FontSize(14)
			if ui.Button(c, "重试").Clicked() {
				a.startSearch()
			}
		})
		return
	}

	if len(a.searchResults) == 0 {
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			ui.Text(c, "🎵").FontSize(44)
			ui.Text(c, "欢迎使用 Go Music DL 原生桌面端").FontSize(17).Bold()
			ui.Text(c, "支持网易云、QQ音乐、酷狗、酷我、咪咕、B站等多平台搜索与无损下载").FontSize(13).TextColor(t.TextMuted)
			ui.Text(c, "在上方输入歌名并点击“搜索”即可开始浏览试听").FontSize(12).TextColor(t.TextMuted)
		})
		return
	}

	// 结果统计栏
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, fmt.Sprintf("搜索到 %d 首歌曲", len(a.searchResults))).FontSize(13).Bold()
		ui.Spacer(c)
		ui.Text(c, "平台: "+sourceDisplayNames[a.selectedSource]).FontSize(12).TextColor(t.TextMuted)
	})

	// 搜索结果列表
	ui.Scroll(c).Grow(1).Gap(6).Children(func() {
		for i, item := range a.searchResults {
			song := item
			isCurrentPlaying := a.currentSong != nil && a.currentSong.ID == song.ID && a.currentSong.Source == song.Source && a.playerPlaying

			row := ui.Row(c.Key(fmt.Sprintf("song-%s-%s-%d", song.Source, song.ID, i))).
				Padding(10, 14).Radius(8).Background(t.Surface).Border(1, t.Border).AlignItems(ui.Center).Gap(12)

			row.Children(func() {
				// 序号
				ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(12).TextColor(t.TextMuted).Width(24)

				// 平台 Badge
				sourceLabel := sourceDisplayNames[song.Source]
				if sourceLabel == "" {
					sourceLabel = song.Source
				}
				ui.Badge(c, sourceLabel)

				// 歌曲信息
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(c, song.Name).FontSize(14).Bold()
						if song.IsVIP {
							ui.Badge(c, "VIP")
						}
						if song.Ext != "" {
							ui.Text(c, strings.ToUpper(song.Ext)).FontSize(10).TextColor(t.TextMuted)
						}
					})
					artistAlbum := song.Artist
					if song.Album != "" {
						artistAlbum += " - " + song.Album
					}
					ui.Text(c, artistAlbum).FontSize(12).TextColor(t.TextMuted)
				})

				// 时长与大小
				ui.Column(c).AlignItems(ui.End).Width(80).Children(func() {
					if song.Duration > 0 {
						ui.Text(c, formatDuration(song.Duration)).FontSize(12).TextColor(t.TextMuted)
					}
					if song.Size > 0 {
						ui.Text(c, core.FormatSize(song.Size)).FontSize(11).TextColor(t.TextMuted)
					}
				})

				// 播放/试听按钮
				playBtnLabel := "▶ 播放"
				if isCurrentPlaying {
					playBtnLabel = "⏸ 暂停"
				}
				if ui.Button(c.Key("play-"+song.ID), playBtnLabel).Clicked() {
					if isCurrentPlaying {
						a.pauseAudio()
					} else {
						a.playSong(song)
					}
				}

				// 下载按钮
				if ui.Button(c.Key("download-"+song.ID), "⬇ 下载").Clicked() {
					a.downloadSong(song)
					c.Toast("已添加到下载列表: " + song.Name)
				}

				// 复制链接
				if ui.Button(c.Key("copy-"+song.ID), "📋").Tooltip("复制歌曲详情链接").Clicked() {
					link := song.Link
					if link == "" {
						link = song.URL
					}
					c.WriteClipboard(link)
					c.Toast("已复制歌曲链接")
				}
			})
		}
	})
}

func (a *NativeApp) downloadsPageView(c *ui.Context) {
	t := c.Theme()

	// 顶部操作栏
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, "下载目录:").FontSize(13).Bold()
		ui.Text(c, a.downloadDir).FontSize(12).TextColor(t.TextMuted).Grow(1)

		if ui.Button(c, "📁 打开下载目录").Clicked() {
			_ = mygo.Shell.OpenPath(a.downloadDir)
		}

		if ui.Button(c, "📂 更改目录").Clicked() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent:    a.win,
				Title:     "选择下载保存目录",
				Directory: true,
			})
			if err == nil && len(paths) > 0 && paths[0] != "" {
				a.downloadDir = paths[0]
				c.Toast("已更新下载保存目录")
			}
		}

		if len(a.downloads) > 0 {
			if ui.Button(c, "🗑️ 清空列表").Clicked() {
				a.downloads = nil
				c.Toast("已清空下载记录")
			}
		}
	})

	if len(a.downloads) == 0 {
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			ui.Text(c, "📥").FontSize(40)
			ui.Text(c, "暂无下载任务").FontSize(15).Bold()
			ui.Text(c, "在音乐搜索页面点击“下载”按钮，下载记录将在此显示。").FontSize(12).TextColor(t.TextMuted)
		})
		return
	}

	ui.Scroll(c).Grow(1).Gap(8).Children(func() {
		for i, item := range a.downloads {
			dl := item
			row := ui.Row(c.Key(fmt.Sprintf("dl-%d", i))).
				Padding(10, 14).Radius(8).Background(t.Surface).Border(1, t.Border).AlignItems(ui.Center).Gap(12)

			row.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(c, dl.Song.Name).FontSize(14).Bold()
						ui.Badge(c, sourceDisplayNames[dl.Song.Source])
					})
					ui.Text(c, dl.Song.Artist).FontSize(12).TextColor(t.TextMuted)
					if dl.SavedPath != "" {
						ui.Text(c, "文件: "+filepath.Base(dl.SavedPath)).FontSize(11).TextColor(t.TextMuted)
					}
					if dl.Error != "" {
						ui.Text(c, "错误: "+dl.Error).FontSize(11).TextColor(t.Danger)
					}
				})

				// 状态标签
				switch dl.Status {
				case "已完成":
					ui.Text(c, "✓ 已完成").FontSize(12).TextColor(t.Success).Bold()
				case "下载失败":
					ui.Text(c, "✕ 失败").FontSize(12).TextColor(t.Danger).Bold()
				default:
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c)
						ui.Text(c, dl.Status).FontSize(12).TextColor(t.TextMuted)
					})
				}

				ui.Text(c, dl.TimeStr).FontSize(11).TextColor(t.TextMuted)

				// 定位文件按钮
				if dl.SavedPath != "" {
					if ui.Button(c.Key("loc-"+fmt.Sprint(i)), "在文件夹中显示").Clicked() {
						mygo.Shell.ShowItemInFolder(dl.SavedPath)
					}
				}
			})
		}
	})
}

func (a *NativeApp) settingsPageView(c *ui.Context) {
	t := c.Theme()

	ui.Scroll(c).Grow(1).Gap(16).Children(func() {
		// 下载设置卡片
		card(c, "📥 下载设置", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "保存路径:").Width(90).FontSize(13)
				ui.Text(c, a.downloadDir).FontSize(12).TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, "更改路径").Clicked() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
						Parent:    a.win,
						Title:     "选择下载保存目录",
						Directory: true,
					})
					if err == nil && len(paths) > 0 && paths[0] != "" {
						a.downloadDir = paths[0]
						c.Toast("已更新下载保存目录")
					}
				}
			})

			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "命名模板:").Width(90).FontSize(13)
				ui.TextInput(c, &a.filenameTemplate).Width(260)
				ui.Text(c, "支持 {artist}, {name}, {album}").FontSize(12).TextColor(t.TextMuted)
			})
		})

		// 桌面通知与系统交互
		card(c, "🔔 桌面交互", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Switch(c, &a.notifEnabled)
				ui.Text(c, "下载完成触发系统桌面通知").FontSize(13)
			})
		})

		// 关于与系统信息
		card(c, "ℹ️ 系统信息与关于", func() {
			infoItems := []struct {
				k, v string
			}{
				{"应用版本", "v" + core.AppVersion},
				{"渲染架构", "MyGo UI (纯 Go 原生 GPU 渲染，无 Webview / 无 CGO)"},
				{"音频引擎", "MyGo Audio (Pure Go MP3/WAV 解码与系统输出)"},
				{"音乐解析库", "github.com/guohuiyuan/music-lib"},
				{"开源主页", githubRepoURL},
			}

			for _, it := range infoItems {
				ui.Row(c).Gap(10).Children(func() {
					ui.Text(c, it.k).Width(100).FontSize(13).TextColor(t.TextMuted)
					ui.Text(c, it.v).FontSize(13).Bold()
				})
			}

			ui.Row(c).Padding(8, 0, 0).Children(func() {
				if ui.PrimaryButton(c, "在浏览器中访问 GitHub 仓库").Clicked() {
					_ = mygo.Shell.OpenExternal(githubRepoURL)
				}
			})
		})
	})
}

func (a *NativeApp) playerBar(c *ui.Context) {
	t := c.Theme()

	ui.Row(c).Height(64).Padding(8, 20).Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).AlignItems(ui.Center).Gap(16).Children(func() {
		// 当前歌曲信息
		if a.currentSong != nil {
			ui.Column(c).Width(240).Gap(3).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.currentSong.Name).FontSize(13).Bold()
					ui.Badge(c, sourceDisplayNames[a.currentSong.Source])
				})
				ui.Text(c, a.currentSong.Artist).FontSize(11).TextColor(t.TextMuted)
			})
		} else {
			ui.Row(c).Width(240).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "尚未播放歌曲").FontSize(12).TextColor(t.TextMuted)
			})
		}

		// 播放器状态 / 加载提示
		if a.loadingAudio {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在解析音频流并加载...").FontSize(12).TextColor(t.TextMuted)
			})
		} else if a.audioError != "" {
			ui.Text(c, "⚠️ "+a.audioError).FontSize(12).TextColor(t.Danger)
		}

		ui.Spacer(c)

		// 播放 / 暂停 控制按钮
		if a.currentSong != nil && !a.loadingAudio {
			playLabel := "▶ 播放"
			if a.playerPlaying {
				playLabel = "⏸ 暂停"
			}

			if ui.PrimaryButton(c, playLabel).Clicked() {
				if a.playerPlaying {
					a.pauseAudio()
				} else {
					a.resumeAudio()
				}
			}

			if ui.Button(c, "⏹ 停止").Clicked() {
				a.stopAudio()
			}
		}

		// 音量控制
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "🔊").FontSize(13)
			slider := ui.Slider(c, &a.volume, 0, 1).Width(90)
			if slider.Changed() && a.player != nil {
				a.player.SetVolume(a.volume)
			}
			ui.Text(c, fmt.Sprintf("%d%%", int(a.volume*100))).FontSize(11).TextColor(t.TextMuted).Width(34)
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

	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
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
	}()
}

func (a *NativeApp) performSearch(keyword, source string) ([]model.Song, error) {
	if source == "all" {
		sources := []string{"netease", "qq", "kugou", "kuwo", "migu", "bilibili"}
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
	if a.player != nil {
		a.player.Stop()
		a.player.Close()
		a.player = nil
	}
	a.currentSong = &song
	a.loadingAudio = true
	a.playerPlaying = false
	a.audioError = ""
	a.mu.Unlock()

	if a.win != nil {
		a.win.Invalidate()
	}

	go func() {
		dlData, err := core.DownloadSongData(&song, false, false)
		if err != nil {
			if a.win != nil {
				a.win.Update(func() {
					a.loadingAudio = false
					a.audioError = "获取音频失败: " + err.Error()
				})
			}
			return
		}

		p, err := audio.OpenStream(bytes.NewReader(dlData.Data))
		if a.win != nil {
			a.win.Update(func() {
				a.loadingAudio = false
				if err != nil {
					a.audioError = "音频解码失败: " + err.Error()
					log.Printf("音频解码异常: %v", err)
				} else {
					a.player = p
					a.player.SetVolume(a.volume)
					a.player.Play()
					a.playerPlaying = true
				}
			})
		}
	}()
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
	if a.player != nil {
		a.player.Stop()
		a.playerPlaying = false
	}
	a.currentSong = nil
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *NativeApp) downloadSong(song model.Song) {
	item := DownloadItem{
		Song:    song,
		Status:  "下载中...",
		TimeStr: time.Now().Format("15:04:05"),
	}

	a.mu.Lock()
	a.downloads = append([]DownloadItem{item}, a.downloads...)
	a.mu.Unlock()

	if a.win != nil {
		a.win.Invalidate()
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
							a.downloads[i].Status = "已完成"
							if saved != nil {
								a.downloads[i].SavedPath = saved.SavedPath
							}
						}
						break
					}
				}
			})
		}

		if err == nil && a.notifEnabled && mygo.NotificationsSupported() {
			n := mygo.NewNotification(mygo.NotificationOptions{
				Title: "音乐下载完成",
				Body:  fmt.Sprintf("%s - %s", song.Artist, song.Name),
			})
			if saved != nil && saved.SavedPath != "" {
				n.OnClick(func() { mygo.Shell.ShowItemInFolder(saved.SavedPath) })
			}
			_ = n.Show()
		}
	}()
}

func card(c *ui.Context, title string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(16).Gap(10).Radius(10).Background(t.Surface).Border(1, t.Border).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(14).Bold()
		}
		body()
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
