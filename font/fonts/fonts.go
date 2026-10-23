// Package fonts embeds the Liberation fonts (SIL Open Font License 1.1)
// into the program. They replace the PDF standard fonts Helvetica,
// Times and Courier with matching metrics, so that pdx never references
// a font without embedding it.
package fonts

import "embed"

// FS holds the Liberation fonts under liberation/.
//
//go:embed liberation/*.ttf liberation/LICENSE
var FS embed.FS
