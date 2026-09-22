package postgres

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
)

// nulEscape - нулевой байт в закодированном JSON. JSONB такое значение не
// принимает вовсе, а BSON принимал: портал в теории отдаёт что угодно.
const nulEscape = `\u0000`

// marshalJSONB кодирует значение для колонки JSONB, выбрасывая нулевые байты:
// иначе PostgreSQL отвечает unsupported Unicode escape sequence.
func marshalJSONB(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	return dropNULEscapes(raw), nil
}

// dropNULEscapes убирает из закодированного JSON экранированные нулевые байты,
// не задевая текст, который сам похож на такое экранирование.
func dropNULEscapes(raw []byte) []byte {
	escape := []byte(nulEscape)
	if !bytes.Contains(raw, escape) {
		return raw
	}

	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		if raw[i] == '\\' {
			if i+1 < len(raw) && raw[i+1] == '\\' {
				out = append(out, raw[i], raw[i+1])
				i += 2
				continue
			}
			if bytes.HasPrefix(raw[i:], escape) {
				i += len(escape)
				continue
			}
		}
		out = append(out, raw[i])
		i++
	}

	return out
}

// subGroupJSON - подгруппа занятия в JSONB. Доменная модель про сериализацию
// не знает, поэтому снимки едут через эти структуры.
type subGroupJSON struct {
	SClID  string `json:"s_cl_id,omitempty"`
	SGrID  string `json:"s_gr_id,omitempty"`
	SGCaID string `json:"s_gca_id,omitempty"`
	STopic string `json:"s_topic,omitempty"`
	STitle string `json:"s_title,omitempty"`
}

// eventJSON - занятие в JSONB.
type eventJSON struct {
	ClID     string         `json:"cl_id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Day      string         `json:"day,omitempty"`
	Group    string         `json:"group,omitempty"`
	Topic    string         `json:"topic,omitempty"`
	Start    string         `json:"start,omitempty"`
	End      string         `json:"end,omitempty"`
	Room     string         `json:"room,omitempty"`
	Color    string         `json:"color,omitempty"`
	Title    string         `json:"title,omitempty"`
	SubGroup []subGroupJSON `json:"subgroup,omitempty"`
}

// toDomain переводит занятие снимка в доменную модель.
func (e *eventJSON) toDomain() domain.Event {
	event := domain.Event{
		ClID:  e.ClID,
		Type:  e.Type,
		Day:   e.Day,
		Group: e.Group,
		Topic: e.Topic,
		Start: e.Start,
		End:   e.End,
		Room:  e.Room,
		Color: e.Color,
		Title: e.Title,
	}

	if len(e.SubGroup) > 0 {
		event.SubGroup = make([]domain.SubGroup, 0, len(e.SubGroup))
		for _, sg := range e.SubGroup {
			event.SubGroup = append(event.SubGroup, domain.SubGroup{
				SClID:  sg.SClID,
				SGrID:  sg.SGrID,
				SGCaID: sg.SGCaID,
				STopic: sg.STopic,
				STitle: sg.STitle,
			})
		}
	}

	return event
}

// eventFromDomain переводит занятие доменной модели в снимок.
func eventFromDomain(e domain.Event) eventJSON {
	item := eventJSON{
		ClID:  e.ClID,
		Type:  e.Type,
		Day:   e.Day,
		Group: e.Group,
		Topic: e.Topic,
		Start: e.Start,
		End:   e.End,
		Room:  e.Room,
		Color: e.Color,
		Title: e.Title,
	}

	if len(e.SubGroup) > 0 {
		item.SubGroup = make([]subGroupJSON, 0, len(e.SubGroup))
		for _, sg := range e.SubGroup {
			item.SubGroup = append(item.SubGroup, subGroupJSON{
				SClID:  sg.SClID,
				SGrID:  sg.SGrID,
				SGCaID: sg.SGCaID,
				STopic: sg.STopic,
				STitle: sg.STitle,
			})
		}
	}

	return item
}

// marshalEvents собирает занятия в JSONB. Нулевой слайс превращается в JSON
// null и читается обратно нулевым: портал так отвечает на пустой период.
func marshalEvents(events []domain.Event) ([]byte, error) {
	if events == nil {
		return []byte("null"), nil
	}

	items := make([]eventJSON, 0, len(events))
	for _, e := range events {
		items = append(items, eventFromDomain(e))
	}

	raw, err := marshalJSONB(items)
	if err != nil {
		return nil, fmt.Errorf("failed to encode events: %w", err)
	}

	return raw, nil
}

// unmarshalEvents разбирает занятия из JSONB. Колонка, не попавшая в выборку,
// приезжает пустой - это не ошибка, а снимок, который не запрашивали.
func unmarshalEvents(raw []byte) ([]domain.Event, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var items []eventJSON
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("failed to decode events: %w", err)
	}
	if items == nil {
		return nil, nil
	}

	events := make([]domain.Event, 0, len(items))
	for i := range items {
		events = append(events, items[i].toDomain())
	}

	return events, nil
}
