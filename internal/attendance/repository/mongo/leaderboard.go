// Package mongo реализует хранилище реестра рейтинга в MongoDB.
package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
	"github.com/anton1ks96/mykct-api/internal/attendance/repository"
	"github.com/anton1ks96/mykct-api/pkg/database/mongodb"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Соответствие интерфейсам проверяется на этапе компиляции.
var (
	_ repository.Leaderboard = (*LeaderboardRepository)(nil)
	_ mongodb.IndexEnsurer   = (*LeaderboardRepository)(nil)
)

var log = logger.ComponentLogger("attendance.repository")

// leaderboardCollection - коллекция участников рейтинга. TTL у неё нет: реестр
// постоянный, протухшая сессия участника из рейтинга не выкидывает.
const leaderboardCollection = "attendance_leaderboard"

// cohortWarnSize - размер когорты, выше которого курс наверняка выведен неверно.
const cohortWarnSize = 1000

// participantDoc - участник рейтинга в MongoDB. Логин лежит открытым: без него
// воркер не спросит портал, который узнаёт студента по cookie с логином. Наружу
// логин не уходит ни в каком виде.
type participantDoc struct {
	Login             string    `bson:"login"`
	AcademicGroup     string    `bson:"academic_group"`
	Course            string    `bson:"course"`
	CurrentStreak     int       `bson:"current_streak"`
	LongestStreak     int       `bson:"longest_streak"`
	TotalDaysAttended int       `bson:"total_days_attended"`
	AttendanceRate    float64   `bson:"attendance_rate"`
	LastAttendedDate  string    `bson:"last_attended_date,omitempty"`
	EmptyRuns         int       `bson:"empty_runs"`
	Inactive          bool      `bson:"inactive"`
	RegisteredAt      time.Time `bson:"registered_at"`
	// StreakAt - когда серия была забрана с портала. По нему отсекается запись
	// более раннего забора поверх более позднего
	StreakAt time.Time `bson:"streak_at"`
	// UpdatedAt - когда участника в последний раз пытались пересчитать, удачно
	// или нет. По нему строится очередь воркера
	UpdatedAt time.Time `bson:"updated_at"`
}

// toDomain переводит документ в доменную модель.
func (d *participantDoc) toDomain() domain.Participant {
	return domain.Participant{
		Login:             d.Login,
		AcademicGroup:     d.AcademicGroup,
		Course:            d.Course,
		CurrentStreak:     d.CurrentStreak,
		LongestStreak:     d.LongestStreak,
		TotalDaysAttended: d.TotalDaysAttended,
		AttendanceRate:    d.AttendanceRate,
		LastAttendedDate:  d.LastAttendedDate,
		EmptyRuns:         d.EmptyRuns,
		Inactive:          d.Inactive,
		RegisteredAt:      d.RegisteredAt,
		UpdatedAt:         d.UpdatedAt,
	}
}

// LeaderboardRepository хранит реестр участников рейтинга в MongoDB.
type LeaderboardRepository struct {
	coll *mongo.Collection
}

// NewLeaderboardRepository создаёт репозиторий реестра поверх клиента MongoDB.
func NewLeaderboardRepository(client *mongo.Client, database string) *LeaderboardRepository {
	return &LeaderboardRepository{coll: client.Database(database).Collection(leaderboardCollection)}
}

// EnsureIndexes заводит индексы коллекции реестра.
func (r *LeaderboardRepository) EnsureIndexes(ctx context.Context) error {
	op := logger.NewLogOp(ctx, log, "EnsureIndexes")

	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "login", Value: 1}},
			Options: options.Index().SetName("leaderboard_login_uniq").SetUnique(true),
		},
		{
			// Поля равенства впереди, поле сортировки последним: выдача курса
			// идёт по индексу целиком, без сортировки в памяти
			Keys: bson.D{
				{Key: "course", Value: 1},
				{Key: "inactive", Value: 1},
				{Key: "current_streak", Value: -1},
			},
			Options: options.Index().SetName("leaderboard_course_rank_idx"),
		},
		{
			Keys:    bson.D{{Key: "inactive", Value: 1}, {Key: "updated_at", Value: 1}},
			Options: options.Index().SetName("leaderboard_refresh_idx"),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		op.Failed(err).Msg("failed to create indexes")
		return fmt.Errorf("failed to create leaderboard indexes: %w", err)
	}

	op.Completed().Msg("leaderboard indexes ensured")

	return nil
}

