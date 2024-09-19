package glob

import (
	"strings"

	"github.com/InseeFrLab/s3-operator/internal/utils/regex"
	"github.com/gobwas/glob"
)

const (
	EXACT  = "exact"
	GLOB   = "glob"
	REGEXP = "regexp"
)

func Match(pattern, text string, separators ...rune) bool {
	compiledGlob, err := glob.Compile(pattern, separators...)
	if err != nil {
		return false
	}
	return compiledGlob.Match(text)
}

// MatchStringInList will return true if item is contained in list.
// patternMatch; can be set to  exact, glob, regexp.
// If patternMatch; is set to exact, the item must be an exact match.
// If patternMatch; is set to glob, the item must match a glob pattern.
// If patternMatch; is set to regexp, the item must match a regular expression or glob.
func MatchStringInList(list []string, item string, patternMatch string) bool {
	for _, ll := range list {
		// If string is wrapped in "/", assume it is a regular expression.
		if patternMatch == REGEXP && strings.HasPrefix(ll, "/") && strings.HasSuffix(ll, "/") && regex.Match(ll[1:len(ll)-1], item) {
			return true
		} else if (patternMatch == REGEXP || patternMatch == GLOB) && Match(ll, item) {
			return true
		} else if patternMatch == EXACT && item == ll {
			return true
		}
	}
	return false
}
