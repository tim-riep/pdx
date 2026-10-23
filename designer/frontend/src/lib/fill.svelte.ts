// Inputs of the fillable preview. They stay in the UI only and
// do not go into the template or the data.

export const fill = $state({ values: {} as Record<string, string> });

// fieldKey is the key of a field: name and page, for repeated
// fields also the position on the page.
export function fieldKey(name: string, page: number, i: number, radio = false): string {
  return radio ? `${name}@${page}` : `${name}@${page}#${i}`;
}
