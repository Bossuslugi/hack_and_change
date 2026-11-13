package domain

import (
	"strings"
	"unicode"
)

// Normalize приводит текст к нижнему регистру, заменяет «ё» на «е» и схлопывает пробелы.
func Normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return strings.Join(strings.Fields(s), " ")
}

// Tokens разбивает нормализованный текст на слова (буквы и цифры).
func Tokens(s string) []string {
	return strings.FieldsFunc(Normalize(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// MatchTerm проверяет один термин триггера:
//   - термин с пробелом или дефисом — подстрока нормализованного текста;
//   - «слово$» — точное совпадение слова;
//   - иначе — префикс любого слова.
func MatchTerm(term, normalized string, tokens []string) bool {
	term = Normalize(term)
	if term == "" {
		return false
	}
	if strings.ContainsAny(term, " -") {
		return strings.Contains(normalized, term)
	}
	exact := strings.HasSuffix(term, "$")
	term = strings.TrimSuffix(term, "$")
	for _, t := range tokens {
		if exact && t == term {
			return true
		}
		if !exact && strings.HasPrefix(t, term) {
			return true
		}
	}
	return false
}

// MatchGroup проверяет группу альтернатив «a|b|c»: достаточно одного совпадения.
func MatchGroup(group, normalized string, tokens []string) bool {
	for _, term := range strings.Split(group, "|") {
		if MatchTerm(term, normalized, tokens) {
			return true
		}
	}
	return false
}

// MatchTrigger проверяет триггер: должны совпасть все группы.
func MatchTrigger(trigger []string, text string) bool {
	if len(trigger) == 0 {
		return false
	}
	normalized := Normalize(text)
	tokens := Tokens(text)
	for _, group := range trigger {
		if !MatchGroup(group, normalized, tokens) {
			return false
		}
	}
	return true
}
