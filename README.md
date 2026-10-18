# pdx

pdx erzeugt PDF-Dateien bis PDF/A-3 aus XSL-FO, komplett in Go und ohne cgo. Es ist die Engine eines Dokumentdesigners nach dem Vorbild von Adobe LiveCycle Designer. Ein XSD beschreibt die Datenschnittstelle, ein Desktop-Designer gestaltet das Dokument, die Engine setzt es. veraPDF ist die verbindliche Prüfinstanz für die Konformität.

Stand: Phase 2. Der Desktop-Designer (Wails und Svelte) liegt in [`designer/`](designer/README.md), mit ihm lässt sich die Musterrechnung ohne XML nachbauen. Darunter die Layout-Engine für Geschäftsdokumente (Seitenfolgen, Tabellen, Listen, Keeps, Marker, Knuth-Plass, Silbentrennung, Links, Lesezeichen, Barcodes), Vorlagenformat v0 mit Compiler nach XSLT 3.0, Bindungsprüfung gegen das Daten-XSD, Vergleich mit Apache FOP, Design-Tokens im Vorlagenformat.

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

`render` schreibt das PDF. Ohne `--creation-date` gilt `SOURCE_DATE_EPOCH`, sonst die aktuelle Zeit. Weitere Optionen: `--font DATEI` (wiederholbar) registriert eigene Schriften, `--area-tree DATEI` schreibt den Area Tree als Text, `--tagged` schreibt Tagged PDF mit Strukturbaum auch dann, wenn das Profil es nicht verlangt.

```bash
pdx structure ausgabe.pdf
```

`structure` gibt den logischen Strukturbaum aus: je Element Typ, Sprache, Alternativtext, Bezug von Kopfzellen und Text in Lesereihenfolge. Widersprüche zwischen Baum, ParentTree und markiertem Inhalt (unmarkierter Inhalt, fehlende oder verwaiste MCIDs) erscheinen als Befund und ergeben Exit-Code 1.

Mit einer Vorlage (ZIP-Paket oder Verzeichnis mit `template.xml`) werden aus Daten Dokumente:

```bash
pdx render --template vorlage.zip daten.xml -o ausgabe.pdf --profile pdfa-3b --strict
```

```bash
pdx check vorlage.zip
```

```bash
pdx compile vorlage.zip -o vorlage.xsl --source-map vorlage.map.json
```

`--strict` prüft die Daten vorher gegen das Daten-XSD des Pakets, `--lang` wählt Textressourcen und Formatmuster, `--fo DATEI` schreibt das erzeugte FO mit. `check` meldet Fehler in Vorlage, Paket, Übersetzung und Bindungen mit der ID des Vorlagenelements (`--format json` für den Designer). `pdx fmt -w template.xml` schreibt die Vorlage kanonisch, `pdx pack verzeichnis -o vorlage.zip` packt sie deterministisch.

`verify` prüft eine Datei mit veraPDF und gibt jeden Befund mit Spezifikation, Klausel, Testnummer, Beschreibung und Objektkontext aus (`--format json` für Maschinen). Exit-Codes: 0 konform, 1 mindestens ein fehlgeschlagener Test oder ein Fehler, 2 Bedienfehler.

## Architektur

```
Vorlage ──compile──▶ XSLT 3.0 ──go-xslt──▶ XSL-FO      Daten-XSD ──dataschema──▶ Befunde
(template, template/pkg)   ▲ Daten                │
                                                  ▼
XSL-FO ──fo/tree──▶ FO-Baum ──fo/layout──▶ Area Tree ──pdf/render──▶ PDF
                     fo/props   fo/linebreak   │           pdf, pdf/profile,
                                font, image,    │           pdf/xmp, icc
                                hyph, barcode   ▼
                                     später: Canvas im Designer, Rastertests
```

