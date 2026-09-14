package timemachine

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"howett.net/plist"
)

// Status is a parsed `tmutil status -X` document.
type Status struct {
	Running               bool
	Phase                 string
	Percent               float64
	RawPercent            float64
	Bytes                 int64
	TotalBytes            int64
	Files                 int64
	TotalFiles            int64
	TimeRemaining         float64
	DestinationMountPoint string
	Stopping              bool
}

func ParseStatus(data []byte) (Status, error) {
	var raw map[string]any
	if _, err := plist.Unmarshal(data, &raw); err != nil {
		return Status{}, fmt.Errorf("parse tmutil status: %w", err)
	}
	st := Status{
		Running:               asBool(raw["Running"]),
		Phase:                 asString(raw["BackupPhase"]),
		Percent:               asFloat(raw["Percent"]),
		RawPercent:            asFloat(raw["_raw_Percent"]),
		DestinationMountPoint: asString(raw["DestinationMountPoint"]),
		Stopping:              asBool(raw["Stopping"]),
	}
	if progress, ok := raw["Progress"].(map[string]any); ok {
		if st.Percent == 0 || st.Percent == -1 {
			if p := asFloat(progress["Percent"]); p != 0 {
				st.Percent = p
			}
		}
		if st.RawPercent == 0 {
			st.RawPercent = asFloat(progress["_raw_Percent"])
		}
		st.Bytes = asInt(progress["bytes"])
		st.TotalBytes = asInt(progress["totalBytes"])
		st.Files = asInt(progress["files"])
		st.TotalFiles = asInt(progress["totalFiles"])
		st.TimeRemaining = asFloat(progress["TimeRemaining"])
	}
	if st.Bytes == 0 {
		st.Bytes = asInt(raw["bytes"])
	}
	if st.TotalBytes == 0 {
		st.TotalBytes = asInt(raw["totalBytes"])
	}
	if st.Files == 0 {
		st.Files = asInt(raw["files"])
	}
	if st.TotalFiles == 0 {
		st.TotalFiles = asInt(raw["totalFiles"])
	}
	return st, nil
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case int, int8, int16, int32, int64:
		return asInt(v) != 0
	case uint, uint8, uint16, uint32, uint64:
		return asInt(v) != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes"
	default:
		return false
	}
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func asInt(v any) int64 {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int8:
		return int64(t)
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case uint:
		return int64(t)
	case uint8:
		return int64(t)
	case uint16:
		return int64(t)
	case uint32:
		return int64(t)
	case uint64:
		return int64(t)
	case float32:
		return int64(t)
	case float64:
		return int64(t)
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(s, 64)
			if ferr != nil {
				return 0
			}
			return int64(f)
		}
		return n
	default:
		return 0
	}
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float32:
		return float64(t)
	case float64:
		if math.IsNaN(t) {
			return 0
		}
		return t
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return float64(asInt(v))
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}
