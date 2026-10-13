package main

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/guohuiyuan/go-music-dl/core"
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
		iconVolume, iconVolumeMute, iconFolder, iconDisc,
		iconBell, iconInfo, iconRadio, iconMinimize,
		iconMaximize, iconClose, iconHeart, iconListMusic,
		iconSliders, iconSun, iconMoon, iconQrCode,
		iconWinMinimize, iconWinMaximize, iconWinRestore, iconWinClose,
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
	tester := ui.NewTester(app.view, 1240, 800)

	if !tester.HasText("Music DL") {
		t.Error("未找到窗口顶栏标题 Music DL")
	}
	if !tester.HasText("搜索") {
		t.Error("未找到搜索导航项")
	}
	if !tester.HasText("已下载") {
		t.Error("未找到已下载导航项")
	}
	if !tester.HasText("设置") {
		t.Error("未找到设置导航项")
	}
	if !tester.HasText("开始探索") {
		t.Error("初始空搜索页面提示未显示")
	}
	if !tester.HasText("高解析度无损") {
		t.Error("侧边栏底部高解析度无损标识未显示")
	}
}

func TestNativeAppTabNavigation(t *testing.T) {
	app := newNativeApp()
	tester := ui.NewTester(app.view, 1240, 800)

	// 初始状态为 search
	if app.activeTab != "search" {
		t.Fatalf("预期初始 activeTab 为 search，实际为 %s", app.activeTab)
	}

	// 切换到已下载
	if err := tester.Click("已下载"); err != nil {
		t.Fatalf("点击已下载标签失败: %v", err)
	}
	tester.Frame()

	if app.activeTab != "downloads" {
		t.Fatalf("预期切换后 activeTab 为 downloads，实际为 %s", app.activeTab)
	}
	if !tester.HasText("无下载内容") {
		t.Error("切换到已下载后未显示'无下载内容'")
	}

	// 切换到设置
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置标签失败: %v", err)
	}
	tester.Frame()

	if app.activeTab != "settings" {
		t.Fatalf("预期切换后 activeTab 为 settings，实际为 %s", app.activeTab)
	}
	if !tester.HasText("下载与存储") {
		t.Error("切换到设置后未显示'下载与存储'")
	}
	if !tester.HasText("关于") {
		t.Error("切换到设置后未显示'关于'")
	}

	// 切换回搜索
	if err := tester.Click("搜索"); err != nil {
		t.Fatalf("点击搜索标签失败: %v", err)
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
	tester := ui.NewTester(app.view, 1240, 800)

	if !tester.HasText("共 2 首歌曲") {
		t.Error("未找到搜索结果数量展示")
	}
	if !tester.HasText("春日影 (Haruhikage)") {
		t.Error("未渲染第一首歌曲名称")
	}
	if !tester.HasText("碧天伴走") {
		t.Error("未渲染第二首歌曲名称")
	}
	if !tester.HasText("网易云") {
		t.Error("未渲染歌曲源标签 网易云")
	}
	if !tester.HasText("QQ 音乐") {
		t.Error("未渲染歌曲源标签 QQ 音乐")
	}
	if !tester.HasText("复制链接") {
		t.Error("未渲染复制链接按钮")
	}

	// 测试播放器常驻栏渲染
	app.currentSong = &mockSongs[0]
	tester.Frame()

	if !tester.HasText("播放") {
		t.Error("未渲染播放器控制按钮 播放")
	}
}

