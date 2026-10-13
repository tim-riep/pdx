# pdx

pdx erzeugt PDF-Dateien bis PDF/A-3 aus XSL-FO, komplett in Go und ohne cgo. Es ist die Engine eines Dokumentdesigners nach dem Vorbild von Adobe LiveCycle Designer. Ein XSD beschreibt die Datenschnittstelle, ein Desktop-Designer gestaltet das Dokument, die Engine setzt es. veraPDF ist die verbindliche Prüfinstanz für die Konformität.

Stand: Phase 0 (Fundament). Area Tree, PDF-Writer, Schriften mit Shaping und Subsetting, Bilder, PDF/A-3b, ein kleiner FO-Teilumfang, veraPDF-Anbindung und CLI.

## Aufruf

```bash
go build -o pdx ./cmd/pdx
```

```bash
pdx render eingabe.fo -o ausgabe.pdf --profile pdfa-3b --attach factur-x.xml:Alternative:text/xml --creation-date 2026-01-01T00:00:00Z
```

```bash
pdx verify ausgabe.pdf --profile pdfa-3b
```

`render` schreibt das PDF. Ohne `--creation-date` gilt `SOURCE_DATE_EPOCH`, sonst die aktuelle Zeit. Weitere Optionen: `--font DATEI` (wiederholbar) registriert eigene Schriften, `--area-tree DATEI` schreibt den Area Tree als Text.

`verify` prüft eine Datei mit veraPDF und gibt jeden Befund mit Spezifikation, Klausel, Testnummer, Beschreibung und Objektkontext aus (`--format json` für Maschinen). Exit-Codes: 0 konform, 1 mindestens ein fehlgeschlagener Test oder ein Fehler, 2 Bedienfehler.

## Architektur

```
XSL-FO ──fo/tree──▶ FO-Baum ──fo/layout──▶ Area Tree ──pdf/render──▶ PDF
                     fo/props   fo/linebreak   │           pdf, pdf/profile,
                                font, image     │           pdf/xmp, icc
                                                ▼
                                     später: Canvas im Designer, Rastertests
```

| Paket | Aufgabe |
|---|---|
| `pdx` (Wurzel) | Fassade `Render` und `RenderFile` für CLI, Designer und Server |
| `cmd/pdx` | CLI |
| `areatree` | stabiles Zwischenformat: Seiten, Container, Glyph-Runs, Bilder, Linien in Millipunkt, Ursprung oben links. Keine Abhängigkeit zu anderen pdx-Paketen |
| `fo/tree` | FO-Parser mit Zeilen- und Spaltenangaben, Prüfung des Teilumfangs |
| `fo/props` | Längen, Farben, Shorthands, Vererbung, Ränder zu Einzügen |
| `fo/linebreak` | Box-Glue-Penalty-Modell, Schnittstelle `Breaker`, Greedy in Phase 0 |
| `fo/layout` | Seitenvorlagen, Regionen, Fluss mit Seitenumbruch, Kopf und Fuß, absolute Positionierung, Bilder, Erweiterungen |
| `font` | Laden über textshape, Shaping, Subsetting, Type0 mit Identity-H, `/W`, ToUnicode, Registry mit Liberation-Schriften |
| `image` | JPEG durchreichen, PNG nach Flate, Alpha getrennt |
| `pdf` | Objektmodell, Content Streams, Xref, Trailer, Prüfung des Objektgraphen gegen die Policy |
| `pdf/profile` | Konformitätsprofile |
| `pdf/xmp` | XMP mit pdfaid und pdfaExtension-Schema |
| `pdf/render` | Area Tree zu PDF |
| `icc` | selbst berechnetes sRGB-Profil (ICC 2.1) für das Output Intent |
| `internal/verapdf` | veraPDF-Aufruf und Auswertung des mrr-Berichts |

### Konformität durch Konstruktion

Ein Profil implementiert `pdf.Policy` (`Allow(Feature)`) und `Prepare`. Jede kritische Funktion (Transparenz, Anhänge, LZW, externe Inhalte, nicht eingebettete Schriften) wird vor der Nutzung angefragt. Vor dem Schreiben durchsucht der Writer zusätzlich den gesamten Objektgraphen. Was das Profil nicht erlaubt, bricht mit einem Fehler samt Normklausel ab, statt eine nicht konforme Datei zu erzeugen. JavaScript, XFA und Verschlüsselung schreibt pdx nie.

### Reproduzierbarkeit

