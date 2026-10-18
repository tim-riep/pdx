# pdx

pdx erzeugt PDF-Dateien in PDF/A-2, PDF/A-3 und PDF/A-4, barrierefrei nach PDF/UA-1 und PDF/UA-2 sowie als E-Rechnung nach ZUGFeRD und Factur-X aus XSL-FO, komplett in Go und ohne cgo. Es ist die Engine eines Dokumentdesigners nach dem Vorbild von Adobe LiveCycle Designer. Ein XSD beschreibt die Datenschnittstelle, ein Desktop-Designer gestaltet das Dokument, die Engine setzt es. veraPDF ist die verbindliche Prüfinstanz für die Konformität.

Stand: Phase 3 (Konformität erweitert: PDF/A-2, -3, -4, PDF/UA-1, -2, Tagged PDF, E-Rechnung). Der Desktop-Designer (Wails und Svelte) liegt in [`designer/`](designer/README.md), mit ihm lässt sich die Musterrechnung ohne XML nachbauen. Darunter die Layout-Engine für Geschäftsdokumente (Seitenfolgen, Tabellen, Listen, Keeps, Marker, Knuth-Plass, Silbentrennung, Links, Lesezeichen, Barcodes), Vorlagenformat v0 mit Compiler nach XSLT 3.0, Bindungsprüfung gegen das Daten-XSD, Vergleich mit Apache FOP, Design-Tokens im Vorlagenformat.

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
| `pdfa-3a` | PDF/A-3a, Tagged PDF mit Strukturbaum (6.7). Verlangt Dokumentsprache und Alternativtexte für Abbildungen |
| `pdfa-4`, `pdfa-4f` | PDF/A-4 nach ISO 19005-4 auf Grundlage von PDF 2.0. Kopf `%PDF-2.0`, kein Info-Dictionary, Metadaten nur im XMP mit `pdfaid:rev` 2020. Unter `pdfa-4` Anhänge nur als PDF/A-1, -2 oder -4 (6.9), `pdfa-4f` erlaubt beliebige Anhänge und verlangt mindestens einen |
| `pdf-2.0` | PDF 2.0 ohne Konformitätsanspruch, Metadaten im XMP |
| `pdfua-2` | PDF/UA-2 nach ISO 14289-2 auf Grundlage von PDF 2.0, kombinierbar mit PDF/A-4, etwa `pdfa-4f,pdfua-2`. Strukturelemente im Namespace von PDF 2.0, Fußnoten als FENote, interne Verweise und Lesezeichen mit Strukturziel, Anhänge mit Beschreibung, `pdfuaid:rev` 2024 |
| `pdfua-1` | PDF/UA-1 nach ISO 14289-1, kombinierbar mit PDF/A-2 und PDF/A-3, etwa `pdfa-3a,pdfua-1`. XMP mit pdfuaid, DisplayDocTitle. Verlangt zusätzlich Titel, lückenlose Überschriften ab H1, Kopfzellen in Tabellen und Verweise mit Text im Strukturbaum |
| `pdf-1.7` | PDF 1.7 ohne Konformitätsanspruch, Schriften trotzdem immer eingebettet |

Profile lassen sich kombinieren, etwa `--profile pdfa-3a,pdfua-1`. Unzulässige Kombinationen lehnt pdx vor dem Rendern mit allen Gründen ab: verschiedene PDF-Versionen (PDF/A-2, PDF/A-3 und PDF/UA-1 beruhen auf PDF 1.7, PDF/A-4 und PDF/UA-2 auf PDF 2.0), zwei PDF/A- oder zwei PDF/UA-Teile. `pdx verify` prüft mit veraPDF jeden beanspruchten Standard in einem eigenen Lauf.

Unter `pdfa-3a` und `pdfua-1` prüft pdx vor dem Schreiben alles, was sich nicht auffüllen lässt, und meldet alle Befunde auf einmal mit der ID des Vorlagenelements, etwa `[girocode] Abbildung ohne Alternativtext (alt oder altBind), ISO 14289-1 7.3, ISO 19005-3 6.7`. Kopf und Fuß sind Artefakte, `artifact="false"` holt Inhalte wie Anschrift und Rechnungsdaten in den Strukturbaum. Wiederholte Tabellenköpfe, Überträge und Zwischensummen sind Artefakte.

Listen bekommen `ListNumbering` aus dem Text ihrer Marken (Ziffern Decimal, Buchstaben LowerAlpha oder UpperAlpha, sonst Disc bzw. Unordered unter PDF 2.0).

