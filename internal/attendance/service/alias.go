package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// aliasScope - метка области рейтинга в HMAC. Другая область рейтинга обязана
// получить другую метку: общий псевдоним в двух таблицах выдаёт пересечением
// областей то, что каждая из них по отдельности скрывает.
const aliasScope = "mykct.leaderboard.course.v1"

// aliasSuffixRange - потолок числового суффикса псевдонима, 0x0000..0xFFFF.
// Пространство 96 x 96 x 65536 делает совпадение на курсе в 300 человек событием
// с вероятностью 0.007%, поэтому разводить совпадения постобработкой не нужно:
// такая постобработка привязывала бы псевдоним к составу когорты.
const aliasSuffixRange = 65536

// aliasMaker считает псевдонимы одной области рейтинга. Псевдоним нигде не
// хранится: он детерминированно выводится из логина и секрета при каждой выдаче.
type aliasMaker struct {
	secret []byte
	course string
}

// newAliasMaker собирает генератор псевдонимов курса.
func newAliasMaker(secret []byte, course string) aliasMaker {
	return aliasMaker{secret: secret, course: course}
}

// digest считает HMAC-SHA256 от области, курса и логина. Части разделены нулевым
// байтом: без него курс "ИТ2" с логином "5i24s0291" дал бы тот же дайджест, что
// курс "ИТ25" с логином "i24s0291".
func (a aliasMaker) digest(login string) []byte {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(aliasScope))
	mac.Write([]byte{0})
	mac.Write([]byte(a.course))
	mac.Write([]byte{0})
	mac.Write([]byte(login))

	return mac.Sum(nil)
}

// alias собирает псевдоним из дайджеста: прилагательное, существительное и
// шестнадцатеричный суффикс - "Рекурсивный Компилятор 0x1A7C".
func (a aliasMaker) alias(login string) string {
	d := a.digest(login)

	adjective := aliasAdjectives[binary.BigEndian.Uint32(d[0:4])%uint32(len(aliasAdjectives))]
	noun := aliasNouns[binary.BigEndian.Uint32(d[4:8])%uint32(len(aliasNouns))]
	suffix := binary.BigEndian.Uint32(d[8:12]) % aliasSuffixRange

	return fmt.Sprintf("%s %s 0x%04X", adjective, noun, suffix)
}

// seed - порядок участника внутри равных серий. Считается тем же дайджестом,
// что и псевдоним, поэтому сортировка не выдаёт о студенте ничего сверх уже
// показанного псевдонима. Порядок по логину выдавал бы номер студенческого.
func (a aliasMaker) seed(login string) uint64 {
	return binary.BigEndian.Uint64(a.digest(login)[12:20])
}
