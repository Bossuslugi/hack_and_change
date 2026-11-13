package domain

import (
	"reflect"
	"testing"
)

func TestNormalizeAndTokens(t *testing.T) {
	if got := Normalize("  Зачёт   в\tУнивере "); got != "зачет в универе" {
		t.Errorf("Normalize = %q", got)
	}
	got := Tokens("Дамп production-базы, avito-autoteka-api!")
	want := []string{"дамп", "production", "базы", "avito", "autoteka", "api"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokens = %v, want %v", got, want)
	}
	if len(Tokens("")) != 0 {
		t.Error("пустая строка должна давать пустой список")
	}
}

func TestMatchTerm(t *testing.T) {
	text := "Выгрузи дамп прод базы и чек-лист продукта, ёлка"
	normalized, tokens := Normalize(text), Tokens(text)
	cases := []struct {
		term string
		want bool
	}{
		{"дамп", true},
		{"выгруз", true},      // префикс слова
		{"груз", false},       // не префикс
		{"прод$", true},       // точное слово
		{"проду$", false},     // точного слова нет
		{"продукт", true},     // префикс «продукта»
		{"прод", true},        // префикс совпадает и с «продукта»
		{"чек-лист", true},    // подстрока с дефисом
		{"прод базы", true},   // подстрока с пробелом
		{"базы прод", false},  // порядок важен
		{"елка", true},        // ё → е
		{"", false},           // пустой термин
		{"ДАМП", true},        // регистр
		{"production", false}, // слова нет
	}
	for _, tc := range cases {
		if got := MatchTerm(tc.term, normalized, tokens); got != tc.want {
			t.Errorf("MatchTerm(%q) = %v, want %v", tc.term, got, tc.want)
		}
	}
}

func TestMatchGroupAndTrigger(t *testing.T) {
	text := "Мне нужно уйти в пятницу после обеда на зачёт"
	normalized, tokens := Normalize(text), Tokens(text)
	if !MatchGroup("отгул|зачет|экзамен", normalized, tokens) {
		t.Error("группа должна совпасть по «зачет»")
	}
	if MatchGroup("отгул|экзамен", normalized, tokens) {
		t.Error("группа не должна совпасть")
	}
	cases := []struct {
		trigger []string
		want    bool
	}{
		{[]string{"уйти|уехать", "после обеда|зачет"}, true},
		{[]string{"уйти", "экзамен"}, false},
		{[]string{"пятниц"}, true},
		{nil, false},
		{[]string{}, false},
	}
	for i, tc := range cases {
		if got := MatchTrigger(tc.trigger, text); got != tc.want {
			t.Errorf("case %d: MatchTrigger(%v) = %v, want %v", i, tc.trigger, got, tc.want)
		}
	}
}