func TestPlatformWindowControls(t *testing.T) {
	app := newNativeApp()
	tester := ui.NewTester(app.view, 1240, 800)

	// 1. 验证默认自动模式
	if app.windowControlsStyle != WindowControlsAuto {
		t.Fatalf("预期初始窗口控制风格为 auto，实际为 %s", app.windowControlsStyle)
	}

	// 2. 切换到设置页测试风格切换
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置导航项失败: %v", err)
	}
	tester.Frame()

	if !tester.HasText("窗口控制风格") {
		t.Error("设置页未渲染'窗口控制风格'卡片")
	}

	// 3. 点击切换至 macOS (交通灯)
	if err := tester.Click("macOS (交通灯)"); err != nil {
		t.Fatalf("点击切换至 macOS 风格失败: %v", err)
	}
	tester.Frame()
	if app.windowControlsStyle != WindowControlsMacOS {
		t.Errorf("预期当前风格为 macos，实际为 %s", app.windowControlsStyle)
	}
	if app.effectiveWindowControlsStyle() != WindowControlsMacOS {
		t.Errorf("预期生效风格为 macos，实际为 %s", app.effectiveWindowControlsStyle())
	}

	// 4. 点击切换至 Windows (WinUI 3 标题栏)
	if err := tester.Click("Windows (WinUI 3 标题栏)"); err != nil {
		t.Fatalf("点击切换至 Windows 风格失败: %v", err)
	}
	tester.Frame()
	if app.windowControlsStyle != WindowControlsWindows {
		t.Errorf("预期当前风格为 windows，实际为 %s", app.windowControlsStyle)
	}
	if app.effectiveWindowControlsStyle() != WindowControlsWindows {
		t.Errorf("预期生效风格为 windows，实际为 %s", app.effectiveWindowControlsStyle())
	}

	// 5. 点击切换至 Linux GNOME
	if err := tester.Click("Linux GNOME"); err != nil {
		t.Fatalf("点击切换至 Linux GNOME 风格失败: %v", err)
	}
	tester.Frame()
	if app.windowControlsStyle != WindowControlsLinuxGNOME {
		t.Errorf("预期当前风格为 linux_gnome，实际为 %s", app.windowControlsStyle)
	}
	if app.effectiveWindowControlsStyle() != WindowControlsLinuxGNOME {
		t.Errorf("预期生效风格为 linux_gnome，实际为 %s", app.effectiveWindowControlsStyle())
	}

	// 6. 点击切换至 Linux KDE
	if err := tester.Click("Linux KDE"); err != nil {
		t.Fatalf("点击切换至 Linux KDE 风格失败: %v", err)
	}
	tester.Frame()
	if app.windowControlsStyle != WindowControlsLinuxKDE {
		t.Errorf("预期当前风格为 linux_kde，实际为 %s", app.windowControlsStyle)
	}
	if app.effectiveWindowControlsStyle() != WindowControlsLinuxKDE {
		t.Errorf("预期生效风格为 linux_kde，实际为 %s", app.effectiveWindowControlsStyle())
	}
}

func TestThemeModeSwitching(t *testing.T) {
	app := newNativeApp()
	tester := ui.NewTester(app.view, 1240, 800)

	// 初始状态默认为深色模式 (Pulse Dark)
	if app.themeMode != ThemeModeDark {
		t.Fatalf("预期初始主题为 dark，实际为 %s", app.themeMode)
	}
	if !app.isDark() {
		t.Fatal("预期 isDark() 为 true")
	}
	if app.colors().Background != ui.Hex("#0b0b0d") {
		t.Errorf("预期深色模式背景为 #0b0b0d，实际为 %v", app.colors().Background)
	}

	// 切换到设置页
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置导航项失败: %v", err)
	}
	tester.Frame()

	if !tester.HasText("外观与主题") {
		t.Error("设置页未渲染'外观与主题'卡片")
	}

	// 切换到浅色模式 (Pulse Light - overview-light.jpg)
	if err := tester.Click("浅色模式 (Pulse Light)"); err != nil {
		t.Fatalf("点击浅色模式切换失败: %v", err)
	}
	tester.Frame()

	if app.themeMode != ThemeModeLight {
		t.Fatalf("预期切换后主题为 light，实际为 %s", app.themeMode)
	}
	if app.isDark() {
		t.Fatal("预期 isDark() 为 false")
	}
	if app.colors().Background != ui.Hex("#f5f5f7") {
		t.Errorf("预期浅色模式背景为 #f5f5f7，实际为 %v", app.colors().Background)
	}
	if app.colors().Surface != ui.Hex("#ffffff") {
		t.Errorf("预期浅色模式卡片背景为 #ffffff，实际为 %v", app.colors().Surface)
	}

	// 切回深色模式 (Pulse Dark)
	if err := tester.Click("深色模式 (Pulse Dark)"); err != nil {
		t.Fatalf("点击深色模式切换失败: %v", err)
	}
	tester.Frame()

	if app.themeMode != ThemeModeDark {
		t.Fatalf("预期切回后主题为 dark，实际为 %s", app.themeMode)
	}
	if !app.isDark() {
		t.Fatal("预期 isDark() 切回为 true")
	}
	if app.colors().Background != ui.Hex("#0b0b0d") {
		t.Errorf("预期切回深色模式背景为 #0b0b0d，实际为 %v", app.colors().Background)
	}
}