| Paket | Aufgabe |
|---|---|
| `pdx` (Wurzel) | Fassade `Render`, `RenderFile`, `RenderTemplate` und `CompileTemplate` für CLI, Designer und Server |
| `cmd/pdx` | CLI |
| `areatree` | stabiles Zwischenformat: Seiten, Container, Glyph-Runs, Bilder, Linien in Millipunkt, Ursprung oben links. Keine Abhängigkeit zu anderen pdx-Paketen |
| `fo/tree` | FO-Parser mit Zeilen- und Spaltenangaben, Prüfung des Teilumfangs |
| `fo/props` | Längen, Farben, Shorthands, Vererbung, Ränder zu Einzügen |
| `fo/linebreak` | Box-Glue-Penalty-Modell, Schnittstelle `Breaker`, Knuth-Plass mit den Parametern von FOP, Greedy als Rückfall |
| `fo/layout` | Galley und Seitenumbruch, Seitenfolgen, Tabellen, Listen, Keeps, Marker, Verweise über mehrere Durchläufe, Links, Lesezeichen, Barcodes |
| `hyph` | Silbentrennung nach Liang mit Mustern aus hyph-utf8 (de-1996, en-us) |
| `barcode` | QR, GiroCode (EPC069-12), DataMatrix, Code 128 als Modulraster |
| `template` | Vorlagenformat v0: XSD, Parser, kanonischer Writer |
| `template/pkg` | Vorlagenpakete aus ZIP oder Verzeichnis, deterministisches Schreiben |
| `compile` | Vorlage zu XSLT 3.0 mit Überträgen, Formaten und Quellzuordnung |
| `dataschema` | statische Prüfung der Bindungen gegen das Daten-XSD |
| `check` | alle Prüfungen von `pdx check` für CLI und Designer |
| `font` | Laden über textshape, Shaping, Subsetting, Type0 mit Identity-H, `/W`, ToUnicode, Registry mit Liberation-Schriften |
| `image` | JPEG durchreichen, PNG nach Flate, Alpha getrennt |
| `pdf` | Objektmodell, Content Streams, Xref, Trailer, Prüfung des Objektgraphen gegen die Policy |
| `pdf/profile` | Konformitätsprofile |
| `pdf/xmp` | XMP mit pdfaid und pdfaExtension-Schema |
| `pdf/render` | Area Tree zu PDF |
| `icc` | selbst berechnetes sRGB-Profil (ICC 2.1) für das Output Intent |
| `verapdf` | veraPDF-Aufruf und Auswertung des mrr-Berichts |
| `internal/fopcompare` | Vergleich mit Apache FOP 2.11 über Docker: Area Tree, Textpositionen, Pixel |

### Konformität durch Konstruktion

Ein Profil implementiert `pdf.Policy` (`Allow(Feature)`) und `Prepare`. Jede kritische Funktion (Transparenz, Anhänge, LZW, externe Inhalte, nicht eingebettete Schriften) wird vor der Nutzung angefragt. Vor dem Schreiben durchsucht der Writer zusätzlich den gesamten Objektgraphen. Was das Profil nicht erlaubt, bricht mit einem Fehler samt Normklausel ab, statt eine nicht konforme Datei zu erzeugen. JavaScript, XFA und Verschlüsselung schreibt pdx nie.

### Reproduzierbarkeit

Feste Objektreihenfolge, sortierte Dictionary-Schlüssel, feste Zahlenformatierung, injizierbares Erstellungsdatum, Teilmengen-Präfixe aus Hashes und eine Datei-ID aus dem MD5 der Dateibytes. Gleiche Eingabe und gleiches Datum ergeben byte-identische Dateien (`TestReproducible`).

## Profile

| Name | Bedeutung |
|---|---|
| `pdfa-3b` | PDF/A-3b nach ISO 19005-3. Output Intent GTS_PDFA1 mit sRGB, XMP mit pdfaid, Anhänge mit AFRelationship, Transparenz erlaubt |
| `pdfa-3u` | PDF/A-3u, zusätzlich eine Unicode-Zuordnung für jede Glyphe (6.2.11.7.2). Fehlt sie, bricht das Schreiben mit Schrift und Glyphe ab |
| `pdfa-2b`, `pdfa-2u` | PDF/A-2 nach ISO 19005-2. Anhänge nur als PDF/A-1 oder PDF/A-2 (6.8), alle unzulässigen Anhänge in einer Meldung |
| `pdf-1.7` | PDF 1.7 ohne Konformitätsanspruch, Schriften trotzdem immer eingebettet |

Profile lassen sich kombinieren, etwa `--profile pdfa-3a,pdfua-1`. Unzulässige Kombinationen lehnt pdx vor dem Rendern mit allen Gründen ab: verschiedene PDF-Versionen (PDF/A-2, PDF/A-3 und PDF/UA-1 beruhen auf PDF 1.7, PDF/A-4 und PDF/UA-2 auf PDF 2.0), zwei PDF/A- oder zwei PDF/UA-Teile. `pdx verify` prüft mit veraPDF jeden beanspruchten Standard in einem eigenen Lauf.

