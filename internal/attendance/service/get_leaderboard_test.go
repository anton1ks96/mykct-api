package service

import (
	"cmp"
	"fmt"
	"slices"
	"testing"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// TestCourseFromGroup - курс выводится из года набора, а мусорная группа не
// должна превращаться в курс, в который попадут чужие люди.
func TestCourseFromGroup(t *testing.T) {
	tests := []struct {
		name  string
		group string
		want  string
	}{
		{"обычная группа", "ИТ25-11", "ИТ25"},
		{"другая группа того же курса", "ИТ25-14", "ИТ25"},
		{"пробелы по краям", "  ИТ25-11  ", "ИТ25"},
		{"нижний регистр", "ит25-11", "ИТ25"},
		{"без дефиса", "ИТ25", ""},
		{"пустая строка", "", ""},
		{"пустой префикс", "-11", ""},
		{"лишний дефис", "ИТ25-11-2", "ИТ25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := courseFromGroup(tt.group); got != tt.want {
				t.Errorf("courseFromGroup(%q) = %q, want %q", tt.group, got, tt.want)
			}
		})
	}
}

// cohort собирает когорту из пар логин-серия.
func cohort(pairs ...any) []domain.Participant {
	participants := make([]domain.Participant, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		participants = append(participants, domain.Participant{
			Login:         pairs[i].(string),
			CurrentStreak: pairs[i+1].(int),
		})
	}

	return participants
}

// TestRankCohortOrder - рейтинг идёт от длинной серии к короткой, места
// спортивные: равные серии делят место, следующее за ними - со сдвигом.
func TestRankCohortOrder(t *testing.T) {
	board := rankCohort(
		cohort("a", 3, "b", 9, "c", 5, "d", 5, "e", 1),
		"e",
		newAliasMaker(testAliasSecret, "ИТ25"),
		10,
	)

	wantStreaks := []int{9, 5, 5, 3, 1}
	wantRanks := []int{1, 2, 2, 4, 5}

	if len(board.Top) != len(wantStreaks) {
		t.Fatalf("len(Top) = %d, want %d", len(board.Top), len(wantStreaks))
	}
	for i := range wantStreaks {
		if board.Top[i].CurrentStreak != wantStreaks[i] {
			t.Errorf("Top[%d].CurrentStreak = %d, want %d", i, board.Top[i].CurrentStreak, wantStreaks[i])
		}
		if board.Top[i].Rank != wantRanks[i] {
			t.Errorf("Top[%d].Rank = %d, want %d", i, board.Top[i].Rank, wantRanks[i])
		}
	}
	if board.Participants != 5 {
		t.Errorf("Participants = %d, want 5", board.Participants)
	}
}

// TestRankCohortTopSize - топ обрезается, а размер когорты остаётся полным:
// студенту важно видеть, среди скольких человек он соревнуется.
func TestRankCohortTopSize(t *testing.T) {
	participants := make([]domain.Participant, 0, 25)
	for i := range 25 {
		participants = append(participants, domain.Participant{
			Login:         fmt.Sprintf("i25s%04d", i),
			CurrentStreak: i,
		})
	}

	board := rankCohort(participants, "i25s0000", newAliasMaker(testAliasSecret, "ИТ25"), 10)

	if len(board.Top) != 10 {
		t.Errorf("len(Top) = %d, want 10", len(board.Top))
	}
	if board.Participants != 25 {
		t.Errorf("Participants = %d, want 25", board.Participants)
	}
}

// TestRankCohortMeOutsideTop - собственная строка отдаётся, даже когда студент
// далеко за топом.
func TestRankCohortMeOutsideTop(t *testing.T) {
	board := rankCohort(
		cohort("a", 10, "b", 9, "c", 8, "me", 1),
		"me",
		newAliasMaker(testAliasSecret, "ИТ25"),
		2,
	)

	if !board.Me.IsMe {
		t.Fatal("Me.IsMe = false, want true")
	}
	if board.Me.Rank != 4 {
		t.Errorf("Me.Rank = %d, want 4", board.Me.Rank)
	}
	if board.Me.CurrentStreak != 1 {
		t.Errorf("Me.CurrentStreak = %d, want 1", board.Me.CurrentStreak)
	}
	for _, entry := range board.Top {
		if entry.IsMe {
			t.Error("Top contains the caller, want them outside the top")
		}
	}
}

