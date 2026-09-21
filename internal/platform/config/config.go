// Package config загружает конфигурацию сервиса из переменных окружения (.env).
package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// DefaultTrustedProxies - подсеть Docker по умолчанию: только её
// X-Forwarded-For считается доверенным.
var DefaultTrustedProxies = []string{"172.16.0.0/12"}

// minWeekStateTTL - минимальный срок хранения состояния недели: неделя плюс запас.
const minWeekStateTTL = 8 * 24 * time.Hour

// minAliasSecretLen - минимальная длина секрета псевдонимов. Логины студентов
// предсказуемы, поэтому короткий секрет перебирается вместе с ними.
const minAliasSecretLen = 32

// minLeaderboardParticipants - нижняя граница порога участников: в выборке
// меньше пяти человек псевдонимы разбираются с первого взгляда.
const minLeaderboardParticipants = 5

type (
	// Config содержит полную конфигурацию сервиса.
	Config struct {
		Service     ServiceConfig
		Logger      LoggerConfig
		Server      ServerConfig
		Sentry      SentryConfig
		Mongo       MongoConfig
		CORS        CORSConfig
		RateLimit   RateLimitConfig
		Auth        AuthConfig
		LDAP        LDAPConfig
		Schedule    ScheduleConfig
		Attendance  AttendanceConfig
		Performance PerformanceConfig
		Push        PushConfig
	}

	// ServiceConfig содержит общие настройки сервиса.
	ServiceConfig struct {
		Name string
	}

	// LoggerConfig содержит настройки логгера.
	LoggerConfig struct {
		MinLevel string // debug, info, warn, error
		Pretty   bool   // Консольный вывод вместо JSON
	}

	// ServerConfig содержит настройки HTTP-сервера.
	ServerConfig struct {
		Port              string
		ReadTimeout       time.Duration
		ReadHeaderTimeout time.Duration
		WriteTimeout      time.Duration
		IdleTimeout       time.Duration
		ShutdownTimeout   time.Duration
		MaxHeaderBytes    int      // В мегабайтах
		TrustedProxies    []string // CIDR/IP прокси, чьему X-Forwarded-For можно верить
	}

	// SentryConfig содержит настройки Sentry (включается только при заданном DSN).
	SentryConfig struct {
		DSN              string
		Environment      string
		TracesSampleRate float64
		Debug            bool
	}

	// MongoConfig содержит настройки подключения к MongoDB.
	MongoConfig struct {
		URI                    string // Строка подключения целиком
		Database               string // Имя базы
		ConnectTimeout         time.Duration
		ServerSelectionTimeout time.Duration
		MaxPoolSize            uint64
		MinPoolSize            uint64
		MaxConnIdleTime        time.Duration
	}

	// CORSConfig содержит список разрешённых origin.
	CORSConfig struct {
		AllowedOrigins []string
	}

	// RateLimitConfig содержит настройки ограничения частоты запросов.
	RateLimitConfig struct {
		Enabled       bool
		IPRate        int // Запросов за окно с одного IP
		IPBurst       int // Размер всплеска
		WindowSeconds int // Длина окна в секундах
	}

	// AuthConfig содержит настройки аутентификации и выпуска токенов.
	AuthConfig struct {
		JWTSigningKey   string        // Ключ подписи access-токенов (HS256)
		AccessTokenTTL  time.Duration // Время жизни access-токена
		RefreshTokenTTL time.Duration // Время жизни refresh-сессии
		TestMode        bool          // Заглушка вместо похода в LDAP, только для разработки
	}

	// LDAPConfig содержит настройки каталога LDAP колледжа.
	LDAPConfig struct {
		URL              string        // Адрес сервера: ldap://host:389
		BaseDN           string        // Корень дерева: dc=it-college,dc=ru
		StudentsBaseDN   string        // Ветка студентов, собирается из OU и BaseDN
		TeachersBaseDN   string        // Ветка преподавателей
		GroupsBaseDN     string        // Ветка учебных групп, профилей и подгрупп
		TeacherUIDPrefix string        // Префикс uid преподавателя, остальные - студенты
		Timeout          time.Duration // Таймаут соединения и запросов к каталогу
	}

	// ScheduleConfig содержит настройки портала колледжа и кэша расписания.
	ScheduleConfig struct {
		PortalURL     string              // Базовый адрес портала: https://students.it-college.ru
		PortalTimeout time.Duration       // Таймаут запроса к порталу, без него не сработает откат на кэш
		CacheTTL      time.Duration       // Сколько снимок расписания хранится в MongoDB
		Watch         ScheduleWatchConfig // Воркер расписания: публикация недели и изменения внутри неё
	}

	// ScheduleWatchConfig содержит настройки воркера, который ловит появление
	// расписания на следующую неделю и его изменения внутри недели.
	ScheduleWatchConfig struct {
		Enabled        bool                  // Запускать ли воркер: он ходит на портал, видимый не отовсюду
		ActiveInterval time.Duration         // Интервал опроса в дни, когда расписание выкладывают
		IdleInterval   time.Duration         // Интервал опроса в остальные дни, он же задержка детекта изменений
		ActiveDays     map[time.Weekday]bool // Дни частого опроса
		GroupDelay     time.Duration         // Пауза между группами, чтобы не бить по порталу пачкой
		StateTTL       time.Duration         // Сколько живёт состояние недели в MongoDB
	}

	// AttendanceConfig содержит настройки портала колледжа для посещаемости.
	AttendanceConfig struct {
		PortalURL     string            // Базовый адрес портала: https://students.it-college.ru
		PortalTimeout time.Duration     // Таймаут запроса к порталу
		Leaderboard   LeaderboardConfig // Анонимный рейтинг по серии посещений
	}

	// LeaderboardConfig содержит настройки анонимного рейтинга по серии посещений.
	LeaderboardConfig struct {
		Enabled         bool          // Включён ли рейтинг: маршрут и воркер поднимаются только с ним
		AliasSecret     string        // Секрет HMAC для псевдонимов, наружу и в логи не попадает
		TopSize         int           // Сколько строк отдаётся в топе
		MinParticipants int           // Порог участников курса, ниже которого рейтинг не показывается
		RefreshInterval time.Duration // Пауза между прогонами воркера
		BatchSize       int           // Сколько участников пересчитывает один прогон
		RefreshTTL      time.Duration // Серия свежее этого срока не пересчитывается
		StudentDelay    time.Duration // Пауза между студентами, чтобы не бить по порталу пачкой
		EmptyRunsLimit  int           // Пустых ответов портала подряд до гашения участника
	}

	// PushConfig содержит настройки push-уведомлений через FCM.
	PushConfig struct {
		CredentialsPath string // JSON-ключ сервисного аккаунта Firebase; пусто - рассылка выключена
	}

	// PerformanceConfig содержит настройки портала колледжа для успеваемости.
	PerformanceConfig struct {
		PortalURL     string        // Базовый адрес портала: https://students.it-college.ru
		PortalTimeout time.Duration // Таймаут запроса к порталу
	}
)

