package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/anton1ks96/mykct-api/internal/attendance/domain"
)

// aliasScope - метка области рейтинга в HMAC. Другая область рейтинга обязана
// получить другую метку: общий псевдоним в двух таблицах выдаёт пересечением
// областей то, что каждая из них по отдельности скрывает.
const aliasScope = "mykct.leaderboard.course.v1"

// aliasSuffixRange - потолок числового суффикса псевдонима, 0x000..0xFFF.
const aliasSuffixRange = 4096

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
// шестнадцатеричный суффикс - "Рекурсивный Компилятор 0x1A7".
func (a aliasMaker) alias(login string) string {
	d := a.digest(login)

	adjective := aliasAdjectives[binary.BigEndian.Uint32(d[0:4])%uint32(len(aliasAdjectives))]
	noun := aliasNouns[binary.BigEndian.Uint32(d[4:8])%uint32(len(aliasNouns))]
	suffix := binary.BigEndian.Uint32(d[8:12]) % aliasSuffixRange

	return fmt.Sprintf("%s %s 0x%03X", adjective, noun, suffix)
}

// seed - порядок участника внутри равных серий. Считается тем же дайджестом,
// что и псевдоним, поэтому сортировка не выдаёт о студенте ничего сверх уже
// показанного псевдонима. Порядок по логину выдавал бы номер студенческого.
func (a aliasMaker) seed(login string) uint64 {
	return binary.BigEndian.Uint64(a.digest(login)[12:20])
}

// resolveAliasCollisions разводит совпавшие псевдонимы, сдвигая суффикс у всех,
// кроме первого по порядку рейтинга. Пространство псевдонимов велико, но на
// когорте в сотни человек совпадение изредка случается и выглядит как баг.
func resolveAliasCollisions(entries []domain.Entry) {
	taken := make(map[string]struct{}, len(entries))

	for i := range entries {
		alias := entries[i].Alias
		if _, busy := taken[alias]; !busy {
			taken[alias] = struct{}{}
			continue
		}

		// Сдвиг идёт по суффиксу: слова псевдонима остаются, меняется хвост
		prefix, suffix, ok := splitAlias(alias)
		if !ok {
			taken[alias] = struct{}{}
			continue
		}
		for shift := 1; shift < aliasSuffixRange; shift++ {
			candidate := fmt.Sprintf("%s 0x%03X", prefix, (suffix+shift)%aliasSuffixRange)
			if _, busy := taken[candidate]; busy {
				continue
			}
			entries[i].Alias = candidate
			taken[candidate] = struct{}{}
			break
		}
	}
}

// splitAlias разбирает псевдоним на словесную часть и числовой суффикс.
func splitAlias(alias string) (prefix string, suffix int, ok bool) {
	for i := len(alias) - 1; i >= 0; i-- {
		if alias[i] != ' ' {
			continue
		}
		if _, err := fmt.Sscanf(alias[i+1:], "0x%X", &suffix); err != nil {
			return "", 0, false
		}
		return alias[:i], suffix, true
	}

	return "", 0, false
}
