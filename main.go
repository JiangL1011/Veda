package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"veda/internal/app"
	"veda/internal/store"
	"veda/internal/version"
)

//go:embed all:frontend/dist
var assets embed.FS

// buildConfigYAML 提供版本号的兜底来源：build/config.yml 的 info.version。
// 发布流水线会先把这个字段改写成 tag 里的版本号，再执行 go build，所以嵌入的
// 就是本次构建的真实版本。
//
//go:embed build/config.yml
var buildConfigYAML []byte

// appVersion 由打包脚本通过 -ldflags "-X main.appVersion=x.y.z" 注入，
// 为空时回退到 build/config.yml。
var appVersion string

func init() {
	version.Init(appVersion, buildConfigYAML)
}

func main() {
	svc := app.New(version.Current)
	// 索引检查必须早于任何工作区/会话读取。旧版本没有 file-index.json 时，
	// 后台会扫描整个 ~/.veda 并把已有配置改写成编号；前端会锁住界面等待。
	store.StartDataMigration()
	external := app.NewExternalOpenCoordinator(svc)
	startupTargets := app.OpenTargetsFromArgs(os.Args, "")

	wailsApp := application.New(application.Options{
		Name:        "Veda",
		Description: "A WYSIWYG markdown editor",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			Middleware: application.ChainMiddleware(
				app.MediaHandler(svc),
				app.PreviewHandler(svc),
			),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: false,
		},
		FileAssociations: app.AssociatedFileExtensions,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.jiangling.veda",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				external.OpenArgs(data.Args, data.WorkingDir, false)
			},
		},
	})
	app.Attach(svc, wailsApp)
	external.Attach(wailsApp)
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(event *application.ApplicationEvent) {
		external.OpenPath(event.Context().Filename(), "", true)
	})
	app.SetupMenu(wailsApp, svc)

	restore := app.ShouldRestoreOnStartup(svc)
	if len(startupTargets) > 0 {
		first := startupTargets[0]
		// 迁移期间 persist 会跳过写 session，窗口先带着启动目标显示出来；
		// 迁移结束后前端会补写这次打开的目标。
		_ = app.Persist(svc, first)
		initial := app.OpenTargetWindow(wailsApp, svc, first)
		external.SetInitialWindow(initial, false)
		if runtime.GOOS == "windows" {
			external.SuppressFrameworkOpen(first.Path)
		}
		for _, target := range startupTargets[1:] {
			external.OpenTarget(target)
			if runtime.GOOS == "windows" {
				external.SuppressFrameworkOpen(target.Path)
			}
		}
	} else {
		initial := app.CreateWindow(wailsApp, svc, restore)
		// macOS delivers a cold-start "open document" event after the app has
		// been created. Let that event claim this window until its runtime is
		// ready, even when normal startup would otherwise restore a session.
		external.SetInitialWindow(initial, true)
	}

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