func TestExpandedFeatures(t *testing.T) {
	app := newNativeApp()
	tester := ui.NewTester(app.view, 1240, 800)

	// 1. 验证搜索类型切换
	if app.searchType != "song" {
		t.Fatalf("预期默认搜索类型为 song，实际为 %s", app.searchType)
	}
	if err := tester.Click("歌单"); err != nil {
		t.Fatalf("切换到歌单类型失败: %v", err)
	}
	tester.Frame()
	if app.searchType != "playlist" {
		t.Fatalf("预期切换后搜索类型为 playlist，实际为 %s", app.searchType)
	}

	if err := tester.Click("专辑"); err != nil {
		t.Fatalf("切换到专辑类型失败: %v", err)
	}
	tester.Frame()
	if app.searchType != "album" {
		t.Fatalf("预期切换后搜索类型为 album，实际为 %s", app.searchType)
	}

	if err := tester.Click("单曲"); err != nil {
		t.Fatalf("切回单曲类型失败: %v", err)
	}
	tester.Frame()
	if app.searchType != "song" {
		t.Fatalf("预期切回后搜索类型为 song，实际为 %s", app.searchType)
	}

	// 2. 验证侧边栏导航到“推荐歌单”
	if err := tester.Click("推荐歌单"); err != nil {
		t.Fatalf("点击推荐歌单导航项失败: %v", err)
	}
	tester.Frame()
	if app.activeTab != "recommend" {
		t.Fatalf("预期 activeTab 为 recommend，实际为 %s", app.activeTab)
	}
	if !tester.HasText("平台精选:") {
		t.Error("未在推荐歌单页面找到'平台精选:'标识")
	}

	// 3. 验证侧边栏导航到“本地音乐”
	if err := tester.Click("本地音乐"); err != nil {
		t.Fatalf("点击本地音乐导航项失败: %v", err)
	}
	tester.Frame()
	if app.activeTab != "local_music" {
		t.Fatalf("预期 activeTab 为 local_music，实际为 %s", app.activeTab)
	}
	if !tester.HasText("重新扫描") {
		t.Error("未在本地音乐页面找到'重新扫描'按钮")
	}

	// 4. 验证歌单与专辑详情展开视图 (在搜索或推荐页面展开)
	if err := tester.Click("搜索"); err != nil {
		t.Fatalf("切回搜索页面失败: %v", err)
	}
	tester.Frame()

	mockPl := model.Playlist{
		ID:         "pl_001",
		Name:       "MyGO!!!!! 热门精选集",
		Creator:    "BangDream",
		Source:     "netease",
		TrackCount: 2,
	}
	mockSongs := []model.Song{
		{ID: "s1", Name: "春日影", Artist: "MyGO!!!!!", Source: "netease"},
		{ID: "s2", Name: "碧天伴走", Artist: "MyGO!!!!!", Source: "netease"},
	}
	app.selectedPlaylist = &mockPl
	app.playlistSongs = mockSongs
	tester.Frame()

	if !tester.HasText("MyGO!!!!! 热门精选集") {
		t.Error("歌单详情页未显示歌单名称")
	}
	if !tester.HasText("全部下载") {
		t.Error("歌单详情页未显示'全部下载'按钮")
	}
	if !tester.HasText("播放全部") {
		t.Error("歌单详情页未显示'播放全部'按钮")
	}
	if !tester.HasText("春日影") {
		t.Error("歌单详情页未列出曲目'春日影'")
	}

	// 点击返回退出歌单详情
	if err := tester.Click("返回"); err != nil {
		t.Fatalf("点击歌单返回按钮失败: %v", err)
	}
	tester.Frame()
	if app.selectedPlaylist != nil {
		t.Error("点击返回后 selectedPlaylist 应重置为 nil")
	}

	// 5. 验证播放器歌词面板展开
	app.currentSong = &mockSongs[0]
	app.currentLyric = "[00:01.00] 想要成为人类"
	tester.Frame()

	if err := tester.Click("歌词"); err != nil {
		t.Fatalf("点击底栏'歌词'按钮失败: %v", err)
	}
	tester.Frame()
	if !app.showLyric {
		t.Error("点击后 showLyric 预期为 true")
	}
	if !tester.HasText("春日影 — 实时歌词") {
		t.Error("实时歌词面板未显示当前歌曲歌词标题")
	}

	// 6. 验证“设置”中新增的 Cookie 授权与 WebDAV 云端同步卡片
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置导航项失败: %v", err)
	}
	tester.Frame()

	if !tester.HasText("平台 Cookie 授权") {
		t.Error("设置页未找到'平台 Cookie 授权'卡片")
	}
	if !tester.HasText("WebDAV 云端同步") {
		t.Error("设置页未找到'WebDAV 云端同步'卡片")
	}
}

