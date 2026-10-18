# pdx Designer

Desktop-Anwendung, mit der man Dokumente gegen ein Daten-XSD gestaltet, ohne XML zu schreiben. Grundlage ist die Engine von pdx (Ordner darüber). Gespeichert wird ausschließlich im Vorlagenformat 1, verlustfrei und mit stabilen Git-Diffs. Ergebnis ist ein PDF/A-3b mit eingebetteten Daten.

Stand: Phase 2 (Designer-MVP). Die Musterrechnung lässt sich im Designer von Grund auf nachbauen und besteht veraPDF 1.30.3 gegen PDF/A-3b (Ende-zu-Ende-Test `e2e/rebuild.spec.ts`).

## Bedienkonzept

Das Fenster hat drei Spalten mit Tabs und einen Bereich unten.

| Bereich | Tabs | Zweck |
| --- | --- | --- |
| links | Daten, Hierarchie, Beispiele | Datenbaum des XSD zum Ziehen, Baum der Vorlage, Beispieldaten und Randfälle |
| Mitte | Entwurf, Vorschau, PDF | Canvas mit Platzhaltern oder Beispieldaten, das fertige PDF mit pdf.js |
| rechts | Inspektor, Stile | Eigenschaften des gewählten Elements, Design-Tokens und Stile |
| unten | Probleme, Konformität, Barrierefreiheit, XPath | laufende Prüfung, veraPDF, Strukturbaum und Lesereihenfolge, großer XPath-Editor |

Grundsätze:

* **Klick wählt das Vorlagenelement.** Alle Vorkommen im Satz (etwa eine Tabellenzeile auf jeder Seite) werden markiert. Klick in Hierarchie, Problemliste oder Konformität führt zum Element.
* **Daten ziehen statt Pfade tippen.** Ein Datenknoten aus dem Tab Daten wird beim Ablegen zum passenden Element: Datum und Dezimalzahl zu Feldern mit Format, wiederholte Knoten zur Tabelle, Gruppen zum gebundenen Teilformular, optionale Knoten mit Präsenzbedingung.
* **Bausteine** (Text, Teilformular positioniert oder fließend, Linie, Rechteck, Bild, Barcode, Seitenzahl, Tabelle) kommen aus der Leiste über dem Canvas, per Klick in die Auswahl oder per Ziehen an eine Stelle.
* **Text mit Feldern** wird direkt bearbeitet: Doppelklick im Canvas oder Inhalt im Inspektor. Felder stehen als Chips im Text, `@` schlägt Datenknoten vor.
* **Jede Änderung ist ein Befehl** mit Undo und Redo. Das Modell lebt nur im Backend, die Oberfläche zeigt eine Projektion.
* **Prüfen läuft mit.** Die Problemliste meldet nach jeder Änderung Fehler aus Vorlage, Übersetzung, Bindungen und Beispieldaten. Die Konformität prüft auf Knopfdruck mit veraPDF gegen jede Norm des Ausgabeprofils und ordnet Befunde über Seite und Operator den Elementen zu.

Weitere Werkzeuge:

