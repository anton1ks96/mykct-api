package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/pkg/logger"
)

// notifyMaxAge - насколько старое неразосланное ещё рассылается. Копится оно,
// пока рассылка выключена или сервис лежал: включили - и студенты не должны
// получить пачку уведомлений о давно прошедшем.
const notifyMaxAge = 12 * time.Hour

// Типы уведомлений в data пуша: по ним клиент решает, что открыть.
const (
	pushSchedulePublished = "schedule_published"
	pushScheduleChanged   = "schedule_changed"
)

// Notifier рассылает уведомление устройствам группы. Узкий интерфейс
// потребителя: реализуется сервисом модуля уведомлений.
type Notifier interface {
	NotifyGroup(ctx context.Context, group, title, body string, data map[string]string) error
}

// NotifyPending рассылает уведомления о появившихся неделях и изменениях,
// которые воркер записал, но о которых ещё не уведомляли.
func (s *Service) NotifyPending(ctx context.Context, now time.Time) {
	op := logger.NewLogOp(ctx, log, "NotifyPending")
	since := now.Add(-notifyMaxAge)

	weeks, err := s.states.PendingPublished(ctx, since)
	if err != nil {
		op.Failed(err).Msg("failed to list published weeks to notify")
	}
	for _, week := range weeks {
		// ponytail: отметка ставится до отправки - сбой FCM теряет уведомление,
		// зато второй инстанс не шлёт дубль. Нужна доставка - заводить повторы
		claimed, err := s.states.MarkNotified(ctx, week.Group, week.WeekStart, now)
		if err != nil || !claimed {
			continue
		}
		title, body := publishedMessage(week)
		s.notify(ctx, op, week.Group, title, body, map[string]string{
			"type": pushSchedulePublished, "week_start": week.WeekStart,
		})
	}

	changes, err := s.changes.Pending(ctx, since)
	if err != nil {
		op.Failed(err).Msg("failed to list schedule changes to notify")
	}
	for _, change := range changes {
		claimed, err := s.changes.MarkNotified(ctx, change.ID, now)
		if err != nil || !claimed {
			continue
		}
		title, body := changesMessage(change)
		s.notify(ctx, op, change.Group, title, body, map[string]string{
			"type": pushScheduleChanged, "week_start": change.WeekStart,
		})
	}
}

// notify отправляет одно уведомление, ошибку только логирует: отметка уже стоит.
func (s *Service) notify(ctx context.Context, op *logger.LogOp, group, title, body string, data map[string]string) {
	if err := s.notifier.NotifyGroup(ctx, group, title, body, data); err != nil {
		op.Failed(err).Str("group", group).Str("type", data["type"]).Msg("failed to send push")
	}
}

// publishedMessage - текст уведомления о том, что выложили расписание недели.
func publishedMessage(week *domain.WeekState) (string, string) {
	return "Расписание на неделю",
		fmt.Sprintf("Выложено расписание на %s - %s", shortDate(week.WeekStart), shortDate(week.WeekEnd))
}

// fieldNames - поля занятия так, как их поймёт студент.
var fieldNames = map[string]string{
	fieldDay:      "день",
	fieldStart:    "время",
	fieldEnd:      "время",
	fieldRoom:     "аудитория",
	fieldTitle:    "предмет",
	fieldType:     "тип занятия",
	fieldSubGroup: "подгруппы",
}

// changesMessage - текст уведомления об изменениях. Одно изменение описывается
// целиком, несколько - числом и днями, иначе текст не влезет в шторку.
func changesMessage(changes *domain.WeekChanges) (string, string) {
	const title = "Изменения в расписании"

	if len(changes.Changes) == 1 {
		change := changes.Changes[0]
		event := changeEvent(change)
		when := dayLabel(event.Day)

		switch change.Kind {
		case domain.ChangeAdded:
			return title, fmt.Sprintf("Добавлена пара: %s, %s в %s", event.Title, when, event.Start)
		case domain.ChangeRemoved:
			return title, fmt.Sprintf("Отменена пара: %s, %s в %s", event.Title, when, event.Start)
		default:
			var names []string
			for _, field := range change.Fields {
				if name := fieldNames[field]; name != "" && !slices.Contains(names, name) {
					names = append(names, name)
				}
			}
			// Занятие берётся после изменения: время в тексте уже новое
			return title, fmt.Sprintf("%s, %s в %s: изменились %s",
				event.Title, when, event.Start, strings.Join(names, ", "))
		}
	}

	var days []string
	for _, change := range changes.Changes {
		if day := changeEvent(change).Day; !slices.Contains(days, day) {
			days = append(days, day)
		}
	}
	slices.Sort(days)
	for i, day := range days {
		days[i] = dayLabel(day)
	}

	return title, fmt.Sprintf("Изменений: %d - %s", len(changes.Changes), strings.Join(days, ", "))
}

// weekdayShort - дни недели по-русски, индекс - time.Weekday.
var weekdayShort = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

// dayLabel переводит "2026-09-22" в "пн 22.09". Непонятная дата уходит как есть.
func dayLabel(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}

	return weekdayShort[t.Weekday()] + " " + t.Format("02.01")
}

// shortDate переводит "2026-09-22" в "22.09". Непонятная дата уходит как есть.
func shortDate(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}

	return t.Format("02.01")
}