## Barrierefreiheit

Mit `pdfa-3a`, `pdfua-1` oder `pdfua-2` (oder `--tagged`) schreibt pdx Tagged PDF. Der Strukturbaum entsteht aus den Rollen der Vorlage: Überschriften, Absätze, Tabellen mit Kopf, Rumpf, Fuß und Kopfzellen samt Bezug, Listen mit Marke und Inhalt, Abbildungen mit Alternativtext, Verweise mit Annotation. Die Lesereihenfolge folgt dem Fluss, frei platzierte Elemente ordnet `readingOrder`, sonst gilt oben nach unten und links nach rechts.

In der Vorlage dienen dazu die Attribute `role`, `alt`, `altBind` (Alternativtext aus den Daten, etwa beim GiroCode), `lang`, `readingOrder`, `artifact` und an Zellen `scope`. `pdx structure ausgabe.pdf` zeigt den Baum mit Text in Lesereihenfolge.

Was keine Software entscheiden kann, etwa ob ein Alternativtext inhaltlich stimmt, listet der Designer als Checkliste nach dem Matterhorn Protocol 1.1 (PDF Association, CC BY 4.0).

### Kurztest mit Screenreader

VoiceOver (macOS) mit Vorschau, geprüft am 18.10.2026 mit der Musterrechnung als `pdfa-3a,pdfua-1`:

1. `pdx render --profile pdfa-3a,pdfua-1 --template testdata/templates/musterrechnung -o rechnung.pdf testdata/templates/musterrechnung/samples/rechnung.xml`
2. Die Datei in Vorschau öffnen, VoiceOver mit Cmd+F5 einschalten, mit Ctrl+Option+Pfeil rechts durch das Dokument gehen.
3. Erwartet: Briefkopf, Absenderzeile, Anschrift und Rechnungsdaten vor der Überschrift „Rechnung“ (als Überschrift angesagt), danach die Tabelle mit Zeilen und Zellen, Seitenzahlen und wiederholte Tabellenköpfe werden übersprungen, der GiroCode wird mit seinem Alternativtext angesagt.

Ergebnis: Vorschau liefert VoiceOver die Überschrift, die Tabelle als Zeilen und Zellen und das Bild mit Alternativtext aus dem Strukturbaum, in der Reihenfolge des Baums. Dieselbe Rechnung ohne Tags (`pdfa-3b`) hat keine Überschrift, die Tabellenzeilen rät Vorschau dort aus dem Layout. Vorschau wertet nicht alles aus, was PDF/UA beschreibt, etwa den Bezug der Kopfzellen. Für eine vollständige Prüfung eignen sich NVDA mit Adobe Acrobat Reader unter Windows oder der PDF Accessibility Checker (PAC).

NVDA (Windows): Datei in Acrobat Reader öffnen, mit H von Überschrift zu Überschrift springen, mit T zur Tabelle, in der Tabelle mit Strg+Alt+Pfeiltasten von Zelle zu Zelle. NVDA sagt dabei die Spaltenköpfe an.

## E-Rechnung

Eine Vorlage mit dem Element `einvoice` erzeugt hybride Rechnungen nach ZUGFeRD 2 und Factur-X 1.0. Die Daten müssen eine CII-Rechnung sein (`rsm:CrossIndustryInvoice`), UBL lehnt pdx mit Meldung ab. Die Stufe steht in den Daten (Guideline-ID, BT-24): MINIMUM, BASIC WL, BASIC, EN 16931, EXTENDED oder XRECHNUNG. pdx bettet die Daten als `factur-x.xml` ein (bei XRECHNUNG `xrechnung.xml`), mit `AFRelationship` Data für MINIMUM und BASIC WL, sonst Alternative (`afRelationship` am Element überschreibt das), und schreibt das XMP-Schema `fx`. Die Werte sind gegen die Referenzimplementierung Mustang geprüft.

E-Rechnungen verlangen PDF/A-3, also `pdfa-3b`, `pdfa-3u` oder `pdfa-3a,pdfua-1`. Andere Profile lehnt pdx mit der ID des Elements ab. Eine Prüfung der Rechnungsdaten gegen die Schematron-Regeln von EN 16931 und XRechnung macht pdx nicht. Beispiel: `testdata/templates/cii-rechnung` mit Daten der Stufen EN 16931 und XRECHNUNG.

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
