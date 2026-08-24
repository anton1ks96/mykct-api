// Package ldap реализует каталог пользователей колледжа поверх LDAP.
package ldap

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/anton1ks96/mykct-api/internal/auth/domain"
	"github.com/anton1ks96/mykct-api/internal/auth/repository"
	"github.com/anton1ks96/mykct-api/internal/platform/config"
	"github.com/anton1ks96/mykct-api/pkg/logger"
	"github.com/go-ldap/ldap/v3"
)

// Соответствие интерфейсу проверяется на этапе компиляции.
var _ repository.UserDirectory = (*Directory)(nil)

var log = logger.ComponentLogger("auth.directory")

// Типы групп в каталоге: атрибут description хранит тип, cn - значение.
const (
	groupTypeAcademic = "Группа"
	groupTypeProfile  = "Профиль"
	groupTypeSubgroup = "Подгруппа"
	groupTypeEnglish  = "Английский язык подгруппа"
)

// Служебные группы, определяющие роль пользователя.
const (
	groupAdmins   = "admin"
	groupTeachers = "teachers"
	groupStudents = "students"
)

// academicGroupPrefix - префикс cn академической группы: ИТ25-11.
const academicGroupPrefix = "ИТ"

// validProfiles - допустимые профили обучения.
var validProfiles = map[string]struct{}{
	"BE": {}, "FE": {}, "PM": {}, "CD": {}, "GD": {}, "SA": {},
}

// Directory - каталог пользователей колледжа поверх LDAP.
type Directory struct {
	cfg config.LDAPConfig
}

// NewDirectory создаёт каталог с настройками подключения.
func NewDirectory(cfg config.LDAPConfig) *Directory {
	return &Directory{cfg: cfg}
}

// Authenticate проверяет пару логин-пароль через bind под DN пользователя.
func (d *Directory) Authenticate(ctx context.Context, userID, password string) error {
	op := logger.NewLogOp(ctx, log, "Authenticate")

	if err := ctx.Err(); err != nil {
		return err
	}

	conn, err := d.dial()
	if err != nil {
		op.Failed(err).Msg("failed to connect to LDAP")
		return err
	}
	defer conn.Close()

	userDN, err := d.findUserDN(conn, userID)
	if err != nil {
		op.Debug().Str("user_id", userID).Msg("user not found in directory")
		return err
	}

	if err := conn.Bind(userDN, password); err != nil {
		op.Debug().Str("user_id", userID).Msg("bind failed, wrong password")
		return fmt.Errorf("%w: bind failed", domain.ErrInvalidCredentials)
	}

	op.Completed().Str("user_id", userID).Msg("user authenticated")

	return nil
}

// GetByID возвращает учётную запись пользователя вместе с определённой ролью.
func (d *Directory) GetByID(ctx context.Context, userID, password string) (*domain.User, error) {
	op := logger.NewLogOp(ctx, log, "GetByID")

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := d.dial()
	if err != nil {
		op.Failed(err).Msg("failed to connect to LDAP")
		return nil, err
	}
	defer conn.Close()

	userDN, err := d.findUserDN(conn, userID)
	if err != nil {
		return nil, err
	}

	if err := conn.Bind(userDN, password); err != nil {
		op.Debug().Str("user_id", userID).Msg("bind failed during lookup")
		return nil, fmt.Errorf("%w: bind failed", domain.ErrInvalidCredentials)
	}

	entry, err := d.searchOne(conn, d.baseDNFor(userID), userID, []string{"uid", "cn", "memberOf"})
	if err != nil {
		op.Failed(err).Str("user_id", userID).Msg("user lookup failed")
		return nil, err
	}

	role := d.determineRole(entry.GetAttributeValues("memberOf"), userDN)
	if role == "" {
		op.Debug().Str("user_id", userID).Str("dn", userDN).Msg("role not determined")
		return nil, domain.ErrRoleNotDetermined
	}

	op.Completed().Str("user_id", userID).Str("role", role).Msg("user fetched")

	return &domain.User{
		ID:       entry.GetAttributeValue("uid"),
		Username: entry.GetAttributeValue("cn"),
		Role:     role,
	}, nil
}

