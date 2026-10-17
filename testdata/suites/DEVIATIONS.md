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

`border-collapse="collapse"` ist der Anfangswert (XSL 1.1 7.26.1). pdx löst
jede Zellkante zwischen Zelle, Nachbarzelle, Zeile und an den Außenkanten
der Tabelle nach Breite und dann nach Stilrang auf (6.7.10). Rahmen von
Spalten und Abschnitten (fo:table-column, fo:table-body) gehen noch nicht in
die Auflösung ein. Das Modell `collapse-with-precedence` wird wie `collapse`
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
