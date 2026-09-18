// Package collegetime содержит время колледжа: часовой пояс и границы учебной недели.
package collegetime

import "time"

// tz - часовой пояс колледжа, Екатеринбург без перехода на летнее время.
// Фиксированное смещение не требует tzdata в контейнере.
var tz = time.FixedZone("YEKT", 5*60*60)

// TZ возвращает часовой пояс колледжа. Функция, а не переменная: от пояса
// зависят и границы недели, и учебный год, переприсвоить его извне нельзя.
func TZ() *time.Location {
	return tz
}

// NextWeek возвращает понедельник и воскресенье следующей недели по времени
// колледжа, в формате ГГГГ-ММ-ДД.
func NextWeek(now time.Time) (start, end string) {
	monday := NextMonday(now)

	return monday.Format(time.DateOnly), monday.AddDate(0, 0, 6).Format(time.DateOnly)
}

// NextMonday возвращает полночь ближайшего будущего понедельника по времени
// колледжа. Для самого понедельника это следующий понедельник, не сегодняшний.
func NextMonday(now time.Time) time.Time {
	now = now.In(tz)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)

	offset := (8 - int(midnight.Weekday())) % 7
	if offset == 0 {
		offset = 7
	}

	return midnight.AddDate(0, 0, offset)
}

// CurrentWeek возвращает понедельник и воскресенье текущей недели по времени
// колледжа, в формате ГГГГ-ММ-ДД.
func CurrentWeek(now time.Time) (start, end string) {
	monday := CurrentMonday(now)

	return monday.Format(time.DateOnly), monday.AddDate(0, 0, 6).Format(time.DateOnly)
}

// CurrentMonday возвращает полночь понедельника текущей недели по времени
// колледжа. Для самого понедельника это сегодня, в отличие от NextMonday.
func CurrentMonday(now time.Time) time.Time {
	now = now.In(tz)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)

	return midnight.AddDate(0, 0, -(int(midnight.Weekday())+6)%7)
}

// Today возвращает сегодняшнюю дату по времени колледжа, ГГГГ-ММ-ДД.
func Today(now time.Time) string {
	return now.In(tz).Format(time.DateOnly)
}
