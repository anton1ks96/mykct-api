package service

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// testAliasSecret - секрет фиксирован, иначе тесты проверяли бы не то.
var testAliasSecret = []byte("0123456789abcdef0123456789abcdef")

// aliasFormat - псевдоним всегда два слова и шестнадцатеричный суффикс.
var aliasFormat = regexp.MustCompile(`^[А-ЯЁ][а-яё]+ [А-ЯЁ][а-яё]+ 0x[0-9A-F]{3}$`)

// TestAliasDeterministic - один логин с одним секретом и курсом даёт один и тот
// же псевдоним: студент не должен переименовываться между запросами.
func TestAliasDeterministic(t *testing.T) {
	maker := newAliasMaker(testAliasSecret, "ИТ25")

	first := maker.alias("i25s0042")
	for range 10 {
		if got := maker.alias("i25s0042"); got != first {
			t.Fatalf("alias() = %q, want %q on repeated calls", got, first)
		}
	}
}

// TestAliasDependsOnSecret - смена секрета меняет псевдоним. Тест ловит регресс
// "секрет прокинули, но забыли использовать": без него псевдонимы сводятся к
// логинам перебором, и вся анонимность рейтинга исчезает молча.
func TestAliasDependsOnSecret(t *testing.T) {
	other := newAliasMaker([]byte("fedcba9876543210fedcba9876543210"), "ИТ25")

	if newAliasMaker(testAliasSecret, "ИТ25").alias("i25s0042") == other.alias("i25s0042") {
		t.Error("alias() ignores the secret")
	}
}

// TestAliasDependsOnCourse - один логин в разных областях рейтинга получает
// разные псевдонимы, иначе пересечение таблиц выдаёт группу студента.
func TestAliasDependsOnCourse(t *testing.T) {
	if newAliasMaker(testAliasSecret, "ИТ25").alias("i25s0042") ==
		newAliasMaker(testAliasSecret, "ИТ24").alias("i25s0042") {
		t.Error("alias() ignores the course")
	}
}

// TestAliasFormat - форма псевдонима и отсутствие в нём логина.
func TestAliasFormat(t *testing.T) {
	maker := newAliasMaker(testAliasSecret, "ИТ25")

	for i := range 100 {
		login := fmt.Sprintf("i25s%04d", i)
		alias := maker.alias(login)

		if !aliasFormat.MatchString(alias) {
			t.Errorf("alias(%q) = %q, does not match the expected format", login, alias)
		}
		if strings.Contains(alias, login) {
			t.Errorf("alias(%q) = %q leaks the login", login, alias)
		}
	}
}

// TestAliasNoCollisionsInCohort - на потоке в 300 человек псевдонимы различны.
// Падение теста означает, что словари пора расширять, а не править тест.
func TestAliasNoCollisionsInCohort(t *testing.T) {
	maker := newAliasMaker(testAliasSecret, "ИТ25")

	seen := make(map[string]string, 300)
	for i := range 300 {
		login := fmt.Sprintf("i25s%04d", i)
		alias := maker.alias(login)
		if other, busy := seen[alias]; busy {
			t.Errorf("alias %q is shared by %s and %s", alias, other, login)
		}
		seen[alias] = login
	}
}

// TestAliasSeedStable - порядок внутри равных серий воспроизводим и различает
// студентов.
func TestAliasSeedStable(t *testing.T) {
	maker := newAliasMaker(testAliasSecret, "ИТ25")

	if maker.seed("i25s0042") != maker.seed("i25s0042") {
		t.Error("seed() is not stable for the same login")
	}
	if maker.seed("i25s0042") == maker.seed("i25s0043") {
		t.Error("seed() collides for different logins")
	}
}

// TestResolveAliasCollisions - совпавшие псевдонимы разводятся суффиксом, первый
// по рейтингу остаётся нетронутым, порядок строк не меняется.
func TestResolveAliasCollisions(t *testing.T) {
	tests := []struct {
		name    string
		aliases []string
		want    []string
	}{
		{
			name:    "без совпадений",
			aliases: []string{"Быстрый Байт 0x001", "Умный Массив 0x002"},
			want:    []string{"Быстрый Байт 0x001", "Умный Массив 0x002"},
		},
		{
			name:    "пара совпадений",
			aliases: []string{"Быстрый Байт 0x001", "Быстрый Байт 0x001"},
			want:    []string{"Быстрый Байт 0x001", "Быстрый Байт 0x002"},
		},
		{
			name:    "три подряд",
			aliases: []string{"Умный Стек 0xFFF", "Умный Стек 0xFFF", "Умный Стек 0xFFF"},
			want:    []string{"Умный Стек 0xFFF", "Умный Стек 0x000", "Умный Стек 0x001"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := make([]domain.Entry, 0, len(tt.aliases))
			for i, alias := range tt.aliases {
				entries = append(entries, domain.Entry{Rank: i + 1, Alias: alias})
			}

			resolveAliasCollisions(entries)

			for i, want := range tt.want {
				if entries[i].Alias != want {
					t.Errorf("entry %d alias = %q, want %q", i, entries[i].Alias, want)
				}
				if entries[i].Rank != i+1 {
					t.Errorf("entry %d rank = %d, want %d", i, entries[i].Rank, i+1)
				}
			}
		})
	}
}

// TestAliasWordsDistinct - повтор слова в словаре сужает пространство молча.
func TestAliasWordsDistinct(t *testing.T) {
	for name, words := range map[string][]string{
		"aliasAdjectives": aliasAdjectives[:],
		"aliasNouns":      aliasNouns[:],
	} {
		seen := make(map[string]struct{}, len(words))
		for _, word := range words {
			if _, busy := seen[word]; busy {
				t.Errorf("%s contains a duplicate: %q", name, word)
			}
			seen[word] = struct{}{}
		}
	}
}
