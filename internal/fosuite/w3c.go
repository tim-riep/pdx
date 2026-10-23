package fosuite

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tim-riep/go-xslt/engine"
	"github.com/tim-riep/pdx/fo/tree"
)

// Archives of the W3C XSL Test Suite (XSL 1.0, 28 August 2001). The W3C has
// not published a separate suite for XSL 1.1. The archives state no license,
// so they are only downloaded and not redistributed.
var w3cArchives = []struct{ URL, SHA256 string }{
	{"https://www.w3.org/Style/XSL/XSL-TS-NIST.zip", "2b34592c37fde21a7b848e549b6b4041e6fb758706e74835be369dd6ab27afb6"},
	{"https://www.w3.org/Style/XSL/XSL-TS-contrib.zip", "83660625533c299b691e8afaa8c68188b27e0e76e4e116635bbda4547f47c36b"},
}

// PrepareW3C downloads the W3C suite (if needed), verifies the checksums and
// stores the cases as FO under out/suites/w3c/fo. Cases outside the supported
// subset go to disabled with a reason.
func PrepareW3C(root string, disabled map[string]string) (int, error) {
	base := W3CDir(root)
	src := filepath.Join(base, "src")
	for _, a := range w3cArchives {
		file := filepath.Join(base, "dl", path.Base(a.URL))
		if err := download(a.URL, file, a.SHA256); err != nil {
			return 0, err
		}
		if err := unzip(file, src); err != nil {
			return 0, err
		}
	}
	for id, why := range disabled {
		if strings.HasPrefix(id, SuiteW3C+"/") && importReason(why) {
			delete(disabled, id)
		}
	}
	dst := caseDir(root, SuiteW3C)
	os.RemoveAll(dst)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	cases := map[string][]byte{}
	var errs []error
	add := func(name string, fo []byte, err error) {
		id := SuiteW3C + "/" + name
		if err != nil {
			disabled[id] = ReasonUnsupported + "XSLT: " + oneLine(err.Error())
			return
		}
		cases[name] = fo
	}
	// NIST: XML and stylesheet per test, linked in testsuite.html.
	nist := filepath.Join(src, "NIST")
	if err := copyResources(nist, dst); err != nil {
		errs = append(errs, err)
	}
	html, err := os.ReadFile(filepath.Join(nist, "testsuite.html"))
	if err != nil {
		return 0, err
	}
	for _, t := range nistTests(html) {
		fo, err := transformFile(filepath.Join(nist, t.xml), filepath.Join(nist, t.xsl))
		add("nist-"+t.name, fo, err)
	}
	// Contributions: FO files from the testsuite.xml of RenderX and Antenna
	// House and the FO files of the FOP contributions.
	for _, ct := range []struct{ dir, prefix string }{{"XEP", "xep-"}, {"XSLFormatter", "ah-"}} {
		dir := filepath.Join(src, "contrib", ct.dir)
		for _, t := range contribTests(filepath.Join(dir, "testsuite.xml")) {
			if !strings.HasSuffix(t, ".fo") {
				continue
			}
			fo, err := os.ReadFile(filepath.Join(dir, t))
			if err != nil {
				continue
			}
			if err := copyResources(filepath.Dir(filepath.Join(dir, t)), dst); err != nil {
				errs = append(errs, err)
			}
			add(ct.prefix+strings.TrimSuffix(path.Base(t), ".fo"), fo, nil)
		}
	}
	fopDir := filepath.Join(src, "contrib", "FOP")
	if err := copyResources(fopDir, dst); err != nil {
		errs = append(errs, err)
	}
	fos, _ := filepath.Glob(filepath.Join(fopDir, "*.fo"))
	for _, f := range fos {
		fo, err := os.ReadFile(f)
		add("fopc-"+strings.TrimSuffix(filepath.Base(f), ".fo"), fo, err)
	}
	n := 0
	for name, fo := range cases {
		fo = upgradeCR(fo)
		id := SuiteW3C + "/" + name
		if m := externalRef.FindSubmatch(fo); m != nil {
			ref := string(m[1])
			if !strings.HasPrefix(ref, "data:") && !strings.HasPrefix(ref, "http") && !strings.HasPrefix(ref, "#") {
				if _, err := os.Stat(filepath.Join(dst, ref)); err != nil {
					disabled[id] = ReasonResources + ref
					continue
				}
			}
		}
		if _, err := tree.Parse(bytes.NewReader(fo)); err != nil {
			disabled[id] = ReasonUnsupported + oneLine(err.Error())
			continue
		}
		if why := unsupported(string(fo)); why != "" {
			disabled[id] = ReasonUnsupported + why
			continue
		}
		if err := os.WriteFile(filepath.Join(dst, name+".fo"), fo, 0o644); err != nil {
			return n, err
		}
		n++
	}
	removeStaleExpect(root, SuiteW3C)
	if len(errs) > 0 {
		return n, errs[0]
	}
	return n, nil
}