// Register заводит участника или освежает его группу. Отметку последнего
// пересчёта метод не двигает: иначе студент, регулярно открывающий приложение,
// навсегда выпал бы из очереди воркера.
func (r *LeaderboardRepository) Register(ctx context.Context, participant *domain.Participant) error {
	op := logger.NewLogOp(ctx, log, "Register")

	now := time.Now()
	update := bson.M{
		// Счётчик простоя здесь не сбрасывается: воркер зовёт Register каждый
		// прогон, и сброс не давал бы счётчику дорасти до предела
		"$set": bson.M{
			"academic_group": participant.AcademicGroup,
			"course":         participant.Course,
		},
		// Нулевое время пересчёта ставит нового участника первым в очередь воркера
		"$setOnInsert": bson.M{
			"login":               participant.Login,
			"current_streak":      0,
			"longest_streak":      0,
			"total_days_attended": 0,
			"attendance_rate":     0.0,
			"empty_runs":          0,
			"inactive":            false,
			"registered_at":       now,
			"streak_at":           time.Time{},
			"updated_at":          time.Time{},
		},
	}

	opts := options.UpdateOne().SetUpsert(true)
	if _, err := r.coll.UpdateOne(ctx, bson.M{"login": participant.Login}, update, opts); err != nil {
		op.Failed(err).Str("login", participant.Login).Msg("failed to register participant")
		return fmt.Errorf("failed to register leaderboard participant: %w", err)
	}

	op.Debug().Str("login", participant.Login).Msg("leaderboard participant registered")

	return nil
}

// SaveStreak записывает серию, забранную с портала в момент fetchedAt. Сравнение
// идёт по времени забора, а не записи: медленный ответ портала иначе затирал бы
// более свежие данные просто потому, что дошёл позже.
func (r *LeaderboardRepository) SaveStreak(
	ctx context.Context,
	login string,
	streak domain.Streak,
	fetchedAt time.Time,
) error {
	op := logger.NewLogOp(ctx, log, "SaveStreak")

	filter := bson.M{"login": login, "streak_at": bson.M{"$lt": fetchedAt}}
	update := bson.M{"$set": bson.M{
		"current_streak":      streak.CurrentStreak,
		"longest_streak":      streak.LongestStreak,
		"total_days_attended": streak.TotalDaysAttended,
		"attendance_rate":     streak.AttendanceRate,
		"last_attended_date":  streak.LastAttendedDate,
		"inactive":            false,
		"empty_runs":          0,
		"streak_at":           fetchedAt,
		"updated_at":          time.Now(),
	}}

	res, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		op.Failed(err).Str("login", login).Msg("failed to save participant streak")
		return fmt.Errorf("failed to save participant streak: %w", err)
	}
	// Ноль совпадений - это не ошибка, а гонка: кто-то записал более свежий
	// забор. Молча считать такую запись успехом нельзя, иначе прогон рапортует
	// о пересчёте, которого не было
	if res.MatchedCount == 0 {
		op.Debug().Str("login", login).Msg("participant streak skipped, a fresher fetch is stored")
		return nil
	}

	op.Debug().Str("login", login).Msg("participant streak saved")

	return nil
}

// Reactivate снимает отметку простоя. Зовётся только на собственный заход
// студента: фоновый пересчёт воскрешать погасший логин не должен.
func (r *LeaderboardRepository) Reactivate(ctx context.Context, login string) error {
	op := logger.NewLogOp(ctx, log, "Reactivate")

	update := bson.M{"$set": bson.M{"inactive": false, "empty_runs": 0}}
	if _, err := r.coll.UpdateOne(ctx, bson.M{"login": login}, update); err != nil {
		op.Failed(err).Str("login", login).Msg("failed to reactivate participant")
		return fmt.Errorf("failed to reactivate participant: %w", err)
	}

	op.Debug().Str("login", login).Msg("participant reactivated")

	return nil
}

// MarkAttempt отмечает неудачную попытку пересчёта. Серия остаётся прежней, но
// участник уходит в конец очереди: иначе логин, который портал не отдаёт
// никогда, вечно занимает её голову и не пускает остальных.
func (r *LeaderboardRepository) MarkAttempt(ctx context.Context, login string) error {
	op := logger.NewLogOp(ctx, log, "MarkAttempt")

	update := bson.M{"$set": bson.M{"updated_at": time.Now()}}
	if _, err := r.coll.UpdateOne(ctx, bson.M{"login": login}, update); err != nil {
		op.Failed(err).Str("login", login).Msg("failed to mark refresh attempt")
		return fmt.Errorf("failed to mark refresh attempt: %w", err)
	}

	return nil
}