func TestFontFamilySwitching(t *testing.T) {
	app := newNativeApp()
	app.fontFamily = ""
	tester := ui.NewTester(app.view, 1240, 800)

	// 1. 验证默认字体
	if app.fontFamily != "" {
		t.Fatalf("预期默认字体为空 (系统默认)，实际为 %s", app.fontFamily)
	}

	// 2. 切换到设置页
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置导航项失败: %v", err)
	}
	tester.Frame()

	if !tester.HasText("界面字体") {
		t.Error("设置页未找到'界面字体'卡片")
	}
	if !tester.HasText("精选字体") {
		t.Error("设置页未找到'精选字体'标签")
	}

	// 3. 切换至预设字体：微软雅黑
	if err := tester.Click("微软雅黑"); err != nil {
		t.Fatalf("点击切换至微软雅黑失败: %v", err)
	}
	tester.Frame()
	if app.fontFamily != "Microsoft YaHei" {
		t.Errorf("预期当前字体为 Microsoft YaHei，实际为 %s", app.fontFamily)
	}

	// 4. 切换至预设字体：Segoe UI
	if err := tester.Click("Segoe UI"); err != nil {
		t.Fatalf("点击切换至 Segoe UI 失败: %v", err)
	}
	tester.Frame()
	if app.fontFamily != "Segoe UI" {
		t.Errorf("预期当前字体为 Segoe UI，实际为 %s", app.fontFamily)
	}

	// 5. 切换至预设字体：等宽代码 (Consolas)
	if err := tester.Click("等宽代码"); err != nil {
		t.Fatalf("点击切换至等宽代码失败: %v", err)
	}
	tester.Frame()
	if app.fontFamily != "Consolas" {
		t.Errorf("预期当前字体为 Consolas，实际为 %s", app.fontFamily)
	}

	// 6. 测试应用自定义字体
	app.customFontInput = "Fira Code"
	tester.Frame()
	if err := tester.Click("应用字体"); err != nil {
		t.Fatalf("点击应用字体按钮失败: %v", err)
	}
	tester.Frame()
	if app.fontFamily != "Fira Code" {
		t.Errorf("预期当前字体为 Fira Code，实际为 %s", app.fontFamily)
	}

	// 7. 切回系统默认
	if err := tester.Click("系统默认"); err != nil {
		t.Fatalf("点击切回系统默认字体失败: %v", err)
	}
	tester.Frame()
	if app.fontFamily != "" {
		t.Errorf("预期当前字体切回为空 (系统默认)，实际为 %s", app.fontFamily)
	}
}

