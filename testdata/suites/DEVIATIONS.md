# Bewusste Abweichungen von Apache FOP

Maßstab ist die XSL-1.1-Spezifikation des W3C, nicht Apache FOP. pdx folgt der XSL-1.1-Spezifikation. Weicht Apache FOP davon ab oder lässt
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
dadurch in Grenzfällen anders umbrechen. Abschalten lässt sich das in pdx
über die Shaping-Optionen.

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

`border-collapse="collapse"` ist der Anfangswert (XSL 1.1 7.28.3). pdx löst
jede Zellkante zwischen Zelle, Nachbarzelle, Zeile, Abschnitt, Spalte und
Tabelle auf, wo sie angrenzen (6.7.10): `hidden` unterdrückt die Kante, sonst
gewinnt der breitere Rahmen, dann der stärkere Stil, dann die Priorität nach
den Anfangswerten von `border-*-precedence` (7.28.1). Überspannt eine Zelle
mehrere Zeilen, bestimmt die breiteste Seitenkante ihren Einzug. Das Modell
`collapse-with-precedence` mit eigenen Prioritäten wird wie `collapse`
behandelt.

## fo:retrieve-table-marker im Tabellenkopf

XSL 1.1 6.13.7 und 7.25.6 erlauben `fo:retrieve-table-marker` in
`fo:table-header` und `fo:table-footer`. Für `first-starting` gilt der
erste Bereich mit Marker, der im primären Suchbereich (dem Tabellenfragment
der Seite) beginnt. pdx setzt den Kopf deshalb erst, nachdem die
Rumpfzeilen der Seite feststehen, und zeigt im Kopf der Folgeseite den
Übertrag aus der ersten dort beginnenden Zeile.

FOP 2.11 setzt Tabellenmarker im Kopf nicht ein, der Kopf bleibt an dieser
Stelle leer. Im Fuß arbeiten beide Engines gleich. Der Vergleich wertet die
fehlende Zeile im Kopf bei FOP nicht als Fehler (Korpus: uebertrag).

In der Musterrechnung (Vorlage testdata/templates/musterrechnung) bleibt die
Übertragszeile im Kopf der Folgeseiten bei FOP leer und damit niedriger. Alle
Zeilen der Seite liegen bei FOP um ihre Höhe (12,35 pt) weiter oben. Der
Seitenumbruch ist bei beiden Engines gleich.

FOP 2.11 bricht außerdem ab, wenn `fo:retrieve-table-marker` direkt in
`fo:table-cell` steht (NullPointerException im TableCellLayoutManager) oder
wenn ein Marker, der in einem `fo:block` eingesetzt wird, Blöcke enthält
(ClassCastException). Der Compiler setzt den Marker deshalb immer in einen
Block und schreibt in Übertragszeilen nur Inline-Inhalt. Beides ist gültiges
XSL und ändert das Ergebnis von pdx nicht.

Marker müssen bei FOP in `fo:table-cell` stehen. XSL erlaubt sie auch
direkt in `fo:table-row`, pdx unterstützt beides. Marker an `fo:inline`
setzt pdx noch nicht um.

## Silbentrennung

pdx bringt Trennmuster für Deutsch (de-1996) und amerikanisches Englisch
(en-us) aus hyph-utf8 mit. FOP 2.11 liefert aus Lizenzgründen keine Muster
mit (das Projekt OFFO wird getrennt verteilt), ohne sie trennt FOP nicht.
Fälle mit `hyphenate="true"` werden deshalb nicht mit FOP verglichen
(Korpus: silbentrennung). Wie FOP versucht pdx zuerst einen Umbruch ohne
Trennstellen und gibt Trennstellen den Strafwert 50.

## Lesezeichen mit external-destination

XSL 1.1 6.11.2 erlaubt an `fo:bookmark` neben `internal-destination` auch
`external-destination`. pdx schreibt dafür einen Gliederungseintrag mit
URI-Aktion. FOP 2.11 bricht dabei mit „DestinationData must contain a ID
reference“ ab. Der Korpusfall verweise nutzt deshalb nur interne Ziele,
externe Lesezeichen prüft ein eigener Test (pdf/render).

## Barcodes in fo:instream-foreign-object

pdx setzt `pdx:barcode` in `fo:instream-foreign-object`, dem Ort, den XSL
1.1 6.6.6 für Inhalte fremder Namespaces vorsieht. FOP 2.11 kennt das
Element nicht: Das PDF entsteht ohne den Barcode, FOP meldet „The intrinsic
dimensions of an instream-foreign-object could not be determined“ (SEVERE),
der Export des Area Tree (`-at`) bricht ab. Der Fall barcodes wird nicht mit
FOP verglichen. Stattdessen dekodiert ein Test die gerasterten Barcodes mit
zbar (QR-Code und Code 128) und prüft ihren Inhalt.

## Attribute fremder Namespaces (pdx:id)

XSL 1.1 1.1.2 erlaubt Attribute aus anderen Namespaces an allen
Formatierungsobjekten, eine Implementierung darf sie ignorieren. Der
Compiler schreibt an jedes erzeugte FO-Element `pdx:id` mit der ID des
Vorlagenelements, damit Designer und Fehlermeldungen Bereiche ihrem
Ursprung zuordnen können.

