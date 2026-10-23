// Package app contains the services of the designer. They do not know Wails
// and talk to the UI only through Emitter and Dialogs. This way all tests
// run without cgo, and the same services serve the native app and the
// server build for the end-to-end tests.
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

// Emitter sends events to the UI.
type Emitter interface {
	Emit(name string, data any)
}

// Filter restricts a file dialog, for example to "*.zip".
type Filter struct {
	Name    string
	Pattern string
}

// ErrNoDialogs reports that there are no native file dialogs, for example in
// the server build. The UI then asks for the path itself.
var ErrNoDialogs = errors.New("Dateidialoge sind hier nicht verfügbar")

// Dialogs opens native file dialogs. The server build has none, then the
// methods return an error.
type Dialogs interface {
	OpenFile(title string, filters []Filter, dirs bool) (string, error)
	SaveFile(title, name string, filters []Filter) (string, error)
	// Show opens a file with the system's default program, for example a PDF.
	Show(path string) error
}

// Events sent to the UI.
const (
	EventTemplateChanged = "template:changed"
	EventRenderPages     = "render:pages"
	EventProblems        = "problems:changed"
	EventProject         = "project:changed"
	// EventCloseRequest asks the UI whether unsaved changes should be
	// saved before the window closes.
	EventCloseRequest = "app:close-request"
)

// Info describes the application for the title bar and "About".
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Engine  string `json:"engine"`
}

// AppService returns general information about the application and controls
// quitting.
type AppService struct {
	c *Core
	// Close closes the window without asking again. Set by main.
	Close func()
}

// NewAppService returns the service. close may be nil.
func NewAppService(c *Core, close func()) *AppService { return &AppService{c: c, Close: close} }

// ConfirmClose closes the window after the UI has asked the user,
// even with unsaved changes.
func (a *AppService) ConfirmClose() {
	a.c.mu.Lock()
	a.c.closing = true
	a.c.mu.Unlock()
	if a.Close != nil {
		a.Close()
	}
}

// PickFile asks for a file, for example an image for the Image building block.
// pattern is a filter such as "*.png;*.jpg".
func (a *AppService) PickFile(title, name, pattern string) (string, error) {
	if a.c.dialogs == nil {
		return "", ErrNoDialogs
	}
	return a.c.dialogs.OpenFile(title, []Filter{{Name: name, Pattern: pattern}}, false)
}

// Notices returns the third-party notices with all license texts.
func (*AppService) Notices() string { return notices.Text }

// Licenses describes the licenses of the app for the Licenses dialog.
type Licenses struct {
	// Name and License apply to the designer itself (Apache License 2.0).
	Name    string `json:"name"`
	License string `json:"license"`
	Notice  string `json:"notice"`
	// Components are all included third-party components with license text.
	Components []pdx.Component `json:"components"`
	// Full is all notices as one text, for copying or saving.
	Full string `json:"full"`
}

// Licenses returns the license of the designer and of all included
// third-party components.
func (*AppService) Licenses() Licenses {
	return Licenses{
		Name:       "pdx Designer",
		License:    pdx.License,
		Notice:     pdx.Notice,
		Components: pdx.Components(notices.Text),
		Full:       pdx.License + "\n\n" + notices.Text,
	}
}

// Info returns the name and versions.
func (*AppService) Info() Info {
	return Info{Name: "pdx Designer", Version: Version, Engine: pdx.Version}
}

// Version is the version of the designer.
const Version = "0.3.1"

// Core holds the shared state of all services: the open project and the
// connection to the UI. The services are views on it, so that every
// method of the bindings belongs to the right service.
type Core struct {
	mu      sync.Mutex
	emit    Emitter
	dialogs Dialogs
	dataDir string

	doc     *model.Document
	session string // ID of the document's automatic backup
	saver   *time.Timer
	closing bool // question on closing has been answered
	r       renderer
	// view is the last reported projection, the basis of the patches.
	view   model.Snapshot
	schema schemaCache
	v      validator
	conf   conformance
	// AutosaveDelay is the wait time after a change until the
	// automatic backup.
	AutosaveDelay time.Duration
}

// NewCore creates the state. dataDir holds settings and automatic
// backups. emit and dialogs may be nil (tests).
func NewCore(emit Emitter, dialogs Dialogs, dataDir string) *Core {
	if emit == nil {
		emit = nopEmitter{}
	}
	return &Core{emit: emit, dialogs: dialogs, dataDir: dataDir, AutosaveDelay: 30 * time.Second,
		r: renderer{pipe: pipeline.New(), view: ViewDesign}}
}

// MayClose reports whether the window may close without asking. Otherwise
// it sends the question to the UI.
func (c *Core) MayClose() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.doc == nil || !c.doc.Dirty() {
		return true
	}
	c.emit.Emit(EventCloseRequest, c.stateLocked())
	return false
}

// DefaultDataDir is the directory for settings in the user profile.
func DefaultDataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "pdx-designer")
}

type nopEmitter struct{}

func (nopEmitter) Emit(string, any) {}
