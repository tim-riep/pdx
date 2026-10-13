// Package fonts bettet die Liberation-Schriften (SIL Open Font License 1.1)
// in das Programm ein. Sie ersetzen die PDF-Standardschriften Helvetica,
// Times und Courier metrikgleich, damit pdx nie unbettet referenziert.
package fonts

import "embed"

// FS enthält die Liberation-Schriften unter liberation/.
//
//go:embed liberation/*.ttf liberation/LICENSE
var FS embed.FS