In Arbeit (Phase 3): 3a, PDF/UA-1, 4, 4f, PDF/UA-2, E-Rechnung.

## FO-Teilumfang

Seitenaufbau: `fo:simple-page-master` mit allen fünf Regionen, `fo:page-sequence-master` mit `fo:single-page-master-reference`, `fo:repeatable-page-master-reference`, `fo:repeatable-page-master-alternatives` und `fo:conditional-page-master-reference` (`page-position`, `odd-or-even`, `blank-or-not-blank`), `force-page-count`, `initial-page-number`.

Blöcke und Inline: `fo:block`, `fo:block-container` (auch `absolute-position`), `fo:inline`, `fo:leader`, `fo:basic-link`, `fo:external-graphic`, `fo:instream-foreign-object`, `fo:page-number`, `fo:page-number-citation`, `fo:page-number-citation-last`.

Tabellen und Listen: `fo:table` mit festem Layout, proportionalen Spalten, Kopf und Fuß mit Wiederholung, Zellen über mehrere Spalten und Zeilen, Rahmenmodelle `separate` und `collapse` (vereinfacht). `fo:list-block` mit `label-end()` und `body-start()`.

Umbruch: `keep-together`, `keep-with-next`, `keep-with-previous` mit Stärken, `break-before`, `break-after`, `widows`, `orphans`. Zeilenumbruch mit Knuth-Plass, Blocksatz mit `text-align-last`, Silbentrennung mit `hyphenate`, `language`, `hyphenation-character` und den Mindestlängen.

Marker und Gliederung: `fo:marker`, `fo:retrieve-marker` mit allen Positionen und Grenzen, `fo:retrieve-table-marker` in Kopf und Fuß, `fo:bookmark-tree`.

Bewusste Abweichungen von Apache FOP stehen mit Begründung aus der Spezifikation in `testdata/suites/DEVIATIONS.md`.

### Erweiterungen

Namespace `https://pdx.example/ns/pdx/1.0` (die Domain pdx.example ist ein Platzhalter). Metadaten und Anhänge stehen in `fo:declarations`, Barcodes in `fo:instream-foreign-object`, dem Ort, den XSL 1.1 für fremde Inhalte vorsieht:

```xml
<fo:declarations>
  <pdx:metadata title="Rechnung" author="Beispiel GmbH" subject="..." keywords="..." creator="..."/>
  <pdx:attachment src="factur-x.xml" name="factur-x.xml" mime-type="text/xml"
                  relationship="Alternative" description="Rechnungsdaten"/>
</fo:declarations>

<fo:instream-foreign-object content-width="30mm">
  <pdx:barcode type="girocode" name="Beispiel GmbH" iban="DE02120300000000202051" amount="1190.00" text="RE-2026-0001"/>
</fo:instream-foreign-object>
```

Das erzeugte FO einer Vorlage trägt an jedem Element `pdx:id` mit der ID des Vorlagenelements. FOP 2.11 überspringt `fo:declarations`, setzt Barcodes aber nicht und lehnt `pdx:id` ohne `fop -r` ab (siehe DEVIATIONS.md).

## Vorlagen

Eine Vorlage beschreibt das Dokument unabhängig von den Daten: Seitenvorlagen für erste, folgende und letzte Seiten, Kopf und Fuß mit positioniertem Inhalt, Teilformulare (positioniert oder fließend, auch wiederholt), Texte mit eingebetteten Feldern, Tabellen mit Kopf, Fuß, Gruppen, Zwischensumme (`subtotal`), Übertrag (`carryOver`) und Summe (`total`), Barcodes, Seitenzahlen („Seite {current} von {total}“), Stile, Formatmuster je Gebietsschema und Textressourcen je Sprache. Bindungen sind XPath-Ausdrücke gegen das Daten-XSD. Das Format beschreibt `template/schema/template-v0.xsd`, Namen sind englisch in camelCase.

