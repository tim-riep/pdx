package fopcompare

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Block ist ein Block mit Zeilen und der Lage seiner ersten und letzten
// Zeile in Punkt (Ursprung oben links). Nur Blöcke mit eigenen Zeilen werden
// erfasst, denn nur sie lassen sich über ihren Text zuordnen.
type Block struct {
	Page      int
	FirstLine string
	Top       float64 // Oberkante der ersten Zeile
	Bottom    float64 // Unterkante der letzten Zeile
	Lines     int
}

// frame ist ein offener Bereich beim Lesen von FOPs Area Tree.
type frame struct {
	name    string
	top     float64 // Oberkante des Inhalts in Punkt
	cursor  float64 // nächste freie Position für Kinder
	advance float64 // Höhe, um die der Elternbereich weiterrückt
	block   *Block  // bei Blöcken mit Zeilen
	line    *strings.Builder
	stacks  bool // Kinder werden übereinander gestapelt
}

func attrF(a []xml.Attr, name string) (float64, bool) {
	for _, x := range a {
		if x.Name.Local == name {
			v, err := strconv.ParseFloat(x.Value, 64)
			return v / 1000, err == nil
		}
	}
	return 0, false
}

func attrS(a []xml.Attr, name string) string {
	for _, x := range a {
		if x.Name.Local == name {
			return x.Value
		}
	}
	return ""
}

// bapBefore liefert Rahmen plus Padding oben aus bap="start end before after".
func bapBefore(a []xml.Attr) float64 {
	parts := strings.Fields(attrS(a, "bap"))
	if len(parts) != 4 {
		return 0
	}
	v, _ := strconv.ParseFloat(parts[2], 64)
	return v / 1000
}

var stacking = map[string]bool{
	"regionBody": true, "regionBefore": true, "regionAfter": true, "regionStart": true, "regionEnd": true,
	"mainReference": true, "span": true, "flow": true, "block": true, "lineArea": false,
	"beforeFloat": true, "footnote": true,
}

// ParseAreaTree liest FOPs Area-Tree-XML (fop -at) und liefert die Blöcke.
func ParseAreaTree(data []byte) ([]Block, int, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*frame
	var blocks []Block
	page := 0
	var regionTop float64
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			var parent *frame
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			f := &frame{name: name}
			switch name {
			case "pageViewport":
				page++
			case "regionViewport":
				r := strings.Fields(attrS(t.Attr, "rect"))
				if len(r) == 4 {
					y, _ := strconv.ParseFloat(r[1], 64)
					regionTop = y / 1000
				}
			}
			bpda, ok := attrF(t.Attr, "bpda")
			if !ok {
				bpda, _ = attrF(t.Attr, "bpd")
			}
			if stacking[name] || name == "lineArea" {
				top := regionTop
				if parent != nil && parent.stacks {
					top = parent.cursor
				}
				switch attrS(t.Attr, "positioning") {
				case "fixed":
					tp, _ := attrF(t.Attr, "top-position")
					top = tp
					bpda = 0
				case "absolute":
					tp, _ := attrF(t.Attr, "top-position")
					top = regionTop + tp
					bpda = 0
				}
				sb, _ := attrF(t.Attr, "space-before")
				f.advance = bpda
				f.stacks = stacking[name]
				if name == "lineArea" {
					f.top = top
					f.line = &strings.Builder{}
				} else {
					f.top = top + sb + bapBefore(t.Attr)
					if attrS(t.Attr, "positioning") != "" {
						f.top = top + bapBefore(t.Attr)
					}
				}
				f.cursor = f.top
			}
			if name == "block" {
				f.block = &Block{Page: page, Top: -1}
			}
			stack = append(stack, f)
		case xml.CharData:
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].line != nil {
					stack[i].line.Write(t)
					break
				}
			}
		case xml.EndElement:
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			var parent *frame
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if f.line != nil {
				// Zeile dem nächsten Block zuordnen.
				for i := len(stack) - 1; i >= 0; i-- {
					if b := stack[i].block; b != nil {
						text := strings.Join(strings.Fields(f.line.String()), " ")
						if b.Lines == 0 {
							b.FirstLine, b.Top = text, f.top
						}
						b.Lines++
						b.Bottom = f.top + f.advance
						break
					}
				}
			}
			if f.block != nil && f.block.Lines > 0 {
				blocks = append(blocks, *f.block)
			}
			if parent != nil && parent.stacks && (stacking[f.name] || f.name == "lineArea") {
				parent.cursor += f.advance
			}
		}
	}
	return blocks, page, nil
}
