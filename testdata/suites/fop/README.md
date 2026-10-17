# Testfälle aus Apache FOP 2.11

Herkunft: `fop-2.11-src.tar.gz` von archive.apache.org (SHA-512
`9eacdee6d81ecf5af02c6266d20f55642febfa10bc9f3754cfb4d1532743899759d6bc7d362e80f3a1d8d370c54485e455d0ca3eac8b6604a05d3326ffefa00c`),
Verzeichnis `fop/test/layoutengine/standard-testcases`. Lizenz: Apache License
2.0, siehe `LICENSE` und `NOTICE` in diesem Verzeichnis. Jede Datei unter
`cases` behält den Lizenzkopf des Testfalls.

`cases` enthält den FO-Teil der Testfälle im Teilumfang von pdx, Variablen
(`##name`) sind eingesetzt wie bei FOPs `testcase2fo.xsl`. Die Prüfungen von
FOP (`<checks>`) beziehen sich auf FOPs Area Tree und werden nicht
übernommen. Stattdessen liegt unter `expect` je Fall, was FOP setzt:
Seitenzahl, Zeilentexte, Grundlinien und Zeilenanfänge in Punkt.

Fälle außerhalb des Teilumfangs, Fälle, die FOP selbst deaktiviert, und Fälle,
die Bilder aus FOPs Testverzeichnis brauchen, stehen mit Begründung in
`../disabled.txt`.

Neu importieren und Erwartungen erzeugen:

```bash
PDX_SUITE_IMPORT_FOP=<pfad>/fop-2.11/fop/test/layoutengine go test -run TestImportFOP ./internal/fosuite
```

```bash
make fo-reference
```
