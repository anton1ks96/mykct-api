// Package domain содержит доменные модели и ошибки модуля аутентификации.
package domain

// Роли пользователей, выводятся из групп и расположения записи в каталоге.
const (
	// RoleStudent - студент колледжа.
	RoleStudent = "student"
	// RoleTeacher - преподаватель.
	RoleTeacher = "teacher"
	// RoleAdmin - администратор.
	RoleAdmin = "admin"
)

// User - учётная запись из каталога колледжа.
type User struct {
	ID       string // Логин: номер студенческого i24s0291 или табельный t001
	Username string // ФИО из атрибута cn
	Role     string // RoleStudent, RoleTeacher или RoleAdmin
}

// UserGroups - учебные группы пользователя из каталога.
type UserGroups struct {
	AcademicGroup string // Академическая группа: ИТ25-11
	Profile       string // Профиль обучения: BE, FE, PM, CD, GD, SA
	Subgroup      string // Подгруппа: Подгр1, Подгр2
	EnglishGroup  string // Подгруппа английского языка: B1.21
}

// UserExtended - учётная запись вместе с учебными группами.
type UserExtended struct {
	ID            string
	Username      string
	Role          string
	AcademicGroup string
	Profile       string
	Subgroup      string
	EnglishGroup  string
}

// NewUserExtended собирает профиль из учётной записи и её учебных групп.
func NewUserExtended(user *User, groups *UserGroups) *UserExtended {
	return &UserExtended{
		ID:            user.ID,
		Username:      user.Username,
		Role:          user.Role,
		AcademicGroup: groups.AcademicGroup,
		Profile:       groups.Profile,
		Subgroup:      groups.Subgroup,
		EnglishGroup:  groups.EnglishGroup,
	}
}

// ActiveStudent - студент с живой refresh-сессией. Снимок для других модулей
// монолита: ни токенов, ни ФИО.
type ActiveStudent struct {
	UserID        string // Логин: i24s0291
	AcademicGroup string // Академическая группа: ИТ25-11
}
