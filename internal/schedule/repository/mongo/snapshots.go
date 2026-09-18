// Package mongo хранит снимки ответов портала, из которых расписание отдаётся,
// пока портал колледжа недоступен.
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/schedule/domain"
	"github.com/anton1ks96/mykct-api/internal/schedule/repository"
	"github.com/anton1ks96/mykct-api/pkg/database/mongodb"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.SnapshotRepository = (*SnapshotRepository)(nil)
	_ mongodb.IndexEnsurer          = (*SnapshotRepository)(nil)
)

var log = logger.ComponentLogger("schedule.repository")

// Коллекции снимков.
const (
	scheduleCollection     = "schedule_snapshots"
	classDetailsCollection = "class_details_snapshots"
)

// subGroupDoc - подгруппа занятия в документе. Доменная модель про bson не знает.
type subGroupDoc struct {
	SClID  string `bson:"s_cl_id,omitempty"`
	SGrID  string `bson:"s_gr_id,omitempty"`
	SGCaID string `bson:"s_gca_id,omitempty"`
	STopic string `bson:"s_topic,omitempty"`
	STitle string `bson:"s_title,omitempty"`
}

// eventDoc - занятие в документе снимка.
type eventDoc struct {
	ClID     string        `bson:"cl_id,omitempty"`
	Type     string        `bson:"type,omitempty"`
	Day      string        `bson:"day,omitempty"`
	Group    string        `bson:"group,omitempty"`
	Topic    string        `bson:"topic,omitempty"`
	Start    string        `bson:"start,omitempty"`
	End      string        `bson:"end,omitempty"`
	Room     string        `bson:"room,omitempty"`
	Color    string        `bson:"color,omitempty"`
	Title    string        `bson:"title,omitempty"`
	SubGroup []subGroupDoc `bson:"subgroup,omitempty"`
}

// snapshotDoc - снимок расписания группы за период.
type snapshotDoc struct {
	Group     string     `bson:"group"`
	Start     string     `bson:"start"`
	End       string     `bson:"end"`
	Events    []eventDoc `bson:"events"`
	FetchedAt time.Time  `bson:"fetched_at"`
	ExpiresAt time.Time  `bson:"expires_at"`
}

// classDetailsDoc - снимок деталей занятия. Портал отдаёт произвольный JSON,
// поэтому тело хранится как есть.
type classDetailsDoc struct {
	ClID      string         `bson:"cl_id"`
	Details   map[string]any `bson:"details"`
	FetchedAt time.Time      `bson:"fetched_at"`
	ExpiresAt time.Time      `bson:"expires_at"`
}

// toDomain переводит занятие документа в доменную модель.
func (d *eventDoc) toDomain() domain.Event {
	event := domain.Event{
		ClID:  d.ClID,
		Type:  d.Type,
		Day:   d.Day,
		Group: d.Group,
		Topic: d.Topic,
		Start: d.Start,
		End:   d.End,
		Room:  d.Room,
		Color: d.Color,
		Title: d.Title,
	}

	if len(d.SubGroup) > 0 {
		event.SubGroup = make([]domain.SubGroup, 0, len(d.SubGroup))
		for _, sg := range d.SubGroup {
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

// eventFromDomain переводит занятие доменной модели в документ.
func eventFromDomain(e domain.Event) eventDoc {
	doc := eventDoc{
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
		doc.SubGroup = make([]subGroupDoc, 0, len(e.SubGroup))
		for _, sg := range e.SubGroup {
			doc.SubGroup = append(doc.SubGroup, subGroupDoc{
				SClID:  sg.SClID,
				SGrID:  sg.SGrID,
				SGCaID: sg.SGCaID,
				STopic: sg.STopic,
				STitle: sg.STitle,
			})
		}
	}

	return doc
}

// eventsToDomain переводит занятия документа в доменную модель. Нулевой слайс
// сохраняется: портал так отвечает на пустой период.
func eventsToDomain(docs []eventDoc) []domain.Event {
	if docs == nil {
		return nil
	}

	events := make([]domain.Event, 0, len(docs))
	for _, doc := range docs {
		events = append(events, doc.toDomain())
	}

	return events
}

// eventsFromDomain переводит занятия доменной модели в документы.
func eventsFromDomain(events []domain.Event) []eventDoc {
	if events == nil {
		return nil
	}

	docs := make([]eventDoc, 0, len(events))
	for _, e := range events {
		docs = append(docs, eventFromDomain(e))
	}

	return docs
}

// toDomain переводит документ снимка в доменную модель.
func (d *snapshotDoc) toDomain() *domain.Snapshot {
	return &domain.Snapshot{
		Group:     d.Group,
		Start:     d.Start,
		End:       d.End,
		Events:    eventsToDomain(d.Events),
		FetchedAt: d.FetchedAt,
	}
}

// SnapshotRepository хранит снимки ответов портала в MongoDB.
type SnapshotRepository struct {
	schedule     *mongo.Collection
	classDetails *mongo.Collection
	ttl          time.Duration
}

// NewSnapshotRepository создаёт хранилище снимков поверх клиента MongoDB.
func NewSnapshotRepository(client *mongo.Client, database string, ttl time.Duration) *SnapshotRepository {
	db := client.Database(database)

	return &SnapshotRepository{
		schedule:     db.Collection(scheduleCollection),
		classDetails: db.Collection(classDetailsCollection),
		ttl:          ttl,
	}
}

// EnsureIndexes заводит индексы коллекций снимков.
func (r *SnapshotRepository) EnsureIndexes(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "EnsureIndexes")

	scheduleModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "group", Value: 1},
				{Key: "start", Value: 1},
				{Key: "end", Value: 1},
			},
			Options: options.Index().SetName("schedule_key_uniq").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("schedule_expires_at_ttl").SetExpireAfterSeconds(0),
		},
	}

	if _, err := r.schedule.Indexes().CreateMany(ctx, scheduleModels); err != nil {
		op.Failed(err).Msg("failed to create schedule snapshot indexes")
		return fmt.Errorf("failed to create schedule snapshot indexes: %w", err)
	}

	classDetailsModels := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "cl_id", Value: 1}},
			Options: options.Index().SetName("cl_id_uniq").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("class_details_expires_at_ttl").SetExpireAfterSeconds(0),
		},
	}

	if _, err := r.classDetails.Indexes().CreateMany(ctx, classDetailsModels); err != nil {
		op.Failed(err).Msg("failed to create class details snapshot indexes")
		return fmt.Errorf("failed to create class details snapshot indexes: %w", err)
	}

	op.Completed().Msg("schedule snapshot indexes ensured")

	return nil
}

