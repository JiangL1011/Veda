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
	external := newExternalOpenCoordinator(svc)
	startupTargets := openTargetsFromArgs(os.Args, "")

	app := application.New(application.Options{
		Name:        "Veda",
		Description: "A WYSIWYG markdown editor",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: svc.mediaHandler,
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
