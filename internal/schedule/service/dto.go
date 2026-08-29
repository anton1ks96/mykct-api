package service

import (
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// GetScheduleInput - группа, период и отбор подгрупп для выдачи расписания.
type GetScheduleInput struct {
	Group           string // Академическая группа: ИТ25-11
	Subgroup        string // Подгруппа или профиль: Подгр1, BE. Пусто или * - без отбора
	EnglishGroup    string // Подгруппа английского: A0.11. Пусто или * - все
	ProfileSubgroup string // Подгруппа внутри профиля для старших курсов
	Start           string // Начало периода, ГГГГ-ММ-ДД
	End             string // Конец периода, ГГГГ-ММ-ДД
}

// GetScheduleOutput - занятия и происхождение данных: из портала или из кэша.
type GetScheduleOutput struct {
	Events    []domain.Event // Занятия, отобранные под запрошенные подгруппы
	Source    string         // domain.SourceLive или domain.SourceCache
	FetchedAt time.Time      // Когда данные получены от портала
}

// GetClassDetailsOutput - произвольный JSON портала с деталями занятия.
type GetClassDetailsOutput struct {
	Details   map[string]any // Тело ответа портала как есть
	Source    string         // domain.SourceLive или domain.SourceCache
	FetchedAt time.Time      // Когда данные получены от портала
}
