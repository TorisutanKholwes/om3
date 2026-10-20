package backup

import (
	"fmt"
	"strconv"
	"strings"
)

type (
	// Retention says which snapshots a prune keeps: the most recent of each
	// day of the last Days days, of each week of the last Weeks weeks, of
	// each month of the last Months months and of each year of the last
	// Years years. A snapshot kept by one rule is kept.
	//
	// To confirm with OpenSVC: this reading of the keep keyword.
	Retention struct {
		Days   int `json:"days"`
		Weeks  int `json:"weeks"`
		Months int `json:"months"`
		Years  int `json:"years"`
	}
)

// ParseRetention parses a keep value, a space separated list of
// "<n><unit>" words, the unit being "d", "w", "m" or "y". For example,
// "7d 4w 6m" keeps one snapshot per day over the last seven days, one per
// week over the last four weeks and one per month over the last six months.
//
// An empty value is the zero Retention, with which no prune is done.
func ParseRetention(s string) (Retention, error) {
	var r Retention
	seen := make(map[byte]bool)
	for _, word := range strings.Fields(s) {
		if len(word) < 2 {
			return Retention{}, fmt.Errorf("invalid retention %q: must be <n>d, <n>w, <n>m or <n>y", word)
		}
		unit := word[len(word)-1]
		n, err := strconv.Atoi(word[:len(word)-1])
		if err != nil || n <= 0 {
			return Retention{}, fmt.Errorf("invalid retention %q: the count must be a positive integer", word)
		}
		if seen[unit] {
			return Retention{}, fmt.Errorf("invalid retention %q: unit %c given twice", s, unit)
		}
		seen[unit] = true
		switch unit {
		case 'd':
			r.Days = n
		case 'w':
			r.Weeks = n
		case 'm':
			r.Months = n
		case 'y':
			r.Years = n
		default:
			return Retention{}, fmt.Errorf("invalid retention %q: unknown unit %c, must be d, w, m or y", word, unit)
		}
	}
	return r, nil
}

// IsZero returns true if the retention keeps nothing in particular, in which
// case no prune is to be done.
func (r Retention) IsZero() bool {
	return r == Retention{}
}

// String returns the keep value the retention was parsed from.
func (r Retention) String() string {
	l := make([]string, 0, 4)
	for _, e := range []struct {
		n    int
		unit string
	}{{r.Days, "d"}, {r.Weeks, "w"}, {r.Months, "m"}, {r.Years, "y"}} {
		if e.n > 0 {
			l = append(l, strconv.Itoa(e.n)+e.unit)
		}
	}
	return strings.Join(l, " ")
}
