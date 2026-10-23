package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	svc := newAppService()
	// 索引检查必须早于任何工作区/会话读取。旧版本没有 file-index.json 时，
	// 后台会扫描整个 ~/.veda 并把已有配置改写成编号；前端会锁住界面等待。
	startDataMigration()
	external := newExternalOpenCoordinator(svc)
	startupTargets := openTargetsFromArgs(os.Args, "")

	app := application.New(application.Options{
		Name:        "Veda",
		Description: "A WYSIWYG markdown editor",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			Middleware: application.ChainMiddleware(
				svc.mediaHandler,
				svc.previewHandler,
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
		FileAssociations: associatedFileExtensions,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.jiangling.veda",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				external.openArgs(data.Args, data.WorkingDir, false)
			},
		},
	})
	svc.attach(app)
	external.attach(app)
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(event *application.ApplicationEvent) {
		external.openPath(event.Context().Filename(), "", true)
	})
	setupMenu(app, svc)

	restore := svc.shouldRestoreOnStartup()
	if len(startupTargets) > 0 {
		first := startupTargets[0]
		// 迁移期间 persist 会跳过写 session，窗口先带着启动目标显示出来；
		// 迁移结束后前端会补写这次打开的目标。
		_ = svc.persist(first)
		initial := openTargetWindow(app, svc, first)
		external.setInitialWindow(initial, false)
		if runtime.GOOS == "windows" {
			external.suppressFrameworkOpen(first.Path)
		}
		for _, target := range startupTargets[1:] {
			external.openTarget(target)
			if runtime.GOOS == "windows" {
				external.suppressFrameworkOpen(target.Path)
			}
		}
	} else {
		initial := createWindow(app, svc, restore)
		// macOS delivers a cold-start "open document" event after the app has
		// been created. Let that event claim this window until its runtime is
		// ready, even when normal startup would otherwise restore a session.
		external.setInitialWindow(initial, true)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