func TestSmartCookieManagement(t *testing.T) {
	// 1. 测试从字符串中智能识别音源平台 Cookie
	testCases := []struct {
		input       string
		expectedSrc string
		expectedOk  bool
	}{
		{"MUSIC_U=123456789abcdef; __csrf=abc", "netease", true},
		{"uin=10001; qm_keyst=Q_H_12345; pskey=abc", "qq", true},
		{"kg_mid=kugou123; token=token456", "kugou", true},
		{"SESSDATA=bili_sess_123; bili_jct=csrf456", "bilibili", true},
		{"random string without cookie keys", "", false},
		{"", "", false},
	}

	for _, tc := range testCases {
		src, cookie, ok := detectAndApplyCookie(tc.input)
		if ok != tc.expectedOk {
			t.Errorf("detectAndApplyCookie(%q) ok = %v, expected %v", tc.input, ok, tc.expectedOk)
		}
		if ok && src != tc.expectedSrc {
			t.Errorf("detectAndApplyCookie(%q) src = %q, expected %q", tc.input, src, tc.expectedSrc)
		}
		if ok && cookie != tc.input {
			t.Errorf("detectAndApplyCookie(%q) cookie = %q, expected %q", tc.input, cookie, tc.input)
		}
	}

	// 2. 验证设置页面的极简 Cookie 卡片呈现
	app := newNativeApp()
	app.cookieNetease = "MUSIC_U=test_key;"
	tester := ui.NewTester(app.view, 1240, 800)

	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置导航项失败: %v", err)
	}
	tester.Frame()

	if !tester.HasText("平台 Cookie 授权") {
		t.Error("设置页未找到'平台 Cookie 授权'卡片")
	}
	if !tester.HasText("从剪贴板智能识别导入") {
		t.Error("未找到'从剪贴板智能识别导入'便捷按钮")
	}
	if !tester.HasText("扫码授权") {
		t.Error("未找到'扫码授权'便捷按钮")
	}
	if !tester.HasText("从剪贴板填入") {
		t.Error("未找到'从剪贴板填入'按钮")
	}
	if !tester.HasText("手动编辑") {
		t.Error("未找到'手动编辑'展开按钮")
	}

	// 3. 验证网易云音乐当前为“已授权”，其他为“未授权”
	if !tester.HasText("已授权 (17 字符)") {
		t.Error("网易云音乐授权状态徽章未正确呈现'已授权 (17 字符)'")
	}
	if !tester.HasText("未授权") {
		t.Error("未配置平台状态徽章未呈现'未授权'")
	}
}

func TestQRLoginModal(t *testing.T) {
	// 1. 验证二维码生成能力
	// 测试 URL 生成二维码 Bitmap (网易云/酷狗/B站场景)
	urlSess := &model.QRLoginSession{
		Source: "netease",
		URL:    "https://music.163.com/login?codekey=mock_test_key",
	}
	urlBmp, err := generateQRCodeBitmap(urlSess)
	if err != nil || urlBmp == nil {
		t.Fatalf("URL 生成二维码失败: %v", err)
	}
	w, h := urlBmp.Size()
	if w <= 0 || h <= 0 {
		t.Fatalf("生成的二维码图片尺寸异常: w=%d, h=%d", w, h)
	}

	// 测试 Base64 生成二维码 Bitmap (QQ 音乐场景)
	b64Sess := &model.QRLoginSession{
		Source:   "qq",
		ImageURL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAG8AAABvAQAAAADKvqPNAAAACXBIWXMAAAsTAAALEwEAmpwYAAABbklEQVQ4jbXVsY3FIAwAUEcUdMkCSKxBx0phgZAskKxExxqRWIB0FOj7TJQ76RqT4u4rzSsSG2P7A/76wR+yAugrnos5ZwDfo0d9FXEFfWHtcpZqCimb6oJ6wxn0R6qXHE1aJcXq06NyUWBU7kmSIwCxPd/HZ0jlo9dPH34Ky7BS/ltRA+oMusspJoyYZdpKi8uTknEBhoAbHaRDeut0MX3oI6HPXepscTVqAeyx+lJHS4HSau5W4YgfI66IGOtoWiCeu03ZKl/o0T1WsGqRp49ih3b7PEc4HarFPEnynFBsJR1FTXdhWeJKlTd4lbS1JuRZXYHR6gMpq1ZJnqOledRZCrw7luciNYZ00GDeSXZoqcPp9sGHFwQ1BHFEBfIOxLHNwgg0NdVH6LHtHIoyBXB3VjxpfgdMmW4zPsPOkDYDhaAeAKhvSOelQdupYV5wCEAduNp7nFnSnswGZtN2Wpe0cwak7jpnKXr8t7+JL7ciZ/nHLB29AAAAAElFTkSuQmCC",
	}
	b64Bmp, err := generateQRCodeBitmap(b64Sess)
	if err != nil || b64Bmp == nil {
		t.Fatalf("Base64 解析二维码失败: %v", err)
	}
	bw, bh := b64Bmp.Size()
	if bw <= 0 || bh <= 0 {
		t.Fatalf("Base64 二维码尺寸异常: w=%d, h=%d", bw, bh)
	}

	// 2. 验证模态弹窗渲染与交互
	app := newNativeApp()
	app.qrModal = QRLoginModalState{
		Active:     true,
		Source:     "netease",
		SourceName: "网易云音乐",
		StatusText: "请使用 网易云音乐 手机 App 扫描",
		Bitmap:     urlBmp,
	}
	tester := ui.NewTester(app.view, 1240, 800)

	if !tester.HasText("网易云音乐 扫码授权") {
		t.Error("扫码模态弹窗未显示标题'网易云音乐 扫码授权'")
	}
	if !tester.HasText("请使用 网易云音乐 手机 App 扫描") {
		t.Error("扫码模态弹窗未显示引导文案")
	}
	if !tester.HasText("关闭") {
		t.Error("扫码模态弹窗未显示'关闭'按钮")
	}

	// 点击关闭按钮关闭弹窗
	if err := tester.Click("关闭"); err != nil {
		t.Fatalf("点击弹窗关闭按钮失败: %v", err)
	}
	tester.Frame()

	if app.qrModal.Active {
		t.Error("点击关闭后 qrModal.Active 应为 false")
	}
}

