package render

import (
	"fmt"

	"github.com/tim-riep/pdx/areatree"
)

// imageSet verwaltet die Bild-XObjects. Die Einbettung folgt im Schritt Bilder.
type imageSet struct{ r *renderer }

func newImageSet(r *renderer) *imageSet { return &imageSet{r: r} }

func (pc *pageCtx) image(im *areatree.Image) error {
	return fmt.Errorf("Bilder werden noch nicht unterstützt")
}
