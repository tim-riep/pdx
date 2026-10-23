package fopcompare

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Block is a block with lines and the position of its first and last line
// in points (origin top left). Only blocks with their own lines are
// recorded, because only they can be matched by their text.
type Block struct {
	Page      int
	FirstLine string
	Top       float64 // top edge of the first line
	Bottom    float64 // bottom edge of the last line
	Lines     int
}

// ATLine is a line from an area tree with the position of its baseline.
type ATLine struct {
	Page     int
	Text     string
	Baseline float64
	// X is the start of the first glyph, only for lines from pdx (HasX).
	X    float64
	HasX bool
}

// frame is an open area while reading FOP's area tree.
type frame struct {
	name    string
	top     float64 // top edge of the content in points
	cursor  float64 // next free position for children
	advance float64 // height by which the parent area advances
	block   *Block  // for blocks with lines
	line    *strings.Builder
	stacks  bool    // children are stacked on top of each other
	space   float64 // space-before of a line (half leading)
	base    float64 // baseline relative to the line top edge, -1 if unknown
	// inlineOff is the offset of an inline area to its parent area.
	inlineOff float64
	// positioned is set for absolutely or fixed positioned areas.
	// They are the reference for absolutely positioned descendants.
	positioned bool
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

// bapBefore returns the top border plus padding from bap="start end before after".
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

// ParseAreaTree reads FOP's area tree XML (fop -at) and returns the blocks.
func ParseAreaTree(data []byte) ([]Block, int, error) {
	b, _, n, err := ParseAreaTreeLines(data)
	return b, n, err
}

// ParseAreaTreeLines reads blocks and lines from FOP's area tree XML.
func ParseAreaTreeLines(data []byte) ([]Block, []ATLine, int, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*frame
	var blocks []Block
	var lines []ATLine
	page := 0
	var regionTop float64
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, 0, err
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
				// Table cells and similar areas are placed with top-offset
				// relative to the content top edge of the parent area.
				if off, ok := attrF(t.Attr, "top-offset"); ok && parent != nil {
					f.top = parent.top + off + bapBefore(t.Attr)
					f.cursor = f.top
					f.advance = 0
					f.stacks = stacking[name]
					if name == "block" {
						f.block = &Block{Page: page, Top: -1}
					}
					stack = append(stack, f)
					continue
				}
				switch attrS(t.Attr, "positioning") {
				case "fixed":
					tp, _ := attrF(t.Attr, "top-position")
					top = tp
					bpda = 0
				case "absolute":
					// With top-position (block-container) relative to the nearest
					// positioned ancestor, otherwise to the region (XSL 1.1
					// 7.6.1), without it (such as label and body of a list)
					// above the parent area.
					if tp, ok := attrF(t.Attr, "top-position"); ok {
						top = regionTop + tp
						for i := len(stack) - 1; i >= 0; i-- {
							if stack[i].positioned {
								top = stack[i].top + tp
								break
							}
						}
					} else if parent != nil {
						top = parent.top
					}
					bpda = 0
				}
				if _, ok := attrF(t.Attr, "top-position"); ok {
					f.positioned = attrS(t.Attr, "positioning") != ""
				}
				sb, _ := attrF(t.Attr, "space-before")
				f.advance = bpda
				f.stacks = stacking[name]
				if name == "lineArea" {
					f.top = top
					f.line = &strings.Builder{}
					f.space = sb
					f.base = -1
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
			if name == "text" || name == "inlineparent" || name == "viewport" || name == "inlineblockparent" {
				off, _ := attrF(t.Attr, "offset")
				f.inlineOff = off
				// The first inline area with baseline determines the baseline.
				// Offsets of enclosing inline areas (such as a leader) are
				// included.
				if bl, ok := attrF(t.Attr, "baseline"); ok {
					outer := 0.0
					for i := len(stack) - 1; i >= 0; i-- {
						if stack[i].line != nil {
							if stack[i].base < 0 {
								stack[i].base = stack[i].space + outer + off + bl
							}
							break
						}
						outer += stack[i].inlineOff
					}
				}
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
				if f.base >= 0 {
					lines = append(lines, ATLine{Page: page, Text: strings.Join(strings.Fields(f.line.String()), " "), Baseline: f.top + f.base})
				}
				// Assign the line to the nearest block.
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
	return blocks, lines, page, nil
}