FOP 2.11 lehnt Attribute aus Namespaces ohne eigenes Element Mapping in der
Standardeinstellung als ungültige Eigenschaft ab und bricht ab („Invalid
property encountered on fo:root: pdx:id“). Mit gelockerter Prüfung (`fop -r`
oder `<strict-validation>false</strict-validation>` in der Konfiguration)
gibt FOP nur Warnungen aus und setzt das Dokument. Eine Konfiguration, die
nur einen Namespace ignoriert, bietet FOP nicht an, nur die Java-API
(`FopFactoryBuilder.ignoreNamespace`). Der FOP-Vergleich der Musterrechnung
läuft deshalb mit `-r`.

## Ungültige Eigenschaftswerte

XSL 1.1 legt für ungültige Werte keine Ersatzregel fest. Nach 5.9.6 ist eine
Länge ohne Einheit eine Zahl mit der Potenz null, verlangt die Eigenschaft
eine Länge (Potenz eins), heißt es: „If any other power is specified, the
value is an error.“ Die zulässigen Einheiten stehen in 5.9.13 (cm, mm, in,
pt, pc, px, em). pdx meldet solche Fehler mit Position und bricht ab, statt
still einen Ersatzwert zu nehmen. FOP setzt dagegen fort, etwa mit dem
Anfangswert oder mit Millipunkt für Zahlen ohne Einheit. Betroffen sind
Testfälle mit Werten wie `space-after="100"`, `height="8400mpt"`,
`font-style="obscure"`, `color="invalidvalue"` oder `padding="auto"`.

Attribute aus fremden Namespaces muss ein Prozessor dagegen ohne Fehler
ignorieren (XSL 1.1 1.2: „must ignore such attributes without giving an
error if it does not recognize the namespace URI“). pdx tut das, FOP 2.11
verletzt diese Regel ohne `-r` (siehe Attribute fremder Namespaces).

## Fehlende Zeichen

Hat keine Schrift eine Glyphe für ein Zeichen, empfiehlt XSL 1.1 7.9.3 ein
sichtbares Ersatzzeichen: „the User Agent should provide a visual indication
that a character is not being displayed (for example, using the 'missing
character' glyph)“. PDF/A verbietet Verweise auf die .notdef-Glyphe
(ISO 19005-2 6.2.11.8). pdx meldet fehlende Zeichen deshalb als Fehler mit
Position, statt eine nicht konforme Datei zu erzeugen. Betroffen sind
Testfälle mit U+0085, U+007F und U+3000, für die die eingebauten
Liberation-Schriften keine Glyphe haben. U+3000 ist nach 7.17.7 kein
Wortzwischenraum, sondern ein gewöhnliches Zeichen.

## body-start()

XSL 1.1 7.30.12 definiert: „body-start() = the value of the start-indent +
start-intrusion-adjustment + the value of the provisional-distance-between-starts
of the closest ancestor fo:list-block“. Wie bei label-end(), wo eine Klammer
die Werte zusammenfasst, liest pdx alle Werte am nächsten fo:list-block. FOP
nimmt den start-indent des Listenpunkts. Unterschiede gibt es nur, wenn ein
Listenpunkt eigene Ränder, Rahmen oder Padding hat.

## Padding bei Tabellen und Zellen

Nach XSL 1.1 5.3.2 ist start-indent die Summe aus Rand, Rahmen und Padding,
das Inhaltsrechteck beginnt bei start-indent (4.2.2). pdx setzt den Inhalt
einer Tabelle oder Zelle deshalb um Rahmen plus Padding ein. FOP rechnet das
Padding zusätzlich ein, wenn margin angegeben ist, und setzt den Inhalt um das
doppelte Padding ein (Fälle table_padding_percentages und
table-cell_padding_percentages).

## Einheit px, thin, medium und thick

Für px verlangt XSL 1.1 5.9.13.1 eine geräteabhängige Größe nach der
„arc-span rule“ aus CSS2, also dem Referenzpixel. pdx nimmt wie CSS 2.1 1/96
Zoll (0,75 pt), FOP 1/72 Zoll. Die Breiten thin, medium und thick sind nach
7.8.24 Sache des Formatierers (pdx: 0,5 pt, 1 pt, 2 pt).

## Leerzeichen an Inline-Grenzen

XSL 1.1 7.16.12 fasst die Wirkung von white-space-collapse so zusammen:
Nach der Verfeinerung wird jede verbleibende Folge aus zwei oder mehr
Leerzeichen durch ein einzelnes ersetzt. Eine Grenze von fo:inline unterbricht
die Folge nicht. pdx behält das erste Leerzeichen. FOP behält vor und nach
der Grenze je eines, wenn das fo:inline einen Rahmen hat, und bricht dadurch
anders um (Fälle nist-cbpbp-border-color-inline3 und
nist-cbpbp-border-width-inline1).

## Wahl im Bereich eines Abstands

Nach XSL 1.1 4.3 begrenzen die aufgelösten Werte minimum und maximum einen
Abstand nur. Welchen Wert im Bereich der Formatierer nimmt, lässt die
Spezifikation offen, das optimum ist der bevorzugte Wert. pdx nimmt das
optimum und staucht nur, wenn ein fo:block-container mit fester Höhe sonst
überliefe. FOP staucht Abstände auch, um Seiten zu füllen (Fälle fopc-hyphen,
fopc-leader, fopc-pagelevel2 und fopc-pagelevel3). Ist nur .optimum
angegeben, bleibt .minimum beim Anfangswert 0pt (7.11.5).

## page-position="rest"

XSL 1.1 7.27.14: „This master is eligible for selection if this is not the
first page nor the last page in the page-sequence.“ Eine Vorlage mit rest
gilt also nicht für die letzte Seite. Passt keine Alternative, nimmt pdx die
zuletzt genutzte Vorlage. FOP wendet rest auch auf die letzte Seite an (Fall
list-item-with-ipd-change).