var crRefs = regexp.MustCompile(`<fo:(page-sequence|single-page-master-reference|repeatable-page-master-reference|conditional-page-master-reference)\b[^>]*>`)

// upgradeCR converts cases from the XSL 1.0 Candidate Recommendation to the
// Recommendation: there, page sequences and references to page masters use
// master-reference instead of master-name. FOP 2.11 rejects the old form.
func upgradeCR(fo []byte) []byte {
	return crRefs.ReplaceAllFunc(fo, func(tag []byte) []byte {
		return bytes.Replace(tag, []byte(" master-name="), []byte(" master-reference="), 1)
	})
}

func download(url, file, sum string) error {
	if data, err := os.ReadFile(file); err == nil && sha(data) == sum {
		return nil
	}
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if got := sha(data); got != sum {
		return fmt.Errorf("%s: SHA-256 %s, erwartet %s", url, got, sum)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// unzip extracts an archive. The archives contain a top-level folder (NIST,
// contrib), which is preserved.
func unzip(file, dst string) error {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := path.Clean(f.Name)
		if strings.HasPrefix(name, "..") || strings.HasPrefix(name, "/") || strings.HasSuffix(f.Name, "/") ||
			strings.Contains(name, "/CVS/") || strings.HasPrefix(name, "CVS/") {
			continue
		}
		out := filepath.Join(dst, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyResources copies images from a directory (including subfolders) into
// the case directory so that relative paths in the FO files resolve.
func copyResources(dir, dst string) error {
	return filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".gif", ".jpg", ".jpeg", ".png", ".svg", ".tif", ".bmp":
		default:
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

type nistTest struct{ name, xml, xsl string }

var (
	nistHead = regexp.MustCompile(`Test #([A-Za-z0-9_.\-]+)`)
	nistHref = regexp.MustCompile(`href="([^"#]+\.(xml|xsl))"`)
)

// nistTests reads the test name, XML and stylesheet from testsuite.html.
func nistTests(html []byte) []nistTest {
	var out []nistTest
	heads := nistHead.FindAllSubmatchIndex(html, -1)
	for i, h := range heads {
		end := len(html)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		t := nistTest{name: string(html[h[2]:h[3]])}
		for _, m := range nistHref.FindAllSubmatch(html[h[1]:end], -1) {
			switch string(m[2]) {
			case "xml":
				if t.xml == "" {
					t.xml = string(m[1])
				}
			case "xsl":
				if t.xsl == "" {
					t.xsl = string(m[1])
				}
			}
		}
		if t.xml != "" && t.xsl != "" {
			out = append(out, t)
		}
	}
	return out
}

// contribTests reads the files of the cases from a testsuite.xml.
func contribTests(file string) []string {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var doc struct {
		Groups []struct {
			Base  string `xml:"base,attr"`
			Tests []struct {
				XML string `xml:"xml,attr"`
			} `xml:"test"`
		} `xml:"testcases"`
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return nil
	}
	var out []string
	for _, g := range doc.Groups {
		for _, t := range g.Tests {
			if t.XML != "" {
				out = append(out, path.Join(g.Base, t.XML))
			}
		}
	}
	return out
}

func transformFile(xmlFile, xslFile string) ([]byte, error) {
	src, err := os.ReadFile(xmlFile)
	if err != nil {
		return nil, err
	}
	sheet, err := os.ReadFile(xslFile)
	if err != nil {
		return nil, err
	}
	res := engine.Transform(engine.Request{Stylesheet: string(sheet), Source: string(src), BaseDir: filepath.Dir(xslFile)})
	for _, d := range res.Diagnostics {
		if d.Severity == engine.SeverityError {
			return nil, fmt.Errorf("%s: %s", filepath.Base(xslFile), d.Message)
		}
	}
	return []byte(res.Output), nil
}