// SaveSchedule сохраняет снимок расписания, затирая предыдущий за тот же период.
func (r *SnapshotRepository) SaveSchedule(ctx context.Context, snapshot *domain.Snapshot) error {
	op := logger.NewLogOp(ctx, log, "SaveSchedule")

	update := bson.M{"$set": bson.M{
		"events":     eventsFromDomain(snapshot.Events),
		"fetched_at": snapshot.FetchedAt,
		"expires_at": snapshot.FetchedAt.Add(r.ttl),
	}}

	_, err := r.schedule.UpdateOne(ctx, scheduleFilter(snapshot.Group, snapshot.Start, snapshot.End),
		update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		op.Failed(err).Str("group", snapshot.Group).Msg("failed to save schedule snapshot")
		return fmt.Errorf("failed to save schedule snapshot: %w", err)
	}

	op.Debug().Str("group", snapshot.Group).Int("events", len(snapshot.Events)).
		Msg("schedule snapshot saved")

	return nil
}

// FindSchedule возвращает последний снимок расписания за период.
func (r *SnapshotRepository) FindSchedule(ctx context.Context, group, start, end string) (*domain.Snapshot, error) {
	op := logger.NewLogOp(ctx, log, "FindSchedule")

	var doc snapshotDoc
	err := r.schedule.FindOne(ctx, scheduleFilter(group, start, end)).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			op.Debug().Str("group", group).Msg("schedule snapshot not found")
			return nil, domain.ErrScheduleUnavailable
		}
		op.Failed(err).Str("group", group).Msg("failed to find schedule snapshot")
		return nil, fmt.Errorf("failed to find schedule snapshot: %w", err)
	}

	op.Debug().Str("group", group).Time("fetched_at", doc.FetchedAt).
		Msg("schedule snapshot found")

	return doc.toDomain(), nil
}

// SaveClassDetails сохраняет детали занятия, затирая предыдущие.
func (r *SnapshotRepository) SaveClassDetails(ctx context.Context, details *domain.ClassDetails) error {
	op := logger.NewLogOp(ctx, log, "SaveClassDetails")

	update := bson.M{"$set": bson.M{
		"details":    details.Details,
		"fetched_at": details.FetchedAt,
		"expires_at": details.FetchedAt.Add(r.ttl),
	}}

	_, err := r.classDetails.UpdateOne(ctx, bson.M{"cl_id": details.ClID},
		update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		op.Failed(err).Str("clid", details.ClID).Msg("failed to save class details snapshot")
		return fmt.Errorf("failed to save class details snapshot: %w", err)
	}

	op.Debug().Str("clid", details.ClID).Msg("class details snapshot saved")

	return nil
}

// FindClassDetails возвращает последние сохранённые детали занятия.
func (r *SnapshotRepository) FindClassDetails(ctx context.Context, clid string) (*domain.ClassDetails, error) {
	op := logger.NewLogOp(ctx, log, "FindClassDetails")

	var doc classDetailsDoc
	err := r.classDetails.FindOne(ctx, bson.M{"cl_id": clid}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			op.Debug().Str("clid", clid).Msg("class details snapshot not found")
			return nil, domain.ErrClassDetailsUnavailable
		}
		op.Failed(err).Str("clid", clid).Msg("failed to find class details snapshot")
		return nil, fmt.Errorf("failed to find class details snapshot: %w", err)
	}

	op.Debug().Str("clid", clid).Time("fetched_at", doc.FetchedAt).
		Msg("class details snapshot found")

	return &domain.ClassDetails{
		ClID:      doc.ClID,
		Details:   doc.Details,
		FetchedAt: doc.FetchedAt,
	}, nil
}

// scheduleFilter адресует снимок расписания группой и периодом.
func scheduleFilter(group, start, end string) bson.M {
	return bson.M{"group": group, "start": start, "end": end}
}
