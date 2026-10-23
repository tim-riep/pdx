package render

import (
	"fmt"
	"strconv"

	"github.com/tim-riep/pdx/areatree"
	"github.com/tim-riep/pdx/image"
	"github.com/tim-riep/pdx/pdf"
)

// imageSet embeds each image once as an XObject, on first use.
type imageSet struct {
	r    *renderer
	refs map[areatree.ImageID]pdf.Ref
}

func newImageSet(r *renderer) *imageSet {
	return &imageSet{r: r, refs: map[areatree.ImageID]pdf.Ref{}}
}

func (is *imageSet) ref(id areatree.ImageID) (pdf.Ref, error) {
	if ref, ok := is.refs[id]; ok {
		return ref, nil
	}
	if int(id) < 0 || int(id) >= len(is.r.doc.Images) {
		return pdf.Ref{}, fmt.Errorf("Bild %d ist im Dokument nicht deklariert", id)
	}
	im, ok := is.r.doc.Images[id].Source.(*image.Image)
	if !ok {
		return pdf.Ref{}, fmt.Errorf("Bild %d hat keine ladbare Bildquelle", id)
	}
	ref, err := is.embed(im)
	if err != nil {
		return pdf.Ref{}, fmt.Errorf("Bild %s: %w", im.Key(), err)
	}
	is.refs[id] = ref
	return ref, nil
}

func colorSpaceName(s image.ColorSpace) pdf.Name {
	switch s {
	case image.Gray:
		return "DeviceGray"
	case image.CMYK:
		return "DeviceCMYK"
	}
	return "DeviceRGB"
}

func (is *imageSet) embed(im *image.Image) (pdf.Ref, error) {
	d := is.r.d
	if im.Space == image.CMYK {
		// With the sRGB output intent, DeviceCMYK would not be allowed under PDF/A
		// (ISO 19005-3 6.2.4.3). CMYK output follows with the CMYK profiles.
		return pdf.Ref{}, fmt.Errorf("CMYK-Bilder werden in Phase 0 nicht unterstützt")
	}
	dict := pdf.D(
		"Type", pdf.Name("XObject"),
		"Subtype", pdf.Name("Image"),
		"Width", pdf.Int(im.Width),
		"Height", pdf.Int(im.Height),
		"ColorSpace", colorSpaceName(im.Space),
		"BitsPerComponent", pdf.Int(im.BitsPerComponent),
	)
	switch im.Format {
	case image.JPEG:
		dict.Set("Filter", pdf.Name("DCTDecode"))
		return d.Add(pdf.NewStream(dict, im.Data)), nil
	case image.PNG:
		if im.Alpha != nil {
			if err := d.Use(pdf.Transparency); err != nil {
				return pdf.Ref{}, fmt.Errorf("Bild mit Alphakanal: %w", err)
			}
			smask := d.Add(pdf.NewFlateStream(pdf.D(
				"Type", pdf.Name("XObject"),
				"Subtype", pdf.Name("Image"),
				"Width", pdf.Int(im.Width),
				"Height", pdf.Int(im.Height),
				"ColorSpace", pdf.Name("DeviceGray"),
				"BitsPerComponent", pdf.Int(8),
			), im.Alpha))
			dict.Set("SMask", smask)
		}
		return d.Add(pdf.NewFlateStream(dict, im.Data)), nil
	}
	return pdf.Ref{}, fmt.Errorf("unbekanntes Bildformat")
}

// image draws an image scaled to its rectangle.
func (pc *pageCtx) image(a *areatree.Image) error {
	ref, err := pc.r.images.ref(a.Image)
	if err != nil {
		return err
	}
	name := pdf.Name("Im" + strconv.Itoa(int(a.Image)))
	pc.res.xobjects.Set(name, ref)
	pc.c.Save()
	pc.c.Transform(a.W.Pt(), 0, 0, a.H.Pt(), pc.x(a.X), pc.y(a.Y+a.H))
	pc.c.XObject(name)
	pc.c.Restore()
	return nil
}