// MarkEmpty засчитывает пустой ответ портала и гасит участника, когда их
// накопилось limit подряд: логин отчислен или сменился. Из реестра участник не
// удаляется, вернуть его в рейтинг может только собственный вход в приложение.
func (r *LeaderboardRepository) MarkEmpty(
	ctx context.Context,
	login string,
	fetchedAt time.Time,
	limit int,
) error {
	op := logger.NewLogOp(ctx, log, "MarkEmpty")

	// Пустой ответ, полученный до уже сохранённой серии, устарел и не должен
	// увеличивать счётчик или гасить участника.
	filter := bson.M{"login": login, "streak_at": bson.M{"$lt": fetchedAt}}

	// Отметка пересчёта двигается вместе со счётчиком: иначе погасший логин
	// крутился бы в выборке воркера каждый прогон
	update := bson.M{
		"$inc": bson.M{"empty_runs": 1},
		"$set": bson.M{"updated_at": time.Now()},
	}
	res, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		op.Failed(err).Str("login", login).Msg("failed to count empty portal answer")
		return fmt.Errorf("failed to count empty portal answer: %w", err)
	}
	if res.MatchedCount == 0 {
		op.Debug().Str("login", login).Msg("empty portal answer skipped, a fresher fetch is stored")
		return nil
	}

	res, err = r.coll.UpdateOne(ctx,
		bson.M{
			"login":      login,
			"streak_at":  bson.M{"$lt": fetchedAt},
			"empty_runs": bson.M{"$gte": limit},
		},
		bson.M{"$set": bson.M{"inactive": true}},
	)
	if err != nil {
		op.Failed(err).Str("login", login).Msg("failed to deactivate participant")
		return fmt.Errorf("failed to deactivate participant: %w", err)
	}

	if res.ModifiedCount > 0 {
		op.Completed().Str("login", login).Msg("participant deactivated after empty portal answers")
	}

	return nil
}

// ByCourse возвращает живых участников курса. Лимита нет: курс - это несколько
// сотен документов, а его отсутствие снимает риск обрезать когорту на месте
// подсчёта мест.
func (r *LeaderboardRepository) ByCourse(ctx context.Context, course string) ([]domain.Participant, error) {
	op := logger.NewLogOp(ctx, log, "ByCourse")

	// Участник без посчитанной серии - это ноль, неотличимый от других нулей:
	// такие строки добивают когорту до порога, не давая никакой анонимности
	filter := bson.M{
		"course":    course,
		"inactive":  false,
		"streak_at": bson.M{"$gt": time.Time{}},
	}
	opts := options.Find().SetSort(bson.D{{Key: "current_streak", Value: -1}})

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		op.Failed(err).Str("course", course).Msg("failed to list course participants")
		return nil, fmt.Errorf("failed to list course participants: %w", err)
	}

	var docs []participantDoc
	if err := cursor.All(ctx, &docs); err != nil {
		op.Failed(err).Str("course", course).Msg("failed to decode course participants")
		return nil, fmt.Errorf("failed to decode course participants: %w", err)
	}

	if len(docs) > cohortWarnSize {
		log.Warn().Str("course", course).Int("participants", len(docs)).
			Msg("leaderboard cohort is suspiciously large, check course extraction")
	}

	participants := make([]domain.Participant, 0, len(docs))
	for i := range docs {
		participants = append(participants, docs[i].toDomain())
	}

	op.Debug().Str("course", course).Int("participants", len(participants)).
		Msg("course participants listed")

	return participants, nil
}

// Stale возвращает участников, чью серию не пересчитывали дольше срока, начиная
// с самых давних. Так прогон воркера ограничен размером выборки и не зависит от
// размера реестра.
func (r *LeaderboardRepository) Stale(
	ctx context.Context,
	olderThan time.Time,
	limit int,
) ([]domain.Participant, error) {
	op := logger.NewLogOp(ctx, log, "Stale")

	filter := bson.M{"inactive": false, "updated_at": bson.M{"$lt": olderThan}}
	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		op.Failed(err).Msg("failed to list stale participants")
		return nil, fmt.Errorf("failed to list stale participants: %w", err)
	}

	var docs []participantDoc
	if err := cursor.All(ctx, &docs); err != nil {
		op.Failed(err).Msg("failed to decode stale participants")
		return nil, fmt.Errorf("failed to decode stale participants: %w", err)
	}

	participants := make([]domain.Participant, 0, len(docs))
	for i := range docs {
		participants = append(participants, docs[i].toDomain())
	}

	op.Debug().Int("participants", len(participants)).Msg("stale participants listed")

	return participants, nil
}
