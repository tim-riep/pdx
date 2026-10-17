// Package app enthält die Services des Designers. Sie kennen Wails nicht,
// sondern sprechen die Oberfläche nur über Emitter und Dialogs an. So laufen
// alle Tests ohne cgo, und dieselben Services dienen der nativen App und dem
// Server-Build für die Ende-zu-Ende-Tests.
package app

import (
	"sync"

	"github.com/tim-riep/pdx"
)

// Emitter schickt Ereignisse an die Oberfläche.
type Emitter interface {
	Emit(name string, data any)
}

// Filter beschränkt einen Dateidialog, etwa auf "*.zip".
type Filter struct {
	Name    string
	Pattern string
}

// Dialogs öffnet native Dateidialoge. Im Server-Build gibt es keine, dann
// liefern die Methoden einen Fehler.
type Dialogs interface {
	OpenFile(title string, filters []Filter, dirs bool) (string, error)
	SaveFile(title, name string, filters []Filter) (string, error)
}

// Ereignisse an die Oberfläche.
const (
	EventTemplateChanged = "template:changed"
	EventRenderPages     = "render:pages"
	EventProblems        = "problems:changed"
	EventProject         = "project:changed"
)

// Info beschreibt die Anwendung für Titelleiste und „Über“.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Engine  string `json:"engine"`
}

// AppService liefert allgemeine Angaben zur Anwendung.
type AppService struct{}

// Info liefert Name und Versionen.
func (AppService) Info() Info {
	return Info{Name: "pdx Designer", Version: Version, Engine: pdx.Version}
}

// Version ist die Version des Designers.
const Version = "0.1.0-dev"

// Core hält den gemeinsamen Zustand aller Services: das geöffnete Projekt und
// die Anbindung an die Oberfläche. Die Services sind Sichten darauf, damit
// jede Methode der Bindungen beim richtigen Service liegt.
type Core struct {
	mu      sync.Mutex
	emit    Emitter
	dialogs Dialogs
}

// NewCore legt den Zustand an. emit und dialogs dürfen nil sein (Tests).
func NewCore(emit Emitter, dialogs Dialogs) *Core {
	if emit == nil {
		emit = nopEmitter{}
	}
	return &Core{emit: emit, dialogs: dialogs}
}

type nopEmitter struct{}

func (nopEmitter) Emit(string, any) {}
