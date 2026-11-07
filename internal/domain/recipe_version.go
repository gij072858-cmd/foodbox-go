package domain

import (
	"regexp"
	"strings"
)

// trailingVersionLabel matches a version note at the very end of a dish
// name: 口水鸡（整鸡版） and 口水鸡(整鸡版) both split into 口水鸡 and 整鸡版.
// The note must end the name; a parenthetical inside the name body is part
// of the dish's own name, not a version.
var trailingVersionLabel = regexp.MustCompile(`^(.+?)\s*[（(]([^（）()]+)[)）]$`)

// VersionGroupOf returns the group a recipe versions itself under.
//
// The stored version_group wins when set; otherwise the seed's fallback
// rule applies: strip a trailing parenthetical note from the name, so
// 口水鸡（整鸡版） joins the 口水鸡 group. A dish with neither has only
// itself in its group.
func VersionGroupOf(r Recipe) string {
	if group := strings.TrimSpace(r.VersionGroup); group != "" {
		return group
	}
	if m := trailingVersionLabel.FindStringSubmatch(strings.TrimSpace(r.Name)); m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(r.Name)
}

// VariantLabelOf returns the version note at the end of a dish name, "" for
// a name without one. The pairing with VersionGroupOf is what lets a version
// switcher show 「整鸡版」 instead of the whole name.
func VariantLabelOf(r Recipe) string {
	if m := trailingVersionLabel.FindStringSubmatch(strings.TrimSpace(r.Name)); m != nil {
		return strings.TrimSpace(m[2])
	}
	return ""
}