Design-Tokens benennen Farben, Längen und Schriften an einer Stelle (`<tokens>` nach `<styles>`). Stile und Elemente verweisen mit `$id` darauf, etwa `color="$primaer"`, `padding="$abstand-m 2mm"` oder `border="0.5pt solid $linie"`. Ein Schrift-Token setzt über `font="$fliesstext"` Familie, Größe, Stärke und Stil, ausdrücklich gesetzte Attribute desselben Elements gehen vor. Tokens dürfen auf Tokens derselben Art verweisen. Der Compiler löst die Verweise auf und meldet unbekannte Tokens, falsche Arten und Kreise mit Vorlagen-ID.

Ein Paket enthält `template.xml` und die Ordner `schema` (Daten-XSD), `assets`, `samples` und `texts`. Beispiel: `testdata/templates/musterrechnung` mit 60 Positionen auf drei Seiten, Adressfenster nach DIN 5008, Übertrag und Zwischensumme je Seite, GiroCode und den Rechnungsdaten als eingebettetem XML (PDF/A-3b).

Der Compiler erzeugt lesbares, deterministisches XSLT 3.0. Laufende Summen entstehen mit `xsl:iterate`, Überträge laufen über Marker je Tabellenzeile. Eine Quellzuordnung übersetzt Fehler beim Transformieren und Setzen in Vorlagen-IDs.

## Leistung

Musterrechnung (60 Positionen, drei Seiten, PDF/A-3b mit Anhang), Apple M4, Go 1.26:

| Benchmark | Zeit | Speicher |
|---|---|---|
| `BenchmarkCompileInvoice` (Vorlage zu kompiliertem XSLT) | 1,4 ms | 0,7 MB |
| `BenchmarkRenderInvoice` (Transformation, Layout, PDF) | 65 ms | 36 MB |

```bash
go test -run '^$' -bench Invoice -benchmem .
```

## Schriften

Schriften werden ausnahmslos als Teilmenge eingebettet. `Helvetica`, `Arial`, `sans-serif` werden auf Liberation Sans abgebildet, `Times` und `serif` auf Liberation Serif, `Courier` und `monospace` auf Liberation Mono (eingebettet im Programm, SIL OFL 1.1). Fehlt einem Wort ein Zeichen, versucht pdx die Liberation-Schriften als Ersatz. Gibt es keines, endet das Rendern mit einem Fehler, weil PDF/A die .notdef-Glyphe verbietet.

## veraPDF

Version 1.30.3, konfiguriert nur in `verapdf`. Eine lokale CLI wird genutzt, wenn ihre Version passt, sonst das Docker-Image `docker.io/verapdf/cli:v1.30.3` (per Digest gepinnt). Auf ghcr.io gibt es nur Entwicklerstände 1.31, das Release liegt auf Docker Hub. `PDX_VERAPDF=local|docker|<pfad>` erzwingt eine Variante.

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
* `make conformance` rendert den Korpus `testdata/fo` und die Vorlagen unter `testdata/templates` per CLI und prüft jede Datei mit veraPDF.
* `make fo-conformance` setzt die FO-Testsuiten (ausgewählte Fälle aus Apache FOP 2.11 und die W3C XSL Test Suite, die beim ersten Lauf heruntergeladen wird) und vergleicht sie mit Erwartungen aus FOP. Bericht je Kategorie in `out/fo-conformance.md`, deaktivierte Fälle mit Begründung in `testdata/suites/disabled.txt`, bekannte und offene Abweichungen in `testdata/suites/deviating.txt`. `make fo-reference` erzeugt die Erwartungen neu (Docker).
* `make fop-compare` vergleicht Korpus und Musterrechnung mit Apache FOP 2.11 (Docker, `PDX_FOP=1`). Toleranz: Positionen 1 pt, höchstens 0,5 % abweichende Pixel je Seite bei 72 dpi.
* Mit `PDX_CONFORMANCE=1` rufen auch die Go-Tests veraPDF auf.
* `make golden` schreibt Golden Files nach bewusster Änderung neu (`UPDATE_GOLDEN=1`).
* `make fuzz` spielt die Fuzz-Targets `FuzzParse` (FO-Parser), `FuzzLoad` (Schriften), `FuzzTemplateParse` (Vorlagen) und `FuzzCompile` (Compiler) an.

## Lizenz

Apache License 2.0, siehe `LICENSE` und `NOTICE`. Die Liberation-Schriften und Source Serif 4 (nur Testdaten) stehen unter der SIL Open Font License 1.1. Das sRGB-Profil berechnet pdx selbst.
