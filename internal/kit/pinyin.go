package kit

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

// PinyinCamelCase converts Chinese characters in s to CamelCase pinyin
// syllables (e.g. "卡通" -> "KaTong", "标签组 1" -> "BiaoQianZu 1") while
// preserving non-Chinese characters.
func PinyinCamelCase(s string) string {
	if s == "" {
		return ""
	}
	args := pinyin.NewArgs()
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		syllables := pinyin.SinglePinyin(r, args)
		if len(syllables) > 0 && syllables[0] != "" {
			b.WriteString(capitalizeASCII(syllables[0]))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// PinyinCategory returns the uppercase single-letter index category (A-Z) for
// a tag or name, preferring pinyinStr when available and falling back to name.
// Non-alphabetic or empty inputs return "#".
func PinyinCategory(name, pinyinStr string) string {
	for _, candidate := range []string{pinyinStr, name} {
		for _, r := range candidate {
			if unicode.IsSpace(r) {
				continue
			}
			if r >= 'a' && r <= 'z' {
				return string(r - ('a' - 'A'))
			}
			if r >= 'A' && r <= 'Z' {
				return string(r)
			}
			break
		}
	}
	return "#"
}

// ExtractPinyin returns both the CamelCase pinyin representation and the A-Z/#
// category letter for name.
func ExtractPinyin(name string) (pinyinStr, category string) {
	pinyinStr = PinyinCamelCase(name)
	category = PinyinCategory(name, pinyinStr)
	return pinyinStr, category
}

func capitalizeASCII(s string) string {
	if s == "" {
		return ""
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

// PinyinSearchIndex converts s containing Chinese characters into Refto's
// "<full-lowercase> <initials-lowercase>" search index format (e.g. "建筑结构" ->
// "jianzhujiegou jzjg"). Returns "" if s contains no Chinese characters.
func PinyinSearchIndex(s string) string {
	if s == "" {
		return ""
	}
	args := pinyin.NewArgs()
	var full, initials strings.Builder
	hasHan := false

	for _, r := range s {
		syllables := pinyin.SinglePinyin(r, args)
		if len(syllables) > 0 && syllables[0] != "" {
			hasHan = true
			syl := strings.ToLower(syllables[0])
			full.WriteString(syl)
			initials.WriteByte(syl[0])
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			full.WriteRune(r)
			initials.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			lr := r + ('a' - 'A')
			full.WriteRune(lr)
			initials.WriteRune(lr)
		}
	}

	if !hasHan {
		return ""
	}
	return full.String() + " " + initials.String()
}
