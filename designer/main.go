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
	"strings"
	"time"

	"github.com/tim-riep/pdx/designer/internal/app"
	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

// Ereignisse mit ihren Datentypen, damit die Bindungen sie typisiert kennen.
func init() {
	application.RegisterEvent[app.ProjectState](app.EventProject)
	application.RegisterEvent[app.ProjectState](app.EventCloseRequest)
	application.RegisterEvent[model.Patch](app.EventTemplateChanged)
	application.RegisterEvent[app.Manifest](app.EventRenderPages)
	application.RegisterEvent[app.Report](app.EventProblems)
	application.RegisterEvent[app.Conformance](app.EventConformance)
}

func main() {
	smoke := flag.Bool("smoke", false, "starten, kurz warten und beenden (Test der Builds)")
	flag.Parse()

	var wails *application.App
	emit := emitter{app: func() *application.App { return wails }}
	// PDX_DESIGNER_DATA ersetzt das Datenverzeichnis, etwa in Tests.
	dataDir := os.Getenv("PDX_DESIGNER_DATA")
	if dataDir == "" {
		dataDir = app.DefaultDataDir()
	}
	core := app.NewCore(emit, dialogs{app: func() *application.App { return wails }}, dataDir)
	var window *application.WebviewWindow
	appService := app.NewAppService(core, func() {
		if window != nil {
			window.Close()
		}
	})

	wails = application.New(application.Options{
		Name:        "pdx Designer",
		Description: "Dokumente gegen ein XSD gestalten",
		Services: []application.Service{
			application.NewService(appService),
			application.NewService(app.NewProjectService(core)),
			application.NewService(app.NewRenderService(core)),
			application.NewService(app.NewTemplateService(core)),
			application.NewService(app.NewSchemaService(core)),
			application.NewService(app.NewValidationService(core)),
			application.NewService(app.NewConformanceService(core)),
			// Seiten, Schriften und Bilder des Canvas über HTTP.
			application.NewServiceWithOptions(app.NewAssets(core), application.ServiceOptions{Route: "/pdx"}),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		// Vor dem Beenden die anstehende automatische Sicherung schreiben.
		OnShutdown: core.Flush,
	})
	window = wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "pdx Designer",
		Width:     1440,
		Height:    900,
		MinWidth:  960,
		MinHeight: 600,
		URL:       "/",
	})
	if window != nil {
		// Ungespeicherte Änderungen: erst die Oberfläche fragen.
		window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			if !core.MayClose() {
				e.Cancel()
			}
		})
	}
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

// noDialogs übersetzt den Fehler fehlender Dialoge (Servermodus).
func noDialogs(path string, err error) (string, error) {
	if err != nil && strings.Contains(err.Error(), "not available") {
		return "", app.ErrNoDialogs
	}
	return path, err
}

func (d dialogs) OpenFile(title string, filters []app.Filter, dirs bool) (string, error) {
	dlg := d.app().Dialog.OpenFile().SetTitle(title).CanChooseFiles(!dirs).CanChooseDirectories(dirs)
	for _, f := range filters {
		dlg = dlg.AddFilter(f.Name, f.Pattern)
	}
	return noDialogs(dlg.PromptForSingleSelection())
}

func (d dialogs) SaveFile(title, name string, filters []app.Filter) (string, error) {
	dlg := d.app().Dialog.SaveFile().SetFilename(name)
	for _, f := range filters {
		dlg = dlg.AddFilter(f.Name, f.Pattern)
	}
	_ = title
	return noDialogs(dlg.PromptForSingleSelection())
}