func TestQRLoginCreateAPI(t *testing.T) {
	sources := []string{"netease", "qq", "kugou", "bilibili"}
	for _, src := range sources {
		fn := core.GetQRLoginCreateFunc(src)
		if fn == nil {
			t.Errorf("source %s: no fn", src)
			continue
		}
		sess, err := fn()
		if err != nil {
			t.Logf("source %s 网络请求未完成 (可能受网络环境影响): %v", src, err)
			continue
		}
		if sess.Key == "" {
			t.Errorf("source %s: sess.Key is empty", src)
		}
		bmp, err := generateQRCodeBitmap(sess)
		if err != nil || bmp == nil {
			t.Errorf("source %s: generateQRCodeBitmap failed: %v", src, err)
		}
	}
}

func TestAuthStatusAndMyPlaylists(t *testing.T) {
	app := newNativeApp()
	app.cookieNetease = "MUSIC_U=valid_mock;"
	app.authStatus["netease"] = PlatformAuthStatus{
		Checked:       true,
		Valid:         true,
		Username:      "小明的音乐主页",
		IsVIP:         true,
		PlaylistCount: 12,
	}

	tester := ui.NewTester(app.view, 1240, 800)

	// 1. 验证在设置页展现的明确登录成功状态、用户昵称和 VIP 标识
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置导航项失败: %v", err)
	}
	tester.Frame()

	if !tester.HasText("已登录: 小明的音乐主页") {
		t.Error("未正确显示已登录账号昵称'已登录: 小明的音乐主页'")
	}
	if !tester.HasText("VIP") {
		t.Error("未正确显示 VIP 账号徽章")
	}
	if !tester.HasText("(12个歌单)") {
		t.Error("未正确显示歌单数量'(12个歌单)'")
	}
	if !tester.HasText("验证状态") {
		t.Error("未找到'验证状态'按钮")
	}

	// 2. 验证侧边栏新增的“我的歌单”导航项与页面
	if err := tester.Click("我的歌单"); err != nil {
		t.Fatalf("点击我的歌单导航项失败: %v", err)
	}
	tester.Frame()

	if app.activeTab != "my_playlists" {
		t.Fatalf("预期 activeTab 为 my_playlists，实际为 %s", app.activeTab)
	}
	if !tester.HasText("账号来源:") {
		t.Error("我的歌单页面未找到'账号来源:'选择器")
	}
	if !tester.HasText("同步歌单") {
		t.Error("我的歌单页面未找到'同步歌单'按钮")
	}
}