Feste Objektreihenfolge, sortierte Dictionary-Schlüssel, feste Zahlenformatierung, injizierbares Erstellungsdatum, Teilmengen-Präfixe aus Hashes und eine Datei-ID aus dem MD5 der Dateibytes. Gleiche Eingabe und gleiches Datum ergeben byte-identische Dateien (`TestReproducible`).

## Profile

| Name | Bedeutung |
|---|---|
| `pdfa-3b` | PDF/A-3b nach ISO 19005-3. Output Intent GTS_PDFA1 mit sRGB, XMP mit pdfaid, Anhänge mit AFRelationship, Transparenz erlaubt |
| `pdf-1.7` | PDF 1.7 ohne Konformitätsanspruch, Schriften trotzdem immer eingebettet |

Geplant: 3u, 3a, 2b, 4, PDF/UA-1 und UA-2.

## FO-Teilumfang (Phase 0)

Elemente: `fo:root`, `fo:layout-master-set`, `fo:simple-page-master`, `fo:region-body`, `fo:region-before`, `fo:region-after`, `fo:declarations`, `fo:page-sequence`, `fo:flow`, `fo:static-content`, `fo:block`, `fo:inline`, `fo:block-container` (auch mit `absolute-position`), `fo:external-graphic`, `fo:page-number`.

Eigenschaften: Schrift (`font-family`, `font-size`, `font-weight`, `font-style`), `color`, `text-align`, `text-align-last`, `text-indent`, `line-height`, Ränder und Einzüge, `space-before`, `space-after`, Padding, Rahmen, `background-color`, `linefeed-treatment`, `top`/`left`/`right`/`bottom`/`width`/`height`, Bildgrößen (`content-width`, `content-height`, `scaling`), `initial-page-number`, `xml:lang`.

Andere FO-Elemente werden mit Position abgelehnt. Unbekannte Eigenschaften werden ignoriert. Zeilenumbruch erfolgt an Leerzeichen (Greedy), Keeps, Tabellen und Silbentrennung folgen in Phase 1.

### Erweiterungen

Namespace `urn:pdx:extensions` (Arbeitsname), nur in `fo:declarations`, damit Apache FOP sie ignoriert:

```xml
<fo:declarations>
  <pdx:metadata title="Rechnung" author="Beispiel GmbH" subject="..." keywords="..." creator="..."/>
  <pdx:attachment src="factur-x.xml" name="factur-x.xml" mime-type="text/xml"
                  relationship="Alternative" description="Rechnungsdaten"/>
</fo:declarations>
```

## Schriften

Schriften werden ausnahmslos als Teilmenge eingebettet. `Helvetica`, `Arial`, `sans-serif` werden auf Liberation Sans abgebildet, `Times` und `serif` auf Liberation Serif, `Courier` und `monospace` auf Liberation Mono (eingebettet im Programm, SIL OFL 1.1). Fehlt einem Wort ein Zeichen, versucht pdx die Liberation-Schriften als Ersatz. Gibt es keines, endet das Rendern mit einem Fehler, weil PDF/A die .notdef-Glyphe verbietet.

## veraPDF

Version 1.30.3, konfiguriert nur in `internal/verapdf`. Eine lokale CLI wird genutzt, wenn ihre Version passt, sonst das Docker-Image `docker.io/verapdf/cli:v1.30.3` (per Digest gepinnt). Auf ghcr.io gibt es nur Entwicklerstände 1.31, das Release liegt auf Docker Hub. `PDX_VERAPDF=local|docker|<pfad>` erzwingt eine Variante.

## Tests

```bash
make test
```

```bash
make conformance
```

```bash
PDX_CONFORMANCE=1 go test ./...
```

* `make test` führt alle Unit-, Golden-File- und Reproduzierbarkeitstests aus.
* `make conformance` rendert den Korpus `testdata/fo` per CLI und prüft jede Datei mit veraPDF.
* Mit `PDX_CONFORMANCE=1` rufen auch die Go-Tests veraPDF auf.
* `make golden` schreibt Golden Files nach bewusster Änderung neu (`UPDATE_GOLDEN=1`).
* `make fuzz` spielt die Fuzz-Targets `FuzzParse` (FO-Parser) und `FuzzLoad` (Schriften) an.

## Lizenz

Apache License 2.0, siehe `LICENSE` und `NOTICE`. Die Liberation-Schriften und Source Serif 4 (nur Testdaten) stehen unter der SIL Open Font License 1.1. Das sRGB-Profil berechnet pdx selbst.
