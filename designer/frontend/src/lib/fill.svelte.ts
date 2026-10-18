// Eingaben der ausfüllbaren Vorschau. Sie bleiben nur in der Oberfläche und
// gehen nicht in Vorlage oder Daten.

export const fill = $state({ values: {} as Record<string, string> });

// fieldKey ist der Schlüssel eines Felds: Name und Seite, bei wiederholten
// Feldern zusätzlich die Stelle auf der Seite.
export function fieldKey(name: string, page: number, i: number, radio = false): string {
  return radio ? `${name}@${page}` : `${name}@${page}#${i}`;
}
