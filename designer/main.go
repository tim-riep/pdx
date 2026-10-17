// Befehl designer startet den pdx Designer. Nur diese Datei kennt Wails. Die
// Services liegen in internal/app und laufen ohne cgo.
//
// Mit dem Build-Tag server läuft der Designer als HTTP-Server ohne Fenster
// (Wails-Servermodus), etwa für die Ende-zu-Ende-Tests im Browser.
package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"time"

	"github.com/tim-riep/pdx/designer/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	smoke := flag.Bool("smoke", false, "starten, kurz warten und beenden (Test der Builds)")
	flag.Parse()

	var wails *application.App
	emit := emitter{app: func() *application.App { return wails }}
	core := app.NewCore(emit, dialogs{app: func() *application.App { return wails }})
	_ = core

	wails = application.New(application.Options{
		Name:        "pdx Designer",
		Description: "Dokumente gegen ein XSD gestalten",
		Services: []application.Service{
			application.NewService(&app.AppService{}),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "pdx Designer",
		Width:     1440,
		Height:    900,
		MinWidth:  960,
		MinHeight: 600,
		URL:       "/",
	})
	if *smoke {
		go func() {
			time.Sleep(3 * time.Second)
			wails.Quit()
		}()
	}
	if err := wails.Run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

// emitter leitet Ereignisse der Services an Wails weiter.
type emitter struct{ app func() *application.App }

func (e emitter) Emit(name string, data any) {
	if a := e.app(); a != nil {
		a.Event.Emit(name, data)
	}
}

// dialogs öffnet native Dateidialoge über Wails.
type dialogs struct{ app func() *application.App }

func (d dialogs) OpenFile(title string, filters []app.Filter, dirs bool) (string, error) {
	dlg := d.app().Dialog.OpenFile().SetTitle(title).CanChooseFiles(!dirs).CanChooseDirectories(dirs)
	for _, f := range filters {
		dlg = dlg.AddFilter(f.Name, f.Pattern)
	}
	return dlg.PromptForSingleSelection()
}

func (d dialogs) SaveFile(title, name string, filters []app.Filter) (string, error) {
	dlg := d.app().Dialog.SaveFile().SetFilename(name)
	for _, f := range filters {
		dlg = dlg.AddFilter(f.Name, f.Pattern)
	}
	_ = title
	return dlg.PromptForSingleSelection()
}
