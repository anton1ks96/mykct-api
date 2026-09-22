CREATE TABLE refresh_sessions (
    token_hash     TEXT        NOT NULL,
    user_id        TEXT        NOT NULL,
    username       TEXT        NOT NULL,
    role           TEXT        NOT NULL,
    academic_group TEXT        NOT NULL DEFAULT '',
    profile        TEXT        NOT NULL DEFAULT '',
    subgroup       TEXT        NOT NULL DEFAULT '',
    english_group  TEXT        NOT NULL DEFAULT '',
    expires_at     TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (token_hash)
);

COMMENT ON TABLE refresh_sessions IS 'Выданные refresh-токены со снимком профиля; сам токен не хранится';
COMMENT ON COLUMN refresh_sessions.token_hash IS 'SHA-256 от refresh-токена в hex';

CREATE INDEX idx_refresh_sessions_user ON refresh_sessions (user_id);
CREATE INDEX idx_refresh_sessions_expires ON refresh_sessions (expires_at);

CREATE TABLE notification_devices (
    device_id      TEXT        NOT NULL,
    token          TEXT        NOT NULL,
    platform       TEXT        NOT NULL,
    user_id        TEXT        NOT NULL,
    academic_group TEXT        NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (device_id),
    CONSTRAINT uq_notification_devices_token UNIQUE (token)
);

COMMENT ON TABLE notification_devices IS 'Устройства с FCM-токенами; ключ записи - установка приложения, а не пара пользователь+устройство';

CREATE INDEX idx_notification_devices_group ON notification_devices (academic_group);

CREATE TABLE schedule_snapshots (
    group_name   TEXT        NOT NULL,
    period_start TEXT        NOT NULL,
    period_end   TEXT        NOT NULL,
    events       JSONB       NOT NULL,
    fetched_at   TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (group_name, period_start, period_end)
);

COMMENT ON TABLE schedule_snapshots IS 'Кэш ответов портала: из него расписание отдаётся, пока портал недоступен';
COMMENT ON COLUMN schedule_snapshots.events IS 'Занятия периода без фильтрации по подгруппам, как их отдал портал';

CREATE INDEX idx_schedule_snapshots_expires ON schedule_snapshots (expires_at);

CREATE TABLE class_details_snapshots (
    cl_id      TEXT        NOT NULL,
    details    JSONB       NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (cl_id)
);

COMMENT ON TABLE class_details_snapshots IS 'Кэш деталей занятия; портал отдаёт произвольный JSON, поэтому тело лежит как есть';

CREATE INDEX idx_class_details_snapshots_expires ON class_details_snapshots (expires_at);

CREATE TABLE schedule_week_states (
    group_name      TEXT        NOT NULL,
    week_start      TEXT        NOT NULL,
    week_end        TEXT        NOT NULL,
    published       BOOLEAN     NOT NULL DEFAULT FALSE,
    events_count    INTEGER     NOT NULL DEFAULT 0,
    published_at    TIMESTAMPTZ NULL,
    notified_at     TIMESTAMPTZ NULL,
    last_checked_at TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    events          JSONB       NOT NULL,
    events_hash     TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (group_name, week_start)
);

COMMENT ON TABLE schedule_week_states IS 'Состояния недель: по ним ловится появление расписания и берётся неразосланное';
COMMENT ON COLUMN schedule_week_states.published_at IS 'Когда зафиксирован переход пусто -> непусто; NULL - неделю заполнили до слежения';
COMMENT ON COLUMN schedule_week_states.notified_at IS 'Когда разослано уведомление; NULL - ждёт рассылки';
COMMENT ON COLUMN schedule_week_states.events IS 'Базовый снимок недели, с которым сверяется свежий ответ портала';
COMMENT ON COLUMN schedule_week_states.events_hash IS 'Отпечаток базового снимка; им же базис меняется под гонку';

CREATE INDEX idx_schedule_week_states_notify ON schedule_week_states (published_at)
    WHERE notified_at IS NULL;
CREATE INDEX idx_schedule_week_states_expires ON schedule_week_states (expires_at);

CREATE TABLE schedule_changes (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY,
    group_name  TEXT        NOT NULL,
    week_start  TEXT        NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL,
    changes     JSONB       NOT NULL,
    notified_at TIMESTAMPTZ NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id)
);

COMMENT ON TABLE schedule_changes IS 'Разница расписания за один прогон воркера: одна строка - одно будущее уведомление';
COMMENT ON COLUMN schedule_changes.notified_at IS 'Когда разослано уведомление; NULL - ждёт рассылки';

CREATE INDEX idx_schedule_changes_notify ON schedule_changes (detected_at)
    WHERE notified_at IS NULL;
CREATE INDEX idx_schedule_changes_expires ON schedule_changes (expires_at);

CREATE TABLE schedule_tracked_groups (
    group_name    TEXT        NOT NULL,
    tracked_since TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (group_name)
);

COMMENT ON TABLE schedule_tracked_groups IS 'С какого момента за группой следят; без срока жизни - отметка переживает любые состояния недель';

CREATE TABLE attendance_leaderboard (
    login               TEXT             NOT NULL,
    academic_group      TEXT             NOT NULL,
    course              TEXT             NOT NULL,
    current_streak      INTEGER          NOT NULL DEFAULT 0,
    longest_streak      INTEGER          NOT NULL DEFAULT 0,
    total_days_attended INTEGER          NOT NULL DEFAULT 0,
    attendance_rate     DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_attended_date  TEXT             NOT NULL DEFAULT '',
    empty_runs          INTEGER          NOT NULL DEFAULT 0,
    inactive            BOOLEAN          NOT NULL DEFAULT FALSE,
    registered_at       TIMESTAMPTZ      NOT NULL DEFAULT now(),
    streak_at           TIMESTAMPTZ      NULL,
    updated_at          TIMESTAMPTZ      NULL,
    PRIMARY KEY (login)
);

COMMENT ON TABLE attendance_leaderboard IS 'Постоянный реестр рейтинга: протухшая сессия участника из него не выкидывает';
COMMENT ON COLUMN attendance_leaderboard.login IS 'Логин лежит открытым: без него воркер не спросит портал. Наружу не уходит';
COMMENT ON COLUMN attendance_leaderboard.inactive IS 'Логин погас: из выдачи убран, из реестра нет';
COMMENT ON COLUMN attendance_leaderboard.streak_at IS 'Когда серия забрана с портала; NULL - ни разу, в выдачу такая строка не идёт';
COMMENT ON COLUMN attendance_leaderboard.updated_at IS 'Когда участника пытались пересчитать; NULL - ни разу, он первый в очереди';

CREATE INDEX idx_attendance_leaderboard_rank ON attendance_leaderboard (course, current_streak DESC)
    WHERE inactive = FALSE AND streak_at IS NOT NULL;
CREATE INDEX idx_attendance_leaderboard_refresh ON attendance_leaderboard (updated_at NULLS FIRST)
    WHERE inactive = FALSE;