// TestRankCohortMeInsideTop - находясь в топе, студент помечен и там, и в
// собственной строке.
func TestRankCohortMeInsideTop(t *testing.T) {
	board := rankCohort(
		cohort("me", 10, "b", 9, "c", 8),
		"me",
		newAliasMaker(testAliasSecret, "ИТ25"),
		10,
	)

	if !board.Top[0].IsMe {
		t.Error("Top[0].IsMe = false, want true")
	}
	if board.Me.Alias != board.Top[0].Alias {
		t.Errorf("Me.Alias = %q, want %q", board.Me.Alias, board.Top[0].Alias)
	}
}

// TestRankCohortTiesOrderedBySeed - при равных сериях порядок обязан совпадать с
// порядком по HMAC-seed. Проверка именно на совпадение с seed, а не на отличие
// от алфавита: сортировка по номеру студенческого отличается от алфавитной лишь
// местами, и проверка "не алфавит" пропустила бы её, а вместе с ней и утечку
// порядка зачисления.
func TestRankCohortTiesOrderedBySeed(t *testing.T) {
	aliases := newAliasMaker(testAliasSecret, "ИТ25")

	logins := make([]string, 0, 40)
	participants := make([]domain.Participant, 0, 40)
	for i := range 40 {
		login := fmt.Sprintf("i25s%04d", i)
		logins = append(logins, login)
		participants = append(participants, domain.Participant{Login: login, CurrentStreak: 7})
	}

	// Ожидаемый порядок считается независимо от rankCohort
	wantLogins := slices.Clone(logins)
	slices.SortFunc(wantLogins, func(a, b string) int {
		return cmp.Compare(aliases.seed(a), aliases.seed(b))
	})

	board := rankCohort(participants, logins[0], aliases, len(logins))

	for i, login := range wantLogins {
		if board.Top[i].Alias != aliases.alias(login) {
			t.Fatalf("Top[%d].Alias = %q, want the entry seeded at position %d",
				i, board.Top[i].Alias, i)
		}
	}

	// Порядок по seed не обязан отличаться от алфавитного в каждой паре, но
	// совпасть целиком на сорока логинах он может лишь случайно
	if slices.Equal(wantLogins, logins) {
		t.Error("seed order equals login order, the fixture no longer proves anything")
	}

	for i, entry := range board.Top {
		if entry.Rank != 1 {
			t.Errorf("Top[%d].Rank = %d, want 1 for equal streaks", i, entry.Rank)
		}
	}
}

// TestRankCohortSingleParticipant - когорта из одного человека не должна падать.
func TestRankCohortSingleParticipant(t *testing.T) {
	board := rankCohort(cohort("me", 4), "me", newAliasMaker(testAliasSecret, "ИТ25"), 10)

	if len(board.Top) != 1 || board.Top[0].Rank != 1 {
		t.Fatalf("Top = %+v, want a single entry ranked first", board.Top)
	}
	if !board.Me.IsMe || board.Me.Rank != 1 {
		t.Errorf("Me = %+v, want the caller ranked first", board.Me)
	}
}

// TestRankCohortStable - порядок рейтинга воспроизводим между вызовами.
func TestRankCohortStable(t *testing.T) {
	participants := cohort("a", 5, "b", 5, "c", 5, "d", 5)
	aliases := newAliasMaker(testAliasSecret, "ИТ25")

	first := rankCohort(participants, "a", aliases, 10)
	second := rankCohort(participants, "a", aliases, 10)

	for i := range first.Top {
		if first.Top[i].Alias != second.Top[i].Alias {
			t.Fatalf("Top[%d] = %q, then %q: ranking is not stable",
				i, first.Top[i].Alias, second.Top[i].Alias)
		}
	}
}