func TestQualityPlaybackAndLoadMore(t *testing.T) {
	app := newNativeApp()
	tester := ui.NewTester(app.view, 1240, 1600)

	// 1. 验证默认下载音质为 lossless
	if app.downloadQuality != "lossless" {
		t.Fatalf("预期初始下载音质为 lossless，实际为 %s", app.downloadQuality)
	}

	// 2. 在设置中验证音质选择器并切换为 exhigh (320Kbps)
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("点击设置导航项失败: %v", err)
	}
	tester.Frame()

	if !tester.HasText("下载音质") {
		t.Error("设置页未找到'下载音质'项")
	}
	if err := tester.Click("极高 (320Kbps)"); err != nil {
		t.Fatalf("点击切换为 320Kbps 音质失败: %v", err)
	}
	tester.Frame()
	if app.downloadQuality != "exhigh" {
		t.Fatalf("预期当前下载音质为 exhigh，实际为 %s", app.downloadQuality)
	}

	// 验证下载时是否成功注入该音质参数
	sampleSong := model.Song{ID: "q_001", Name: "测试单曲", Artist: "测试歌手", Source: "netease"}
	app.downloadSong(sampleSong)
	if len(app.downloads) == 0 {
		t.Fatal("预期下载队列非空")
	}
	if app.downloads[0].Song.Extra["level"] != "exhigh" {
		t.Errorf("预期下载曲目 Extra[level] 为 exhigh，实际为 %v", app.downloads[0].Song.Extra["level"])
	}

	// 3. 验证播放试听切歌与 Seek 控制
	mockList := []model.Song{
		{ID: "m1", Name: "第一首", Artist: "A", Duration: 200, Source: "netease"},
		{ID: "m2", Name: "第二首", Artist: "B", Duration: 240, Source: "netease"},
		{ID: "m3", Name: "第三首", Artist: "C", Duration: 180, Source: "netease"},
	}
	app.playSongFromList(mockList[0], mockList, 0)
	if app.playIndex != 0 || app.currentSong.ID != "m1" {
		t.Fatalf("初始播放索引预期为 0 (m1)，实际为 %d (%s)", app.playIndex, app.currentSong.ID)
	}

	// 下一首
	app.playNextSong()
	if app.playIndex != 1 || app.currentSong.ID != "m2" {
		t.Fatalf("点击下一首后索引预期为 1 (m2)，实际为 %d (%s)", app.playIndex, app.currentSong.ID)
	}

	// 上一首
	app.playPrevSong()
	if app.playIndex != 0 || app.currentSong.ID != "m1" {
		t.Fatalf("点击上一首后索引预期为 0 (m1)，实际为 %d (%s)", app.playIndex, app.currentSong.ID)
	}

	// 验证底栏控制器按键
	tester.Frame()
	if !tester.HasText("上一首") {
		t.Error("底栏未显示'上一首'控制按钮")
	}
	if !tester.HasText("下一首") {
		t.Error("底栏未显示'下一首'控制按钮")
	}
	if !tester.HasText("列表") {
		t.Error("底栏未显示循环模式切换按键")
	}

	// 4. 验证搜索结果底部“加载更多结果”按钮呈现
	if err := tester.Click("搜索"); err != nil {
		t.Fatalf("切回搜索页失败: %v", err)
	}
	app.searchKeyword = "春日影"
	app.searchResults = mockList
	tester.Frame()

	if !tester.HasText("加载更多音源 (已载入 3 首)") {
		t.Error("搜索结果列表底部未渲染'加载更多音源 (已载入 3 首)'按钮")
	}
}