// GetUserGroups возвращает учебные группы пользователя: академическую группу,
// профиль, подгруппу и подгруппу английского языка.
func (d *Directory) GetUserGroups(ctx context.Context, userID, password string) (*domain.UserGroups, error) {
	op := logger.NewLogOp(ctx, log, "GetUserGroups")

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := d.dial()
	if err != nil {
		op.Failed(err).Msg("failed to connect to LDAP")
		return nil, err
	}
	defer conn.Close()

	userDN, err := d.findUserDN(conn, userID)
	if err != nil {
		return nil, err
	}

	if err := conn.Bind(userDN, password); err != nil {
		op.Debug().Str("user_id", userID).Msg("bind failed during group lookup")
		return nil, fmt.Errorf("%w: bind failed", domain.ErrInvalidCredentials)
	}

	filter := fmt.Sprintf(
		"(&(|(objectClass=groupOfNames)(objectClass=posixGroup)(objectClass=group))"+
			"(|(member=%s)(memberUid=%s)))",
		ldap.EscapeFilter(userDN),
		ldap.EscapeFilter(userID),
	)

	req := ldap.NewSearchRequest(
		d.cfg.GroupsBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, d.timeLimit(), false,
		filter,
		[]string{"cn", "description"},
		nil,
	)

	res, err := conn.Search(req)
	if err != nil {
		op.Failed(err).Str("user_id", userID).Msg("group search failed")
		return nil, fmt.Errorf("%w: group search failed: %v", domain.ErrDirectoryUnavailable, err)
	}

	groups := &domain.UserGroups{}
	for _, entry := range res.Entries {
		cn := entry.GetAttributeValue("cn")

		switch entry.GetAttributeValue("description") {
		case groupTypeAcademic:
			if strings.HasPrefix(cn, academicGroupPrefix) {
				groups.AcademicGroup = cn
			}
		case groupTypeProfile:
			if _, ok := validProfiles[cn]; ok {
				groups.Profile = cn
			}
		case groupTypeSubgroup:
			groups.Subgroup = cn
		case groupTypeEnglish:
			groups.EnglishGroup = cn
		}
	}

	op.Completed().Str("user_id", userID).Str("academic_group", groups.AcademicGroup).
		Msg("user groups fetched")

	return groups, nil
}

// dial открывает соединение с каталогом и выставляет таймауты.
func (d *Directory) dial() (*ldap.Conn, error) {
	conn, err := ldap.DialURL(d.cfg.URL, ldap.DialWithDialer(&net.Dialer{Timeout: d.cfg.Timeout}))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrDirectoryUnavailable, err)
	}
	conn.SetTimeout(d.cfg.Timeout)

	return conn, nil
}

// isTeacher различает преподавателей и студентов по префиксу логина.
func (d *Directory) isTeacher(userID string) bool {
	return strings.HasPrefix(userID, d.cfg.TeacherUIDPrefix)
}

// baseDNFor возвращает ветку каталога, в которой искать пользователя.
func (d *Directory) baseDNFor(userID string) string {
	if d.isTeacher(userID) {
		return d.cfg.TeachersBaseDN
	}
	return d.cfg.StudentsBaseDN
}

// timeLimit - предел времени поиска в секундах.
func (d *Directory) timeLimit() int {
	if seconds := int(d.cfg.Timeout.Seconds()); seconds > 0 {
		return seconds
	}
	return 1
}

// findUserDN находит DN пользователя анонимным поиском по uid.
func (d *Directory) findUserDN(conn *ldap.Conn, userID string) (string, error) {
	// Часть каталогов запрещает анонимный поиск - тогда пробуем без bind
	if err := conn.UnauthenticatedBind(""); err != nil {
		log.Debug().Err(err).Msg("anonymous bind failed, searching without bind")
	}

	entry, err := d.searchOne(conn, d.baseDNFor(userID), userID, []string{"dn"})
	if err != nil {
		return "", err
	}

	return entry.DN, nil
}

// searchOne ищет ровно одну запись по uid в указанной ветке каталога.
func (d *Directory) searchOne(conn *ldap.Conn, baseDN, userID string, attrs []string) (*ldap.Entry, error) {
	req := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		// Лимит 2, а не 1: иначе дубль uid не отличить от единственной записи
		2, d.timeLimit(), false,
		fmt.Sprintf("(uid=%s)", ldap.EscapeFilter(userID)),
		attrs,
		nil,
	)

	res, err := conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("%w: search failed: %v", domain.ErrDirectoryUnavailable, err)
	}

	switch len(res.Entries) {
	case 0:
		return nil, fmt.Errorf("%w: user not found", domain.ErrInvalidCredentials)
	case 1:
		return res.Entries[0], nil
	default:
		log.Error().Str("user_id", userID).Str("base_dn", baseDN).
			Msg("multiple LDAP entries for one uid")
		return nil, fmt.Errorf("%w: multiple entries for uid", domain.ErrDirectoryUnavailable)
	}
}

// determineRole выводит роль из членства в служебных группах, а для веток
// преподавателей - из расположения записи в дереве.
func (d *Directory) determineRole(memberOf []string, userDN string) string {
	userDNLower := strings.ToLower(userDN)
	inTeachersOU := strings.HasSuffix(userDNLower, strings.ToLower(d.cfg.TeachersBaseDN))
	inStudentsOU := !inTeachersOU && strings.HasSuffix(userDNLower, strings.ToLower(d.cfg.StudentsBaseDN))
	groupsSuffix := strings.ToLower(d.cfg.GroupsBaseDN)

	for _, group := range memberOf {
		if !strings.HasSuffix(strings.ToLower(group), groupsSuffix) {
			continue
		}

		cn, ok := strings.CutPrefix(strings.Split(group, ",")[0], "cn=")
		if !ok {
			continue
		}

		switch {
		case cn == groupAdmins:
			return domain.RoleAdmin
		case cn == groupTeachers:
			return domain.RoleTeacher
		case inStudentsOU && (cn == groupStudents || strings.HasPrefix(cn, academicGroupPrefix)):
			return domain.RoleStudent
		}
	}

	// Преподаватель может не состоять ни в одной группе - хватает ветки дерева
	if inTeachersOU {
		return domain.RoleTeacher
	}

	return ""
}