* **Struktur:** Kontextmenü in Hierarchie und Canvas fügt jedes erlaubte Kind ein (etwa Formate, Textressourcen, Seitenvorlagen mit Kopf und Fuß, Anhänge), dupliziert, verschiebt und löscht.
* **Tabellen:** Spalten und Zeilen einfügen, Kopf und Fuß mit Wiederholung, Zellen über Spalten, Zwischensumme, Übertrag und Summe über Zahlenspalten.
* **XPath:** Felder im Inspektor mit Vorschlägen und Prüfzeile (Wert mit den Beispieldaten), der Tab XPath als großer Editor, Präsenz über eine Auswahl (immer, wenn vorhanden, wenn nicht leer, eigener Ausdruck).
* **Tokens und Stile:** Farben, Längen und Schriften benennen und mit `$name` verwenden, Umbenennen zieht alle Verweise mit.
* **Schemaänderung:** Ein neues XSD zeigt erst die Unterschiede, stellt Bindungen auf umbenannte Knoten um und lässt eine Aufgabenliste für den Rest.
* **Beispieldaten:** importieren, wählen, Randfälle erzeugen (minimal, maximal, lange Texte).
* **Ausgabeprofil:** Die Vorlage wählt ihr Profil aus den zulässigen Kombinationen, etwa PDF/A-3a mit PDF/UA-1 oder PDF/A-4f mit PDF/UA-2, jeweils mit einem Satz Erklärung.
* **Barrierefreiheit:** Der Inspektor hat eine eigene Gruppe für Rolle, Alternativtext (fest oder aus den Daten), Sprache, Lesereihenfolge, Dekoration und Kopfbezug. Das Panel Barrierefreiheit zeigt die Befunde der Vorprüfung und den Strukturbaum in Lesereihenfolge, blendet die Reihenfolge als Nummern auf der Seite ein, verschiebt frei platzierte Elemente mit Pfeilen und führt die manuellen Prüfpunkte nach Matterhorn 1.1 als Checkliste. Verlangt das Profil keine Barrierefreiheit, prüft es zur Vorschau nach PDF/A-3a mit PDF/UA-1.
* **E-Rechnung:** Am Element E-Rechnung zeigt der Inspektor die aus den Beispieldaten erkannte Stufe, die eingebettete Datei und die Beziehung und warnt, wenn das Profil nicht PDF/A-3 ist.
* **Formulare:** Textfelder, Kontrollkästchen, Optionsgruppen, Listen und Signaturfelder aus der Palette, Optionen aus der Enumeration des XSD, in der Vorschau ausfüllbar, Tab-Reihenfolge als Nummern auf der Seite.
* **Veröffentlichen:** Der Dialog Veröffentlichen verbindet sich mit einem pdx-server (Adresse und API-Schlüssel mit Recht templates), vergleicht die Vorlage mit der aktiven Version auf dem Server Datei für Datei und `template.xml` Zeile für Zeile, lädt sie als neue Version hoch und aktiviert sie. Der Schlüssel bleibt nur im Speicher der Sitzung, gespeichert wird nur die Adresse.
* **Ansicht:** Lineale in mm oder pt, Raster, Einrasten an Kanten und Mitten, Seitenübersicht, Ausrichten und Verteilen mit Mehrfachauswahl (Umschalt+Klick).

## Tastenkürzel

Unter macOS steht ⌘ für Strg, ⌥ für Alt.

| Kürzel | Wirkung |
| --- | --- |
| Strg+K oder Strg+Umschalt+P | Befehlspalette mit allen Aktionen und Suche nach Elementen |
| Strg+N, Strg+O, Strg+S, Strg+Umschalt+S | Neu, Öffnen, Speichern, Speichern unter |
| Strg+Z, Strg+Umschalt+Z oder Strg+Y | Rückgängig, Wiederholen |
| Strg+D | Duplizieren |
| Entf oder Rücktaste | Löschen |
| Alt+Pfeil hoch oder runter | unter den Geschwistern verschieben |
| Pfeiltasten | positioniertes Element um 1 mm oder 1 pt verschieben, mit Umschalt um 10 |
| Pfeiltasten in der Hierarchie | Zeile wählen, Rechts klappt auf, Links zu oder zum Elternelement |
| Umschalt+F10 oder Kontextmenütaste | Kontextmenü des Elements |
| Doppelklick auf Text | Text an Ort und Stelle bearbeiten, Eingabe übernimmt, Escape verwirft |
| @ im Text | Feld einfügen, Umschalt+Eingabe bricht die Zeile um |
| Strg+Leertaste im XPath-Feld | Vorschläge, Strg+Eingabe übernimmt im Tab XPath |
| Escape | Menü schließen, sonst Auswahl aufheben |

## Architektur

```
designer/
  main.go               einzige Stelle mit Wails (und cgo im App-Binary)
  internal/app/         Services als normale Go-Typen, ohne Wails, cgo-frei
  internal/model/       Dokument, Befehle mit Inverse, Katalog aus dem XSD, Projektion und Patches
  internal/pipeline/    Satz mit Caches, Seiten als kompaktes JSON, PDF mit Render-Spur
  internal/schema/      Datenbaum des Daten-XSD, Kontext von Bindungen, Platzhalter und Randfälle
  internal/xpath/       Pfade prüfen, auswerten (go-xslt), vervollständigen
  notices/              Drittanbieterhinweise (make notices)
  frontend/             Svelte 5, TypeScript, Vite, alles lokal gebündelt
```