func TestCustomPlaylistsBatchAndLRCSync(t *testing.T) {
	// 1. 验证 LRC 歌词解析
	rawLRC := `[ti:春日影]
[ar:MyGO!!!!!]
[00:01.50]想要成为人类
[00:05.80]迷茫也是一种前行
[01:23.00]在迷茫中前行吧`

	lines := parseLRC(rawLRC)
	if len(lines) != 3 {
		t.Fatalf("预期解析出 3 行时间戳歌词，实际为 %d", len(lines))
	}
	if lines[0].TimeSec != 1.5 || lines[0].Text != "想要成为人类" {
		t.Errorf("第一行歌词解析异常: %v", lines[0])
	}
	if lines[1].TimeSec != 5.8 || lines[1].Text != "迷茫也是一种前行" {
		t.Errorf("第二行歌词解析异常: %v", lines[1])
	}
	if lines[2].TimeSec != 83.0 || lines[2].Text != "在迷茫中前行吧" {
		t.Errorf("第三行歌词解析异常: %v", lines[2])
	}

	// 2. 验证本地自制歌单 (Collections)
	app := newNativeApp()
	testSong := model.Song{
		ID:     "c_song_01",
		Name:   "迷星叫",
		Artist: "MyGO!!!!!",
		Source: "netease",
	}

	// 新建自制歌单
	app.createCustomPlaylist("我的摇滚精选", "ACG 摇滚收藏")
	foundCol := false
	var colID string
	for _, col := range app.customPlaylists {
		if col.Name == "我的摇滚精选" {
			foundCol = true
			colID = col.ID
			break
		}
	}
	if !foundCol {
		t.Fatal("新建自制歌单未能成功保存到列表中")
	}

	// 添加歌曲到自制歌单
	added := app.addSongToCustomPlaylist(colID, testSong)
	if !added {
		t.Fatal("添加歌曲到自制歌单预期返回 true")
	}
	if !app.isSongInCustomPlaylist(colID, testSong.ID, testSong.Source) {
		t.Fatal("添加后 isSongInCustomPlaylist 预期为 true")
	}

	// 从自制歌单移除歌曲
	app.removeSongFromCustomPlaylist(colID, testSong.ID, testSong.Source)
	if app.isSongInCustomPlaylist(colID, testSong.ID, testSong.Source) {
		t.Fatal("移除后 isSongInCustomPlaylist 预期为 false")
	}

	// 3. 验证批量选择模式
	tester := ui.NewTester(app.view, 1240, 1200)
	app.searchKeyword = "MyGO"
	app.searchResults = []model.Song{
		{ID: "s1", Name: "春日影", Artist: "MyGO!!!!!", Source: "netease"},
		{ID: "s2", Name: "碧天伴走", Artist: "MyGO!!!!!", Source: "netease"},
	}
	tester.Frame()

	// 开启批量选择模式
	if err := tester.Click("批量"); err != nil {
		t.Fatalf("点击批量按钮失败: %v", err)
	}
	tester.Frame()
	if !app.batchMode {
		t.Fatal("预期开启批量模式，实际 batchMode 为 false")
	}

	// 全选歌曲
	if err := tester.Click("全选"); err != nil {
		t.Fatalf("点击全选按钮失败: %v", err)
	}
	tester.Frame()
	if len(app.selectedSongIDs) != 2 {
		t.Fatalf("全选后预期选中 2 首歌曲，实际为 %d", len(app.selectedSongIDs))
	}
	if !tester.HasText("已勾选 2 首歌曲") {
		t.Error("未找到浮动工具栏'已勾选 2 首歌曲'")
	}

	// 退出批量
	if err := tester.Click("退出批量"); err != nil {
		t.Fatalf("点击退出批量失败: %v", err)
	}
	tester.Frame()
	if app.batchMode {
		t.Fatal("点击退出后 batchMode 预期为 false")
	}
}

func TestDesktopSettingsPersistence(t *testing.T) {
	app := newNativeApp()
	app.themeMode = ThemeModeLight
	app.windowControlsStyle = WindowControlsLinuxGNOME
	app.fontFamily = "Segoe UI"
	app.downloadQuality = "exhigh"
	app.volume = 0.65
	app.notifEnabled = false
	app.filenameTemplate = "{artist} - {name}"

	app.saveDesktopSettings()

	app2 := newNativeApp()
	if app2.themeMode != ThemeModeLight {
		t.Errorf("themeMode 未正确持久化: %v", app2.themeMode)
	}
	if app2.windowControlsStyle != WindowControlsLinuxGNOME {
		t.Errorf("windowControlsStyle 未正确持久化: %v", app2.windowControlsStyle)
	}
	if app2.fontFamily != "Segoe UI" {
		t.Errorf("fontFamily 未正确持久化: %v", app2.fontFamily)
	}
	if app2.downloadQuality != "exhigh" {
		t.Errorf("downloadQuality 未正确持久化: %v", app2.downloadQuality)
	}
	if app2.volume != 0.65 {
		t.Errorf("volume 未正确持久化: %v", app2.volume)
	}
	if app2.notifEnabled != false {
		t.Errorf("notifEnabled 未正确持久化: %v", app2.notifEnabled)
	}

	// 恢复默认设置并落盘
	app2.themeMode = ThemeModeDark
	app2.windowControlsStyle = WindowControlsAuto
	app2.fontFamily = ""
	app2.downloadQuality = "lossless"
	app2.volume = 0.8
	app2.notifEnabled = true
	app2.saveDesktopSettings()
}
