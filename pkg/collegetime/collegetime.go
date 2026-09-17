// Package collegetime содержит время колледжа: часовой пояс и границы учебной недели.
package collegetime

import "time"

// TZ - часовой пояс колледжа, Екатеринбург без перехода на летнее время.
// Фиксированное смещение не требует tzdata в контейнере.
var TZ = time.FixedZone("YEKT", 5*60*60)

// NextWeek возвращает понедельник и воскресенье следующей недели по времени
// колледжа, в формате ГГГГ-ММ-ДД.
func NextWeek(now time.Time) (start, end string) {
	monday := NextMonday(now)

	return monday.Format(time.DateOnly), monday.AddDate(0, 0, 6).Format(time.DateOnly)
}

// NextMonday возвращает полночь ближайшего будущего понедельника по времени
// колледжа. Для самого понедельника это следующий понедельник, не сегодняшний.
func NextMonday(now time.Time) time.Time {
	now = now.In(TZ)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, TZ)

	offset := (8 - int(midnight.Weekday())) % 7
	if offset == 0 {
		offset = 7
	}

	return midnight.AddDate(0, 0, offset)
}