* **Eine Wahrheit:** Die Vorlage (`template.Template`) lebt nur im Backend. Die Oberfläche schickt Befehle (`setAttr`, `insert`, `remove`, `move`, `content`, `batch`, `setFile`) und erhält Patches einer Projektion über das Ereignis `template:changed`. Nach jedem Befehl prüft das Backend gegen das XSD des Formats und die Verweise im Paket und nimmt ihn sonst zurück.
* **Kein Layout im Frontend:** Alles Sichtbare kommt aus dem Area Tree der Engine. Jeder Bereich trägt die Vorlagen-ID, darauf beruhen Auswahl, Treffertest, Hilfslinien und das Zuordnen von Befunden.
* **Services:** AppService (Info, Lizenzen), ProjectService (Öffnen, Speichern, Autosave alle 30 s, Wiederherstellung), TemplateService (Befehle, Tabellen, Struktur, Inhalt, Tokens, XPath), RenderService (Ansicht, Seiten, Schriften, Bilder und PDF über `/pdx/`), SchemaService (Datenbaum, Import, Vergleich und Ersetzen), SampleDataService, ValidationService und ConformanceService.
* **Pipeline:** 80 ms nach der letzten Änderung setzt das Backend neu. Der Compiler läuft nur bei geändertem Inhalt, ein neuer Lauf bricht den alten ab. Nur Seiten mit neuem Hash werden übertragen.
* **Prüfung:** dieselben Prüfungen wie `pdx check` (Paket `check` der Engine), dazu Pfade in Präsenzbedingungen. veraPDF läuft lokal oder in Docker im Hintergrund.

Lizenzen: Der Designer steht unter der Apache License 2.0. Alles Mitgelieferte steht unter MIT, BSD, Apache 2.0 oder (für die eingebetteten Liberation-Schriften) der SIL Open Font License 1.1, die Trennmuster unter MIT und freier Weitergabe mit Hinweis. Die Volltexte erzeugt `make notices` mit dem gemeinsamen Werkzeug `../internal/tools/notices` nach `notices/THIRD_PARTY_NOTICES.txt`. Die App zeigt sie über den Knopf Lizenzen in der Werkzeugleiste (auch unter Über und in der Befehlspalette): die eigene Lizenz und jeden Bestandteil mit Lizenz und vollem Text, mit Suche und zum Kopieren.

## Leistung

Gemessen auf Apple M4 mit der Musterrechnung (60 Positionen, drei Seiten).

| Messung | Ergebnis | Ziel | Messort |
| --- | --- | --- | --- |
| Befehl bis Patch, p95 | 4,8 ms | 20 ms | `BenchmarkCommandPatch` |
| Neu setzen nach einer Änderung, Mittel | 58 ms | 300 ms | `BenchmarkDesignerEdit` |
| Bearbeiten bis zur neuen Seite im Canvas, p95 | 200 ms | 300 ms | `e2e/perf.spec.ts` (mit 80 ms Entprellung) |
| Ziehen, Frame-Abstand p95 | 16,8 ms (60 fps) | 33 ms | `e2e/perf.spec.ts` |
| Öffnen bis zur ersten Seite | 160 ms | 1,5 s | `e2e/perf.spec.ts` |
| Start der App bis zur ersten Seite | 280 ms | 1,5 s | `--smoke` |

```bash
go test -run '^$' -bench . ./internal/...
```

```bash
PDX_PERF=1 npx playwright test e2e/perf.spec.ts --project chromium
```

## Bauen und testen

Voraussetzungen: Go 1.26, Node 24. Unter Linux GTK ab 4.10 und WebKitGTK 6.0 (`libgtk-4-dev libwebkitgtk-6.0-dev`, etwa Ubuntu 24.04 oder Debian 13). In Containern braucht WebKitGTK `WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1`.

```bash
make deps
```

```bash
make build
```

| Ziel | Wirkung |
| --- | --- |
| `make build` | Frontend bauen, natives Binary nach `../out/pdx-designer` |
| `make server` | Server-Build ohne Fenster und ohne cgo (für Playwright) |
| `make test` | Go-Tests der Services (`CGO_ENABLED=0`) und Vitest |
| `make check` | `go vet` und svelte-check |
| `make e2e` | Playwright in Chromium und WebKit gegen den Server-Build |
| `make smoke` | natives Binary starten, Musterrechnung setzen, beenden |
| `make bindings` | TypeScript-Bindungen der Services neu erzeugen |
| `make notices` | Drittanbieterhinweise neu schreiben, `notices-check` prüft sie |

Die Ende-zu-Ende-Tests nutzen eine veraPDF-Attrappe. Mit echtem veraPDF (lokal 1.30.3 oder Docker):

```bash
PDX_E2E_REAL_VERAPDF=1 npx playwright test e2e/rebuild.spec.ts --project chromium
```

Startoptionen des Binarys: `--open <paket>` öffnet ein Paket, `--smoke` setzt es und beendet sich (Code 1 bei Fehlern). `PDX_DESIGNER_DATA` ersetzt das Datenverzeichnis für Einstellungen und automatische Sicherungen, `PDX_VERAPDF` wählt veraPDF (`docker`, `local` oder ein Pfad).

CI (`.github/workflows/ci.yml`) baut auf macOS, Windows und Linux, startet den Smoke-Test und führt alle Tests aus.
