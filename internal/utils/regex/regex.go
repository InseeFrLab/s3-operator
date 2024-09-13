package regex

import (
	"github.com/dlclark/regexp2"
)

func Match(pattern, text string) bool {
	compiledRegex, err := regexp2.Compile(pattern, 0)
	if err != nil {
		return false
	}
	regexMatch, err := compiledRegex.MatchString(text)
	if err != nil {
		return false
	}
	return regexMatch
}
