package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "视频压缩 - OrangBus",
		Width:     1180,
		Height:    780,
		MinWidth:  980,
		MinHeight: 680,
		Frameless: true, // 无边框设计，顶部自定义标题栏（可拖拽）
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true, // 必须开启才能用 runtime.OnFileDrop 拿到拖入文件的真实路径
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 246, G: 248, B: 252, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.BeforeClose,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			BackdropType:         windows.Mica,
			Theme:                windows.SystemDefault,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
