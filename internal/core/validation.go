package core

import (
	"math"
	"path/filepath"
	"strings"
)

func finiteNonnegative(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 }
func NormalizeRequest(r Request) (Request, error) {
	r.URL = strings.TrimSpace(r.URL)
	if err := Validate(r); err != nil {
		return r, &Failure{Code: CodeValidation, Message: err.Error(), Cause: err}
	}
	out, err := filepath.Abs(r.Out)
	if err != nil {
		return r, err
	}
	r.Out = out
	r.Codec = VideoCodec(r)
	r.Container = VideoContainer(r)
	return r, nil
}
func ValidateMediaInfo(m MediaInfo) error {
	if !finiteNonnegative(m.Duration) {
		return ContractError("adapter returned invalid media duration")
	}
	if m.Live || m.Type == "playlist" || m.Type == "multi_video" {
		return ContractError("adapter must return one non-live media item")
	}
	for _, f := range m.Formats {
		if f.Height < 0 || f.Height > 16384 {
			return ContractError("adapter returned invalid format height")
		}
	}
	return nil
}
func ValidateMetrics(m Metrics) error {
	if m.DownloadedBytes < 0 || m.TotalBytes != nil && *m.TotalBytes < 0 {
		return ContractError("adapter returned negative byte counts")
	}
	for _, p := range []*float64{m.SpeedBPS, m.ETASeconds, m.Percent} {
		if p != nil && !finiteNonnegative(*p) {
			return ContractError("adapter returned invalid numerical metrics")
		}
	}
	if m.Percent != nil && *m.Percent > 100 {
		return ContractError("adapter progress exceeds 100 percent")
	}
	return nil
}
func ValidateResult(r Request, result Result) error {
	if result.Path == "" || !filepath.IsAbs(result.Path) {
		return ContractError("adapter result must contain an absolute output path")
	}
	rel, err := filepath.Rel(r.Out, result.Path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ContractError("adapter result is outside the requested output directory")
	}
	return nil
}
func StageOrder(s Stage) int {
	switch s {
	case Downloading:
		return 1
	case Processing:
		return 2
	case Verifying:
		return 3
	}
	return 0
}
