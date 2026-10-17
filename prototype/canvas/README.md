# Canvas-Prototyp (Phase 2)

Vergleich von Svelte und React für den Canvas des Designers. Beide zeichnen
die Musterrechnung aus dem Area Tree der Engine mit demselben Kern
(`core/`), nur Zustand und Überlagerung (Hover, Auswahl, Griffe, Ziehen)
gehören zum Framework. Der Prototyp wird nach der Entscheidung entfernt.

```bash
go run ./prototype/canvas/export
```

```bash
npm --prefix prototype/canvas ci --ignore-scripts
```

```bash
npm --prefix prototype/canvas run build
```

```bash
npm --prefix prototype/canvas run preview
```

Danach `http://localhost:5180/index.html?suite=1` öffnen. Die Seite misst
beide Frameworks mit 3 und mit 30 Seiten (`?x=10`) und zeigt eine Übersicht.
`svelte.html?bench=1` und `react.html?bench=1` messen einzeln.

Gemessen werden Bilder je Sekunde bei Zoom, Scrollen, Hover und Ziehen
(ein Schritt je Frame), die Zeit vom Ereignis bis zur DOM-Änderung (Commit)
und bis zum nächsten Frame.
