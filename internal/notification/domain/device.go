// Package domain содержит доменные модели модуля уведомлений.
package domain

import "time"

// Платформы устройств.
const (
	// PlatformAndroid - Android.
	PlatformAndroid = "android"
	// PlatformIOS - iOS.
	PlatformIOS = "ios"
)

// Device - устройство, на которое уходят push-уведомления. Одно устройство
// принадлежит одному пользователю: вошёл другой - запись переходит к нему.
type Device struct {
	DeviceID      string    // Идентификатор установки приложения
	Token         string    // FCM registration token
	Platform      string    // PlatformAndroid или PlatformIOS
	UserID        string    // Логин владельца
	AcademicGroup string    // Группа владельца на момент регистрации, по ней идёт рассылка
	UpdatedAt     time.Time // Последняя регистрация
}
