package areatree

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Dump serialisiert den Area Tree in ein stabiles, zeilenorientiertes
// Textformat. Es dient Golden-File-Tests und der Fehlersuche. Gleiche Bäume
// ergeben gleiche Bytes.
func Dump(d *Document) []byte {
	var b bytes.Buffer
	b.WriteString("document")
	m := d.Meta
	for _, kv := range [][2]string{{"title", m.Title}, {"author", m.Author}, {"subject", m.Subject},
		{"keywords", m.Keywords}, {"creator", m.Creator}, {"lang", m.Lang}} {
		if kv[1] != "" {
			fmt.Fprintf(&b, " %s=%s", kv[0], strconv.Quote(kv[1]))
		}
	}
	b.WriteByte('\n')
	for i, f := range d.Fonts {
		name := "<nil>"
		if f.Source != nil {
			name = f.Source.PostScriptName()
		}
		fmt.Fprintf(&b, "  font %d %s\n", i, name)
	}
	for i, im := range d.Images {
		w, h := im.Source.PixelSize()
		fmt.Fprintf(&b, "  image %d %s %dx%d\n", i, im.Source.Key(), w, h)
	}
	for _, a := range d.Attachments {
		fmt.Fprintf(&b, "  attachment %s mime=%s rel=%s bytes=%d", strconv.Quote(a.Name), a.MIMEType, a.Relationship, len(a.Data))
		if a.Description != "" {
			fmt.Fprintf(&b, " desc=%s", strconv.Quote(a.Description))
		}
		b.WriteByte('\n')
	}
	ids := make([]string, 0, len(d.Dests))
	for id := range d.Dests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ds := d.Dests[id]
		fmt.Fprintf(&b, "  dest %s page=%d %s,%s\n", strconv.Quote(id), ds.Page, ds.At.X, ds.At.Y)
	}
	dumpBookmarks(&b, d.Bookmarks, 1)
	for _, p := range d.Pages {
		fmt.Fprintf(&b, "  page %d %sx%s", p.Number, p.Width, p.Height)
		if p.TrimBox != nil {
			fmt.Fprintf(&b, " trim=%s", rect(*p.TrimBox))
		}
		if p.BleedBox != nil {
			fmt.Fprintf(&b, " bleed=%s", rect(*p.BleedBox))
		}
		b.WriteByte('\n')
		dumpAreas(&b, p.Areas, 2)
	}
	return b.Bytes()
}

func dumpAreas(b *bytes.Buffer, areas []Area, depth int) {
	ind := strings.Repeat("  ", depth)
	for _, a := range areas {
		b.WriteString(ind)
		switch v := a.(type) {
		case *Container:
			fmt.Fprintf(b, "%s %s", v.Kind, rect(v.Rect))
			if v.Background != nil {
				fmt.Fprintf(b, " bg=%s", color(*v.Background))
			}
			sides := [4]string{"top", "right", "bottom", "left"}
			for i, br := range v.Border {
				if br.Visible() {
					fmt.Fprintf(b, " %s=%s", sides[i], border(br))
				}
			}
			if v.Role != "" {
				fmt.Fprintf(b, " role=%s", v.Role)
			}
			if v.Source != "" {
				fmt.Fprintf(b, " src=%s", v.Source)
			}
			b.WriteByte('\n')
			dumpAreas(b, v.Children, depth+1)
		case *GlyphRun:
			var txt strings.Builder
			for _, g := range v.Glyphs {
				txt.WriteString(g.Text)
			}
			fmt.Fprintf(b, "text %s,%s font=%d size=%s color=%s asc=%s desc=%s %s\n",
				v.Origin.X, v.Origin.Y, v.Font, v.Size, color(v.Color), v.Ascent, v.Descent, strconv.Quote(txt.String()))
			b.WriteString(ind)
			b.WriteString("  glyphs")
			for _, g := range v.Glyphs {
				fmt.Fprintf(b, " %d:%s", g.ID, g.Advance)
				if g.XOffset != 0 || g.YOffset != 0 {
					fmt.Fprintf(b, "@%s,%s", g.XOffset, g.YOffset)
				}
			}
			b.WriteByte('\n')
		case *Image:
			fmt.Fprintf(b, "image %s id=%d\n", rect(v.Rect), v.Image)
		case *Line:
			fmt.Fprintf(b, "line %s,%s %s,%s %s\n", v.From.X, v.From.Y, v.To.X, v.To.Y,
				border(Border{Width: v.Width, Style: v.Style, Color: v.Color}))
		case *Link:
			target := "dest=" + strconv.Quote(v.Dest)
			if v.URI != "" {
				target = "uri=" + strconv.Quote(v.URI)
			}
			fmt.Fprintf(b, "link %s %s\n", rect(v.Rect), target)
		case *Box:
			fmt.Fprintf(b, "box %s", rect(v.Rect))
			if v.Fill != nil {
				fmt.Fprintf(b, " fill=%s", color(*v.Fill))
			}
			if v.Stroke != nil {
				fmt.Fprintf(b, " stroke=%s", border(*v.Stroke))
			}
			b.WriteByte('\n')
		default:
			fmt.Fprintf(b, "unbekannt %T\n", a)
		}
	}
}

func dumpBookmarks(b *bytes.Buffer, bms []Bookmark, depth int) {
	for _, bm := range bms {
		fmt.Fprintf(b, "%sbookmark %s", strings.Repeat("  ", depth), strconv.Quote(bm.Title))
		if bm.Dest != "" {
			fmt.Fprintf(b, " dest=%s", strconv.Quote(bm.Dest))
		}
		if bm.URI != "" {
			fmt.Fprintf(b, " uri=%s", strconv.Quote(bm.URI))
		}
		if bm.Open {
			b.WriteString(" open")
		}
		b.WriteByte('\n')
		dumpBookmarks(b, bm.Children, depth+1)
	}
}

func rect(r Rect) string { return fmt.Sprintf("%s,%s %sx%s", r.X, r.Y, r.W, r.H) }

func border(br Border) string { return fmt.Sprintf("%s/%s/%s", br.Width, br.Style, color(br.Color)) }

func color(c Color) string {
	n := map[ColorSpace]int{Gray: 1, RGB: 3, CMYK: 4}[c.Space]
	name := map[ColorSpace]string{Gray: "gray", RGB: "rgb", CMYK: "cmyk"}[c.Space]
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strconv.FormatFloat(c.C[i], 'f', -1, 64)
	}
	return name + "(" + strings.Join(parts, ",") + ")"
}
