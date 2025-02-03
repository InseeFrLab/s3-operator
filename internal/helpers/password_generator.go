package helpers

import (
	"crypto/rand"
	"io"
	"math/big"
	"strings"
)

type PasswordGenerator struct {
}

func NewPasswordGenerator() *PasswordGenerator {
	return &PasswordGenerator{}
}

const (
	// LowerLetters is the list of lowercase letters.
	LowerLetters = "abcdefghijklmnopqrstuvwxyz"

	// UpperLetters is the list of uppercase letters.
	UpperLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Digits is the list of permitted digits.
	Digits = "0123456789"

	// Symbols is the list of symbols.
	Symbols = "~!@#$%^&*()_+`-={}|[]\\:\"<>?,./"
)

// func GeneratePassword(length int, useLetters bool, useSpecial bool, useNum bool) string {
func (p *PasswordGenerator) Generate(length int, useLetters bool, useSpecial bool, useNum bool) (string, error) {
	gen, err := NewGenerator(nil)
	if err != nil {
		return "", err
	}
	return gen.Generate(length, true, false, true, true, true)
}

// Generate generates a password with the given requirements. length is the
// total number of characters in the password. numDigits is the number of digits
// to include in the result. numSymbols is the number of symbols to include in
// the result. noUpper excludes uppercase letters from the results. allowRepeat
// allows characters to repeat.
//
// The algorithm is fast, but it's not designed to be performant; it favors
// entropy over speed. This function is safe for concurrent use.
func (g *Generator) Generate(
	length int,
	useDigit bool,
	useSymbol bool,
	useUpper bool,
	useLower bool,
	allowRepeat bool,
) (string, error) {
	choices := ""

	if useDigit {
		choices += g.digits
	}

	if useSymbol {
		choices += g.symbols
	}

	if useUpper {
		choices += g.upperLetters
	}

	if useLower {
		choices += g.lowerLetters
	}

	if len(choices) == 0 {
		choices += g.lowerLetters
	}

	var result string

	for i := 0; i < length; i++ {
		ch, err := randomElement(g.reader, choices)
		if err != nil {
			return "", err
		}

		if !allowRepeat && strings.Contains(result, ch) {
			i--
			continue
		}

		result, err = randomInsert(g.reader, result, ch)
		if err != nil {
			return "", err
		}
	}

	return result, nil
}

// Generator is the stateful generator which can be used to customize the list
// of letters, digits, and/or symbols.
type Generator struct {
	lowerLetters string
	upperLetters string
	digits       string
	symbols      string
	reader       io.Reader
}

// GeneratorInput is used as input to the NewGenerator function.
type GeneratorInput struct {
	LowerLetters string
	UpperLetters string
	Digits       string
	Symbols      string
	Reader       io.Reader // rand.Reader by default
}

// NewGenerator creates a new Generator from the specified configuration. If no
// input is given, all the default values are used. This function is safe for
// concurrent use.
func NewGenerator(i *GeneratorInput) (*Generator, error) {
	if i == nil {
		i = new(GeneratorInput)
	}

	g := &Generator{
		lowerLetters: i.LowerLetters,
		upperLetters: i.UpperLetters,
		digits:       i.Digits,
		symbols:      i.Symbols,
		reader:       i.Reader,
	}

	if g.lowerLetters == "" {
		g.lowerLetters = LowerLetters
	}

	if g.upperLetters == "" {
		g.upperLetters = UpperLetters
	}

	if g.digits == "" {
		g.digits = Digits
	}

	if g.symbols == "" {
		g.symbols = Symbols
	}

	if g.reader == nil {
		g.reader = rand.Reader
	}

	return g, nil
}

// randomInsert randomly inserts the given value into the given string.
func randomInsert(reader io.Reader, s, val string) (string, error) {
	if s == "" {
		return val, nil
	}

	n, err := rand.Int(reader, big.NewInt(int64(len(s)+1)))
	if err != nil {
		return "", err
	}
	i := n.Int64()
	return s[0:i] + val + s[i:], nil
}

// randomElement extracts a random element from the given string.
func randomElement(reader io.Reader, s string) (string, error) {
	n, err := rand.Int(reader, big.NewInt(int64(len(s))))
	if err != nil {
		return "", err
	}
	return string(s[n.Int64()]), nil
}
