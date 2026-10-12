package main

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/guohuiyuan/music-lib/model"
)

func TestAppIconPNG(t *testing.T) {
	if len(appIconPNG) == 0 {
		t.Fatal("嵌入的 appIconPNG 资源为空")
	}

	pngSignature := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.HasPrefix(appIconPNG, pngSignature) {
		t.Fatalf("嵌入的图标不是合法的 PNG 文件头: %v", appIconPNG[:8])
	}
}

func TestLucideIconsLoaded(t *testing.T) {
	icons := []*ui.SVG{
		iconSearch, iconMusic, iconDownload, iconSettings,
		iconPlay, iconPause, iconSquare, iconVolume,
		iconVolumeMute, iconFolder, iconFolderOpen, iconTrash,
		iconCopy, iconCheck, iconExternalLink, iconDisc,
		iconBell, iconInfo, iconSparkles, iconRadio,
		iconMinimize, iconMaximize, iconClose,
	}

	for i, ic := range icons {
		if ic == nil {
			t.Fatalf("第 %d 个 Lucide SVG 图标未正常加载解析", i)
		}
		w, h := ic.Size()
		if w <= 0 || h <= 0 {
			t.Fatalf("第 %d 个 Lucide SVG 图标尺寸异常: w=%v, h=%v", i, w, h)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "--:--"},
		{-10, "--:--"},
		{60, "01:00"},
		{125, "02:05"},
		{3599, "59:59"},
		{3600, "60:00"},
	}

	for _, tc := range tests {
		got := formatDuration(tc.input)
		if got != tc.expected {
			t.Errorf("formatDuration(%d) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestNativeAppInitialView(t *testing.T) {
	app := newNativeApp()
	tester := ui.NewTester(app.view, 1200, 780)

	if !tester.HasText("Go Music DL") {
		t.Error("未找到窗口顶栏标题 Go Music DL")
	}
	if !tester.HasText("GPU Native") {
		t.Error("未找到顶栏标签 GPU Native")
	}
	if !tester.HasText("音乐搜索") {
		t.Error("未找到音乐搜索导航项")
	}
	if !tester.HasText("下载管理") {
		t.Error("未找到下载管理导航项")
	}
	if !tester.HasText("偏好设置") {
		t.Error("未找到偏好设置导航项")
	}
	if !tester.HasText("欢迎使用 Go Music DL 原生桌面端") {
		t.Error("初始空搜索页面提示未显示")
	}
	if !tester.HasText("Pulse UI 引擎") {
		t.Error("侧边栏底部 Pulse UI 引擎标识未显示")
	}
}

func TestNativeAppTabNavigation(t *testing.T) {
	app := newNativeApp()
	tester := ui.NewTester(app.view, 1200, 780)

	// 初始状态为 search
	if app.activeTab != "search" {
		t.Fatalf("预期初始 activeTab 为 search，实际为 %s", app.activeTab)
	}

	// 切换到下载管理
	if err := tester.Click("下载管理"); err != nil {
		t.Fatalf("点击下载管理标签失败: %v", err)
	}
	tester.Frame()

	if app.activeTab != "downloads" {
		t.Fatalf("预期切换后 activeTab 为 downloads，实际为 %s", app.activeTab)
	}
	if !tester.HasText("暂无下载任务") {
		t.Error("切换到下载管理后未显示'暂无下载任务'")
	}

	// 切换到偏好设置
	if err := tester.Click("偏好设置"); err != nil {
		t.Fatalf("点击偏好设置标签失败: %v", err)
	}
	tester.Frame()

	if app.activeTab != "settings" {
		t.Fatalf("预期切换后 activeTab 为 settings，实际为 %s", app.activeTab)
	}
	if !tester.HasText("下载设置") {
		t.Error("切换到偏好设置后未显示'下载设置'")
	}
	if !tester.HasText("系统信息与关于") {
		t.Error("切换到偏好设置后未显示'系统信息与关于'")
	}

	// 切换回音乐搜索
	if err := tester.Click("音乐搜索"); err != nil {
		t.Fatalf("点击音乐搜索标签失败: %v", err)
	}
	tester.Frame()

	if app.activeTab != "search" {
		t.Fatalf("预期切回 activeTab 为 search，实际为 %s", app.activeTab)
	}
}

func TestNativeAppSearchResultsAndPlayerView(t *testing.T) {
	app := newNativeApp()
	mockSongs := []model.Song{
		{
			ID:       "song_001",
			Name:     "春日影 (Haruhikage)",
			Artist:   "MyGO!!!!!",
			Album:    "迷迹波",
			Duration: 245,
			Source:   "netease",
			IsVIP:    true,
			Ext:      "flac",
		},
		{
			ID:       "song_002",
			Name:     "碧天伴走",
			Artist:   "MyGO!!!!!",
			Album:    "迷迹波",
			Duration: 218,
			Source:   "qq",
			IsVIP:    false,
			Ext:      "mp3",
		},
	}

	app.searchResults = mockSongs
	tester := ui.NewTester(app.view, 1200, 780)

	if !tester.HasText("搜索到 2 首歌曲") {
		t.Error("未找到搜索结果数量展示")
	}
	if !tester.HasText("春日影 (Haruhikage)") {
		t.Error("未渲染第一首歌曲名称")
	}
	if !tester.HasText("碧天伴走") {
		t.Error("未渲染第二首歌曲名称")
	}
	if !tester.HasText("网易云音乐") {
		t.Error("未渲染歌曲源标签 网易云音乐")
	}
	if !tester.HasText("QQ音乐") {
		t.Error("未渲染歌曲源标签 QQ音乐")
	}

	// 测试播放器常驻栏渲染
	app.currentSong = &mockSongs[0]
	tester.Frame()

	if !tester.HasText("播放") {
		t.Error("未渲染播放器控制按钮 播放")
	}
}
