// Package app enthält die Services des Designers. Sie kennen Wails nicht,
// sondern sprechen die Oberfläche nur über Emitter und Dialogs an. So laufen
// alle Tests ohne cgo, und dieselben Services dienen der nativen App und dem
// Server-Build für die Ende-zu-Ende-Tests.
package app

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/designer/internal/model"
	"github.com/tim-riep/pdx/designer/internal/pipeline"
	"github.com/tim-riep/pdx/designer/notices"
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

// ErrNoDialogs meldet, dass es keine nativen Dateidialoge gibt, etwa im
// Server-Build. Die Oberfläche fragt dann selbst nach dem Pfad.
var ErrNoDialogs = errors.New("Dateidialoge sind hier nicht verfügbar")

// Dialogs öffnet native Dateidialoge. Im Server-Build gibt es keine, dann
// liefern die Methoden einen Fehler.
type Dialogs interface {
	OpenFile(title string, filters []Filter, dirs bool) (string, error)
	SaveFile(title, name string, filters []Filter) (string, error)
	// Show öffnet eine Datei mit dem Programm des Systems, etwa ein PDF.
	Show(path string) error
}

// Ereignisse an die Oberfläche.
const (
	EventTemplateChanged = "template:changed"
	EventRenderPages     = "render:pages"
	EventProblems        = "problems:changed"
	EventProject         = "project:changed"
	// EventCloseRequest fragt die Oberfläche, ob ungespeicherte Änderungen
	// gesichert werden sollen, bevor das Fenster schließt.
	EventCloseRequest = "app:close-request"
)

// Info beschreibt die Anwendung für Titelleiste und „Über“.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Engine  string `json:"engine"`
}

// AppService liefert allgemeine Angaben zur Anwendung und steuert das
// Beenden.
type AppService struct {
	c *Core
	// Close schließt das Fenster ohne weitere Rückfrage. Gesetzt von main.
	Close func()
}

// NewAppService liefert den Service. close darf nil sein.
func NewAppService(c *Core, close func()) *AppService { return &AppService{c: c, Close: close} }

// ConfirmClose schließt das Fenster nach der Rückfrage der Oberfläche,
// auch mit ungespeicherten Änderungen.
func (a *AppService) ConfirmClose() {
	a.c.mu.Lock()
	a.c.closing = true
	a.c.mu.Unlock()
	if a.Close != nil {
		a.Close()
	}
}

// PickFile fragt nach einer Datei, etwa einem Bild für den Baustein Bild.
// pattern ist ein Filter wie "*.png;*.jpg".
func (a *AppService) PickFile(title, name, pattern string) (string, error) {
	if a.c.dialogs == nil {
		return "", ErrNoDialogs
	}
	return a.c.dialogs.OpenFile(title, []Filter{{Name: name, Pattern: pattern}}, false)
}

// Notices liefert die Drittanbieterhinweise mit allen Lizenztexten.
func (*AppService) Notices() string { return notices.Text }

// Licenses beschreibt die Lizenzen der App für den Dialog Lizenzen.
type Licenses struct {
	// Name und License gelten für den Designer selbst (Apache License 2.0).
	Name    string `json:"name"`
	License string `json:"license"`
	Notice  string `json:"notice"`
	// Components sind alle enthaltenen Fremdbestandteile mit Lizenztext.
	Components []pdx.Component `json:"components"`
	// Full sind alle Hinweise als ein Text, zum Kopieren oder Sichern.
	Full string `json:"full"`
}

// Licenses liefert die Lizenz des Designers und aller enthaltenen
// Fremdbestandteile.
func (*AppService) Licenses() Licenses {
	return Licenses{
		Name:       "pdx Designer",
		License:    pdx.License,
		Notice:     pdx.Notice,
		Components: pdx.Components(notices.Text),
		Full:       pdx.License + "\n\n" + notices.Text,
	}
}

// Info liefert Name und Versionen.
func (*AppService) Info() Info {
	return Info{Name: "pdx Designer", Version: Version, Engine: pdx.Version}
}

// Version ist die Version des Designers.
const Version = "0.2.0"

// Core hält den gemeinsamen Zustand aller Services: das geöffnete Projekt und
// die Anbindung an die Oberfläche. Die Services sind Sichten darauf, damit
// jede Methode der Bindungen beim richtigen Service liegt.
type Core struct {
	mu      sync.Mutex
	emit    Emitter
	dialogs Dialogs
	dataDir string

	doc     *model.Document
	session string // Kennung der automatischen Sicherung des Dokuments
	saver   *time.Timer
	closing bool // Rückfrage beim Schließen ist beantwortet
	r       renderer
	// view ist die zuletzt gemeldete Projektion, Grundlage der Patches.
	view   model.Snapshot
	schema schemaCache
	v      validator
	conf   conformance
	// AutosaveDelay ist die Wartezeit nach einer Änderung bis zur
	// automatischen Sicherung.
	AutosaveDelay time.Duration
}

// NewCore legt den Zustand an. dataDir nimmt Einstellungen und automatische
// Sicherungen auf. emit und dialogs dürfen nil sein (Tests).
func NewCore(emit Emitter, dialogs Dialogs, dataDir string) *Core {
	if emit == nil {
		emit = nopEmitter{}
	}
	return &Core{emit: emit, dialogs: dialogs, dataDir: dataDir, AutosaveDelay: 30 * time.Second,
		r: renderer{pipe: pipeline.New(), view: ViewDesign}}
}

// MayClose meldet, ob das Fenster ohne Rückfrage schließen darf. Sonst
// schickt es die Rückfrage an die Oberfläche.
func (c *Core) MayClose() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.doc == nil || !c.doc.Dirty() {
		return true
	}
	c.emit.Emit(EventCloseRequest, c.stateLocked())
	return false
}

// DefaultDataDir ist das Verzeichnis für Einstellungen im Benutzerprofil.
func DefaultDataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "pdx-designer")
}

type nopEmitter struct{}

func (nopEmitter) Emit(string, any) {}
