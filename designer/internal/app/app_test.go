package app

import (
	"testing"

	"github.com/tim-riep/pdx"
)

func TestInfo(t *testing.T) {
	i := AppService{}.Info()
	if i.Version != Version || i.Engine != pdx.Version || i.Name == "" {
		t.Errorf("Info: %+v", i)
	}
}
