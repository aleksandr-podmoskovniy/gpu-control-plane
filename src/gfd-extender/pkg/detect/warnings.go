package detect

import "fmt"

// warningCollector helps accumulate human-readable warnings during detection.
type warningCollector []string

func (w *warningCollector) addf(format string, args ...interface{}) {
	*w = append(*w, fmt.Sprintf(format, args...))
}
