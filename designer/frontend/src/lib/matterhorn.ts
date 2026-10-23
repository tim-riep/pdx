// Manual checkpoints of the Matterhorn Protocol 1.1 (© 2021 PDF Association,
// PDF/UA TWG, license CC BY 4.0, https://pdfa.org/resource/the-matterhorn-protocol/).
// The texts are our own German versions. The official count names 47
// human checks, the table of conditions contains 48, all of them are
// listed here. na gives the reason why an item does not apply to pdx.

export interface Check {
  id: string;
  clause: string; // ISO 14289-1
  text: string;
  na?: string;
}

const noJS = "pdx schreibt kein JavaScript";
const noMedia = "pdx bettet keine Audio- oder Videoinhalte ein";
const noOCR = "pdx erzeugt Text, kein OCR";

export const checks: Check[] = [
  { id: "01-001", clause: "7.1-1", text: "Nichts Dekoratives ist als Inhalt getaggt." },
  { id: "01-002", clause: "7.1-1", text: "Kein echter Inhalt ist als Dekoration (Artefakt) markiert." },
  { id: "01-006", clause: "7.1-2", text: "Typ und Attribute jedes Strukturelements passen zum Inhalt." },
  { id: "02-002", clause: "7.1-3", text: "Eigene Strukturtypen sind sinnvoll auf Standardtypen abgebildet.", na: "pdx verwendet nur Standardtypen" },
  { id: "03-001", clause: "7.1-5", text: "Keine Aktion führt zu Flackern.", na: noJS },
  { id: "03-002", clause: "7.1-5", text: "Kein Multimedia-Inhalt flackert.", na: noMedia },
  { id: "03-003", clause: "7.1-5", text: "Keine JavaScript-Aktion führt zu Flackern.", na: noJS },
  { id: "04-001", clause: "7.1-6", text: "Information wird nicht allein über Kontrast, Farbe, Format oder Position vermittelt." },
  { id: "05-001", clause: "7.1-7", text: "Audioinhalte in Medien haben eine Textalternative.", na: noMedia },
  { id: "05-002", clause: "7.1-7", text: "Audio-Annotationen haben eine Textalternative.", na: noMedia },
  { id: "05-003", clause: "7.1-7", text: "Signaltöne aus JavaScript haben eine Alternative.", na: noJS },
  { id: "06-004", clause: "7.1-8", text: "Der Titel (dc:title) beschreibt das Dokument eindeutig." },
  { id: "08-001", clause: "7.1-10", text: "Per OCR erkannter Text ist weitgehend fehlerfrei.", na: noOCR },
  { id: "08-002", clause: "7.1-10", text: "Per OCR erkannter Text ist getaggt.", na: noOCR },
  { id: "09-001", clause: "7.2-1", text: "Die Tags stehen in logischer Lesereihenfolge." },
  { id: "09-002", clause: "7.2-1", text: "Die Verschachtelung der Strukturelemente ist inhaltlich sinnvoll." },
  { id: "09-003", clause: "7.2-1", text: "Jeder Strukturtyp passt zum Inhalt, den er auszeichnet." },
  { id: "11-007", clause: "7.2-3", text: "Die angegebene Sprache stimmt mit dem Text überein." },
  { id: "12-001", clause: "7.2-4", text: "Gedehnte Zeichen (etwa Klammern über mehrere Zeilen) sind passend wiedergegeben." },
  { id: "13-001", clause: "7.3-1", text: "Grafiken mit Bedeutung sind als Abbildung getaggt." },
  { id: "13-002", clause: "7.3-1", text: "Ein Verweis auf einem bedeutsamen Hintergrund hat einen Alternativtext." },
  { id: "13-003", clause: "7.3-2", text: "Bildunterschriften sind als Beschriftung getaggt." },
  { id: "13-005", clause: "7.3-4", text: "ActualText steht nicht dort, wo ein Alternativtext gemeint ist." },
  { id: "13-006", clause: "7.3-5", text: "Grafiken mit Bedeutung nur in ihrer Gruppe sind gemeinsam getaggt." },
  { id: "13-007", clause: "7.3-6", text: "Wo möglich wird eine zugänglichere Darstellung verwendet (Text statt Bild von Text)." },
  { id: "13-008", clause: "7.3-4", text: "Eine Abbildung, die Text darstellt, hat ActualText." },
  { id: "14-001", clause: "7.4-1", text: "Überschriften sind als Überschrift getaggt." },
  { id: "14-004", clause: "7.4.3-1", text: "Nummerierte Überschriften nutzen arabische Ziffern (H1 bis H6)." },
  { id: "14-005", clause: "7.4.3-1", text: "Eine siebte oder tiefere Gliederungsebene ist passend ausgezeichnet." },
  { id: "15-001", clause: "7.5-1", text: "Zeilenköpfe sind als Kopfzellen getaggt." },
  { id: "15-002", clause: "7.5-1", text: "Spaltenköpfe sind als Kopfzellen getaggt." },
  { id: "15-004", clause: "7.5-3", text: "Nur echte Tabellen sind als Tabelle getaggt, keine Layout-Raster." },
  { id: "15-005", clause: "7.5-2", text: "Jede Zelle lässt sich eindeutig ihren Kopfzellen zuordnen." },
  { id: "16-001", clause: "7.6-1", text: "Geordnete Listen geben ihre Nummerierung an." },
  { id: "16-002", clause: "7.6-1", text: "Die Nummerierung einer geordneten Liste passt zu den Marken." },
  { id: "16-003", clause: "7.6-2", text: "Was eine Liste ist, ist als Liste getaggt." },
  { id: "17-001", clause: "7.7-1", text: "Mathematische Ausdrücke sind als Formel getaggt." },
  { id: "18-001", clause: "7.8-1", text: "Kopf- und Fußzeilen sind als Seitengestaltung markiert." },
  { id: "18-002", clause: "7.8-1", text: "Kopf- und Fußzeilen-Artefakte sind richtig eingeordnet." },
  { id: "19-001", clause: "7.9-1", text: "Fußnoten und Endnoten sind als Fußnote getaggt." },
  { id: "19-002", clause: "7.9-1", text: "Querverweise sind als Querverweis getaggt." },
  { id: "22-001", clause: "7.12-1", text: "Artikelketten folgen der logischen Lesereihenfolge.", na: "pdx erzeugt keine Artikelketten" },
  { id: "24-001", clause: "7.14-1", text: "Nicht interaktive Formulare sind passend getaggt.", na: "pdx erzeugt keine Formularfelder" },
  { id: "28-001", clause: "7.18.1-2", text: "Annotationen (etwa Verweise) stehen an der richtigen Stelle der Lesereihenfolge." },
  { id: "28-003", clause: "7.18.1-3", text: "Annotationen zur reinen Gestaltung sind nicht als Inhalt getaggt.", na: "pdx erzeugt nur Verweis-Annotationen" },
  { id: "28-013", clause: "7.18.5-3", text: "Image-Maps (IsMap) haben eine Textalternative.", na: "pdx erzeugt keine Image-Maps" },
  { id: "29-001", clause: "7.19-1", text: "Kein Skript verlangt bestimmte Zeiten für Tastenanschläge.", na: noJS },
  { id: "31-010", clause: "7.21.4.1-2", text: "Eingebettete Schriften dürfen rechtlich eingebettet werden." },
];
