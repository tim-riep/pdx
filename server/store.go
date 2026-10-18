package server

import (
	"context"
	"errors"
	"sync"

	"github.com/tim-riep/pdx/template/pkg"
)

// ErrNotFound meldet eine fehlende Vorlage oder Version.
var ErrNotFound = errors.New("nicht gefunden")

// Store ist die Vorlagenablage.
type Store interface {
	// Get liefert eine Version einer Vorlage, leer bedeutet die aktive.
	// Geliefert werden die Rohdaten des Pakets und die Version.
	Get(ctx context.Context, id, version string) ([]byte, string, error)
}

// MemStore hält Vorlagen im Speicher, für Tests.
type MemStore struct {
	mu   sync.Mutex
	data map[string]map[string][]byte
	act  map[string]string
}

// Put legt eine Version ab und aktiviert sie.
func (m *MemStore) Put(id, version string, p *pkg.Package) error {
	b, err := p.Bytes()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data, m.act = map[string]map[string][]byte{}, map[string]string{}
	}
	if m.data[id] == nil {
		m.data[id] = map[string][]byte{}
	}
	m.data[id][version] = b
	m.act[id] = version
	return nil
}

func (m *MemStore) Get(_ context.Context, id, version string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if version == "" {
		version = m.act[id]
	}
	b, ok := m.data[id][version]
	if !ok {
		return nil, "", ErrNotFound
	}
	return b, version, nil
}