// Init загружает конфигурацию из .env и переменных окружения.
func Init() (*Config, error) {
	var cfg Config

	_ = godotenv.Load()

	if err := setFromEnv(&cfg); err != nil {
		return nil, fmt.Errorf("failed to set environment variables: %w", err)
	}

	return &cfg, nil
}

// setFromEnv заполняет конфигурацию значениями из переменных окружения.
func setFromEnv(cfg *Config) error {
	var err error

	// Service
	cfg.Service.Name = getEnvOrDefault("SERVICE_NAME", "mykct-api")

	// Logger
	cfg.Logger.MinLevel = getEnvOrDefault("LOG_LEVEL", "info")
	cfg.Logger.Pretty = getEnvAsBool("LOG_PRETTY", false)

	// Server
	cfg.Server.Port = getEnvOrDefault("HTTP_PORT", "8080")
	cfg.Server.ReadTimeout, err = getEnvAsDuration("HTTP_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return fmt.Errorf("invalid HTTP_READ_TIMEOUT: %w", err)
	}
	cfg.Server.ReadHeaderTimeout, err = getEnvAsDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return fmt.Errorf("invalid HTTP_READ_HEADER_TIMEOUT: %w", err)
	}
	cfg.Server.WriteTimeout, err = getEnvAsDuration("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return fmt.Errorf("invalid HTTP_WRITE_TIMEOUT: %w", err)
	}
	cfg.Server.IdleTimeout, err = getEnvAsDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return fmt.Errorf("invalid HTTP_IDLE_TIMEOUT: %w", err)
	}
	cfg.Server.ShutdownTimeout, err = getEnvAsDuration("SHUTDOWN_TIMEOUT", 20*time.Second)
	if err != nil {
		return fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
	}
	cfg.Server.MaxHeaderBytes = getEnvAsInt("HTTP_MAX_HEADER_BYTES", 1)
	cfg.Server.TrustedProxies = slices.Clone(DefaultTrustedProxies)
	if raw, ok := os.LookupEnv("TRUSTED_PROXIES"); ok {
		cfg.Server.TrustedProxies = splitAndTrim(raw)
	}

	// Sentry
	cfg.Sentry.DSN = os.Getenv("SENTRY_DSN")
	cfg.Sentry.Environment = getEnvOrDefault("SENTRY_ENVIRONMENT", "development")
	cfg.Sentry.TracesSampleRate = getEnvAsFloat("SENTRY_TRACES_SAMPLE_RATE", 1.0)
	cfg.Sentry.Debug = getEnvAsBool("SENTRY_DEBUG", false)

	// MongoDB
	cfg.Mongo.URI = getEnvOrDefault("MONGO_URI", "mongodb://localhost:27017/?directConnection=true")
	cfg.Mongo.Database, err = getRequiredEnv("MONGO_DATABASE")
	if err != nil {
		return err
	}
	cfg.Mongo.ConnectTimeout, err = getEnvAsDuration("MONGO_CONNECT_TIMEOUT", 10*time.Second)
	if err != nil {
		return fmt.Errorf("invalid MONGO_CONNECT_TIMEOUT: %w", err)
	}
	cfg.Mongo.ServerSelectionTimeout, err = getEnvAsDuration("MONGO_SERVER_SELECTION_TIMEOUT", 5*time.Second)
	if err != nil {
		return fmt.Errorf("invalid MONGO_SERVER_SELECTION_TIMEOUT: %w", err)
	}
	cfg.Mongo.MaxPoolSize = uint64(getEnvAsInt("MONGO_MAX_POOL_SIZE", 100))
	cfg.Mongo.MinPoolSize = uint64(getEnvAsInt("MONGO_MIN_POOL_SIZE", 0))
	cfg.Mongo.MaxConnIdleTime, err = getEnvAsDuration("MONGO_MAX_CONN_IDLE_TIME", 5*time.Minute)
	if err != nil {
		return fmt.Errorf("invalid MONGO_MAX_CONN_IDLE_TIME: %w", err)
	}

	// CORS
	cfg.CORS.AllowedOrigins = getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"})

	// Rate limit
	cfg.RateLimit.Enabled = getEnvAsBool("RATE_LIMIT_ENABLED", true)
	cfg.RateLimit.IPRate = getEnvAsInt("RATE_LIMIT_IP_RATE", 100)
	cfg.RateLimit.IPBurst = getEnvAsInt("RATE_LIMIT_IP_BURST", 200)
	cfg.RateLimit.WindowSeconds = getEnvAsInt("RATE_LIMIT_WINDOW_SECONDS", 60)

	// Auth
	cfg.Auth.JWTSigningKey, err = getRequiredEnv("AUTH_JWT_SIGNING_KEY")
	if err != nil {
		return err
	}
	cfg.Auth.AccessTokenTTL, err = getEnvAsDuration("AUTH_ACCESS_TOKEN_TTL", time.Hour)
	if err != nil {
		return fmt.Errorf("invalid AUTH_ACCESS_TOKEN_TTL: %w", err)
	}
	cfg.Auth.RefreshTokenTTL, err = getEnvAsDuration("AUTH_REFRESH_TOKEN_TTL", 720*time.Hour)
	if err != nil {
		return fmt.Errorf("invalid AUTH_REFRESH_TOKEN_TTL: %w", err)
	}
	cfg.Auth.TestMode = getEnvAsBool("AUTH_TEST_MODE", false)

	// LDAP. В тестовом режиме каталог не нужен, поэтому URL обязателен только без него
	cfg.LDAP.URL = os.Getenv("LDAP_URL")
	if cfg.LDAP.URL == "" && !cfg.Auth.TestMode {
		return fmt.Errorf("environment variable LDAP_URL is required unless AUTH_TEST_MODE=true")
	}
	cfg.LDAP.BaseDN = getEnvOrDefault("LDAP_BASE_DN", "dc=it-college,dc=ru")
	cfg.LDAP.StudentsBaseDN = joinDN(getEnvOrDefault("LDAP_STUDENTS_OU", "ou=people"), cfg.LDAP.BaseDN)
	cfg.LDAP.TeachersBaseDN = joinDN(getEnvOrDefault("LDAP_TEACHERS_OU", "ou=people,ou=Teachers"), cfg.LDAP.BaseDN)
	cfg.LDAP.GroupsBaseDN = joinDN(getEnvOrDefault("LDAP_GROUPS_OU", "ou=Current"), cfg.LDAP.BaseDN)
	cfg.LDAP.TeacherUIDPrefix = getEnvOrDefault("LDAP_TEACHER_UID_PREFIX", "t")
	cfg.LDAP.Timeout, err = getEnvAsDuration("LDAP_TIMEOUT", 5*time.Second)
	if err != nil {
		return fmt.Errorf("invalid LDAP_TIMEOUT: %w", err)
	}

	// Расписание
	cfg.Schedule.PortalURL, err = getRequiredEnv("SCHEDULE_PORTAL_URL")
	if err != nil {
		return err
	}
	cfg.Schedule.PortalTimeout, err = getEnvAsDuration("SCHEDULE_PORTAL_TIMEOUT", 15*time.Second)
	if err != nil {
		return fmt.Errorf("invalid SCHEDULE_PORTAL_TIMEOUT: %w", err)
	}
	cfg.Schedule.CacheTTL, err = getEnvAsDuration("SCHEDULE_CACHE_TTL", 720*time.Hour)
	if err != nil {
		return fmt.Errorf("invalid SCHEDULE_CACHE_TTL: %w", err)
	}

	cfg.Schedule.Watch.Enabled = getEnvAsBool("SCHEDULE_WATCH_ENABLED", false)
	cfg.Schedule.Watch.ActiveInterval, err = getEnvAsDuration("SCHEDULE_WATCH_ACTIVE_INTERVAL", 15*time.Minute)
	if err != nil {
		return fmt.Errorf("invalid SCHEDULE_WATCH_ACTIVE_INTERVAL: %w", err)
	}
	cfg.Schedule.Watch.IdleInterval, err = getEnvAsDuration("SCHEDULE_WATCH_IDLE_INTERVAL", 30*time.Minute)
	if err != nil {
		return fmt.Errorf("invalid SCHEDULE_WATCH_IDLE_INTERVAL: %w", err)
	}
	// Заданная пустой переменная означает "учащённых дней нет", поэтому
	// getEnvOrDefault не подходит: он считает пустое значение отсутствующим
	activeDays, ok := os.LookupEnv("SCHEDULE_WATCH_ACTIVE_DAYS")
	if !ok {
		activeDays = "Fri,Sat,Sun"
	}
	cfg.Schedule.Watch.ActiveDays, err = parseWeekdays(activeDays)
	if err != nil {
		return fmt.Errorf("invalid SCHEDULE_WATCH_ACTIVE_DAYS: %w", err)
	}
	cfg.Schedule.Watch.GroupDelay, err = getEnvAsDuration("SCHEDULE_WATCH_GROUP_DELAY", 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("invalid SCHEDULE_WATCH_GROUP_DELAY: %w", err)
	}
	cfg.Schedule.Watch.StateTTL, err = getEnvAsDuration("SCHEDULE_WATCH_STATE_TTL", 720*time.Hour)
	if err != nil {
		return fmt.Errorf("invalid SCHEDULE_WATCH_STATE_TTL: %w", err)
	}
	// Состояние обязано пережить неделю, за которой следит: иначе оно исчезнет
	// раньше, чем неделя наступит, и статус пропадёт прямо посреди слежения
	if cfg.Schedule.Watch.StateTTL < minWeekStateTTL {
		return fmt.Errorf("SCHEDULE_WATCH_STATE_TTL must be at least %s", minWeekStateTTL)
	}

	// Посещаемость
	cfg.Attendance.PortalURL, err = getRequiredEnv("ATTENDANCE_PORTAL_URL")
	if err != nil {
		return err
	}
	cfg.Attendance.PortalTimeout, err = getEnvAsDuration("ATTENDANCE_PORTAL_TIMEOUT", 15*time.Second)
	if err != nil {
		return fmt.Errorf("invalid ATTENDANCE_PORTAL_TIMEOUT: %w", err)
	}

	// Рейтинг посещаемости
	cfg.Attendance.Leaderboard.Enabled = getEnvAsBool("ATTENDANCE_LEADERBOARD_ENABLED", false)
	cfg.Attendance.Leaderboard.TopSize = getEnvAsInt("ATTENDANCE_LEADERBOARD_TOP_SIZE", 10)
	cfg.Attendance.Leaderboard.MinParticipants = getEnvAsInt("ATTENDANCE_LEADERBOARD_MIN_PARTICIPANTS", 15)
	cfg.Attendance.Leaderboard.BatchSize = getEnvAsInt("ATTENDANCE_LEADERBOARD_BATCH_SIZE", 40)
	cfg.Attendance.Leaderboard.EmptyRunsLimit = getEnvAsInt("ATTENDANCE_LEADERBOARD_EMPTY_RUNS_LIMIT", 5)

	cfg.Attendance.Leaderboard.RefreshInterval, err = getEnvAsDuration("ATTENDANCE_LEADERBOARD_REFRESH_INTERVAL", time.Hour)
	if err != nil {
		return fmt.Errorf("invalid ATTENDANCE_LEADERBOARD_REFRESH_INTERVAL: %w", err)
	}
	cfg.Attendance.Leaderboard.RefreshTTL, err = getEnvAsDuration("ATTENDANCE_LEADERBOARD_REFRESH_TTL", 6*time.Hour)
	if err != nil {
		return fmt.Errorf("invalid ATTENDANCE_LEADERBOARD_REFRESH_TTL: %w", err)
	}
	cfg.Attendance.Leaderboard.StudentDelay, err = getEnvAsDuration("ATTENDANCE_LEADERBOARD_STUDENT_DELAY", 2*time.Second)
	if err != nil {
		return fmt.Errorf("invalid ATTENDANCE_LEADERBOARD_STUDENT_DELAY: %w", err)
	}

	// Секрет спрашивается только с включённым рейтингом: без рейтинга он не нужен,
	// а с ним псевдонимы без секрета сводятся к логинам перебором
	if cfg.Attendance.Leaderboard.Enabled {
		cfg.Attendance.Leaderboard.AliasSecret, err = getRequiredEnv("ATTENDANCE_LEADERBOARD_ALIAS_SECRET")
		if err != nil {
			return err
		}
		if len(cfg.Attendance.Leaderboard.AliasSecret) < minAliasSecretLen {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_ALIAS_SECRET must be at least %d characters", minAliasSecretLen)
		}
		if cfg.Attendance.Leaderboard.MinParticipants < minLeaderboardParticipants {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_MIN_PARTICIPANTS must be at least %d", minLeaderboardParticipants)
		}
		if cfg.Attendance.Leaderboard.TopSize < 1 {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_TOP_SIZE must be positive")
		}
		// Топ размером с порог публикует когорту целиком ровно тогда, когда она
		// наименьшая, то есть когда рейтинг разбирается легче всего
		if cfg.Attendance.Leaderboard.TopSize >= cfg.Attendance.Leaderboard.MinParticipants {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_TOP_SIZE must be less than ATTENDANCE_LEADERBOARD_MIN_PARTICIPANTS")
		}
		if cfg.Attendance.Leaderboard.RefreshInterval <= 0 {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_REFRESH_INTERVAL must be positive")
		}
		if cfg.Attendance.Leaderboard.RefreshTTL <= 0 {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_REFRESH_TTL must be positive")
		}
		// Нулевая пауза снимает единственный тормоз перед порталом
		if cfg.Attendance.Leaderboard.StudentDelay <= 0 {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_STUDENT_DELAY must be positive")
		}
		if cfg.Attendance.Leaderboard.BatchSize < 1 {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_BATCH_SIZE must be positive")
		}
		if cfg.Attendance.Leaderboard.EmptyRunsLimit < 1 {
			return fmt.Errorf("ATTENDANCE_LEADERBOARD_EMPTY_RUNS_LIMIT must be positive")
		}
	}

	// Успеваемость
	cfg.Performance.PortalURL, err = getRequiredEnv("PERFORMANCE_PORTAL_URL")
	if err != nil {
		return err
	}
	cfg.Performance.PortalTimeout, err = getEnvAsDuration("PERFORMANCE_PORTAL_TIMEOUT", 15*time.Second)
	if err != nil {
		return fmt.Errorf("invalid PERFORMANCE_PORTAL_TIMEOUT: %w", err)
	}

	// Push-уведомления (опционально: без ключа устройства копятся, но рассылки нет)
	cfg.Push.CredentialsPath = os.Getenv("FCM_CREDENTIALS_PATH")

	return nil
}

