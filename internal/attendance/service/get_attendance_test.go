package service

import (
	"testing"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// TestCollapseSubGroupsSingle - одиночная подгруппа описывает занятие целиком,
// а ClID заменяется на SClID, по которому портал отдаёт детали.
func TestCollapseSubGroupsSingle(t *testing.T) {
	records := []domain.Record{{
		ClID:  8515,
		Title: "Профильный предмет по подгруппам",
		SubGroup: []domain.SubGroup{
			{SClID: 10417, SCaID: "3-3", STopic: "Модули", STitle: "Разработка программных модулей"},
		},
	}}

	collapseSubGroups(records)

	r := records[0]
	if r.ClID != 10417 {
		t.Errorf("expected ClID 10417, got %d", r.ClID)
	}
	if r.Title != "Разработка программных модулей" {
		t.Errorf("expected subgroup title, got %q", r.Title)
	}
	if r.Topic != "Модули" {
		t.Errorf("expected subgroup topic, got %q", r.Topic)
	}
	if r.Room != "3-3" {
		t.Errorf("expected subgroup room, got %q", r.Room)
	}
	if r.SubGroup != nil {
		t.Errorf("expected SubGroup to be dropped, got %v", r.SubGroup)
	}
}

// TestCollapseSubGroupsKeepsFilledFields - тема и аудитория самого занятия не
// затираются, а без SClID остаётся родительский ClID.
func TestCollapseSubGroupsKeepsFilledFields(t *testing.T) {
	records := []domain.Record{{
		ClID:  8516,
		Topic: "Success",
		Room:  "404",
		SubGroup: []domain.SubGroup{
			{SCaID: "3-3", STopic: "Другая тема", STitle: "Иностранный язык"},
		},
	}}

	collapseSubGroups(records)

	r := records[0]
	if r.ClID != 8516 {
		t.Errorf("expected parent ClID 8516, got %d", r.ClID)
	}
	if r.Topic != "Success" || r.Room != "404" {
		t.Errorf("expected topic and room to stay, got %q and %q", r.Topic, r.Room)
	}
}

// TestCollapseSubGroupsSkipsOthers - занятие без подгрупп или с несколькими не меняется.
func TestCollapseSubGroupsSkipsOthers(t *testing.T) {
	records := []domain.Record{
		{ClID: 1, Title: "Обществознание"},
		{ClID: 2, Title: "Английский", SubGroup: []domain.SubGroup{
			{SClID: 21, STitle: "A0.11"},
			{SClID: 22, STitle: "B1.21"},
		}},
	}

	collapseSubGroups(records)

	if records[0].ClID != 1 || records[0].Title != "Обществознание" {
		t.Errorf("expected plain record to stay, got %+v", records[0])
	}
	if records[1].ClID != 2 || len(records[1].SubGroup) != 2 {
		t.Errorf("expected record with two subgroups to stay, got %+v", records[1])
	}
}
