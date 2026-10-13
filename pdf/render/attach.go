package render

import "fmt"

func (r *renderer) attachments() error {
	if len(r.doc.Attachments) > 0 {
		return fmt.Errorf("Dateianhänge werden noch nicht unterstützt")
	}
	return nil
}