// getRequiredEnv возвращает значение обязательной переменной окружения.
func getRequiredEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("environment variable %s is required", key)
	}
	return val, nil
}

// getEnvOrDefault возвращает значение переменной окружения или значение по умолчанию.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsBool парсит переменную окружения как bool.
func getEnvAsBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// getEnvAsFloat парсит переменную окружения как float64.
func getEnvAsFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// getEnvAsInt парсит переменную окружения как int.
func getEnvAsInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// getEnvAsDuration парсит переменную окружения как time.Duration.
func getEnvAsDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

// getEnvAsSlice парсит переменную окружения как список строк через запятую.
func getEnvAsSlice(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return splitAndTrim(value)
}

// joinDN приклеивает ветку к корню дерева: "ou=people" + "dc=a,dc=b".
func joinDN(ou, baseDN string) string {
	ou = strings.Trim(strings.TrimSpace(ou), ",")
	if ou == "" {
		return baseDN
	}
	return ou + "," + baseDN
}

// weekdayNames - короткие имена дней недели, как они пишутся в .env.
var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// parseWeekdays разбирает "Fri,Sat,Sun" в набор дней недели.
func parseWeekdays(raw string) (map[time.Weekday]bool, error) {
	days := make(map[time.Weekday]bool, 7)

	for _, part := range splitAndTrim(raw) {
		day, ok := weekdayNames[strings.ToLower(part)]
		if !ok {
			return nil, fmt.Errorf("unknown weekday %q", part)
		}
		days[day] = true
	}

	return days, nil
}

// splitAndTrim разбирает строку "a, b, c" в список непустых значений.
func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
