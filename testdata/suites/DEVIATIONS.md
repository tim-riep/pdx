# Bewusste Abweichungen von Apache FOP

pdx folgt der XSL-1.1-Spezifikation. Weicht Apache FOP davon ab oder lässt
die Spezifikation einen Spielraum, den beide Engines verschieden nutzen,
steht der Fall hier mit Begründung. Der FOP-Vergleich wertet diese Fälle
nicht als Fehler.

## left und top bei absoluter Positionierung

XSL 1.1 Abschnitt 7.6.4 bis 7.6.7 übernimmt `top`, `right`, `bottom` und
`left` aus CSS2 9.3.2. Dort heißt es zu `left`: „specifies how far a box's
left margin edge is offset to the right of the left edge of the box's
containing block“. Gemeint ist die Randkante (margin edge). Ohne Margin ist
das die Außenkante des Rahmens.

FOP 2.11 setzt bei `fo:block-container` mit `absolute-position` waagerecht
die Inhaltskante auf `left`, der Rahmen ragt links darüber hinaus
(`space-start` negativ). Senkrecht nutzt FOP die Randkante. pdx nutzt für
beide Richtungen die Randkante. Betroffen: Zeilen in absolut positionierten
Containern mit Rahmen oder Padding liegen waagerecht um Rahmen plus Padding
verschoben (Korpus: adressblock).

`width` und `height` beziehen sich bei beiden Engines auf das
Inhaltsrechteck (XSL 1.1 7.15.14, `inline-progression-dimension`).

## Ligaturen

pdx wendet beim Shaping die OpenType-Standardfeatures an, darunter `liga`
(fi, fl, ffi). FOP 2.11 nutzt für lateinische Schrift keine Ligaturen. XSL
1.1 regelt Ligaturen nicht, die Glyphenwahl ist Sache der Implementierung
(4.7.2 „glyph-areas“). Ligaturen verkürzen Wörter leicht, eine Zeile kann
dadurch anders umbrechen (Korpus: sonderzeichen). Abschalten lässt sich das
in pdx über die Shaping-Optionen.

## Verteilung des Weißraums im Blocksatz

Beide Engines dehnen U+0020 und U+00A0, wie XSL 1.1 7.17.7
(`treat-as-word-space`) es vorsieht. Wie der Weißraum auf die
Zwischenräume verteilt wird, lässt die Spezifikation offen: „Word spacing
algorithms are user agent-dependent“ (7.17.8 aus CSS2). pdx verteilt
proportional zur Dehnbarkeit jedes Zwischenraums. Bei Zeilen mit sehr viel
Weißraum (etwa `text-align-last="justify"` mit wenigen Wörtern) liegen
Wörter daher an anderen Stellen, Zeileninhalte und Grundlinien stimmen
überein. Für Spalten ist eine Tabelle das richtige Mittel.

## Tabellen im Modell collapse

`border-collapse="collapse"` ist der Anfangswert (XSL 1.1 7.26.1). pdx löst
jede Zellkante zwischen Zelle, Nachbarzelle, Zeile und an den Außenkanten
der Tabelle nach Breite und dann nach Stilrang auf (6.7.10). Rahmen von
Spalten und Abschnitten (fo:table-column, fo:table-body) gehen noch nicht in
die Auflösung ein. Das Modell `collapse-with-precedence` wird wie `collapse`
behandelt.
