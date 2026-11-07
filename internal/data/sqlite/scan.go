package sqlite

import "time"

func asInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case bool:
		if n {
			return 1
		}
	}
	return 0
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	}
	return 0
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBool(v any) bool { return asInt(v) != 0 }

func asIntPtr(v any) *int {
	if v == nil {
		return nil
	}
	n := int(asInt(v))
	return &n
}

func asFloatPtr(v any) *float64 {
	if v == nil {
		return nil
	}
	n := asFloat(v)
	return &n
}

func asBoolPtr(v any) *bool {
	if v == nil {
		return nil
	}
	b := asBool(v)
	return &b
}

func asTimePtr(v any) *time.Time {
	if v == nil {
		return nil
	}
	n, ok := v.(int64)
	if !ok {
		if f, isFloat := v.(float64); isFloat {
			n = int64(f)
		} else {
			return nil
		}
	}
	t := time.Unix(n, 0).UTC()
	return &t
}

func asTime(v any) time.Time {
	if t := asTimePtr(v); t != nil {
		return *t
	}
	return time.Time{}
}
