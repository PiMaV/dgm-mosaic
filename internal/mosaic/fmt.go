package mosaic

import "fmt"

func FmtMB(nbytes int) string {
	return fmt.Sprintf("%.1f MB", float64(nbytes)/(1024*1024))
}
