package text

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/iDigitalFlame/xmt/util"
)

var (
	builders = sync.Pool{
		New: func() interface{} {
			return new(strings.Builder)
		},
	}

	regxFalse = falseRegexp(false)
	regxBuild = regexp.MustCompile(`(\%(\d+f?)?[dhcsuln])`)
)

// String is a wrapper for strings to support the fmt.Stringer interface.
type String string

// Matcher is an alias of a string that can contain specific variable instructsions to be replaced
// when calling the 'String' function. This alias provides the 'Match' function, which returns a Regexp
// struct that will match any value generated.
type Matcher string
type falseRegexp bool

// Regexp is a compatibility interface that is used to allow for UnMatch values to be used lacking
// the ability of RE2 used in Golang to do negative lookahead.
type Regexp interface {
	String() string
	Match([]byte) bool
	MatchString(string) bool
}
type inverseRegexp string

// Raw returns the raw string value of this Matcher without preforming any replacements.
func (s Matcher) Raw() string {
	return string(s)
}

// String returns the string value of itself.
func (s String) String() string {
	return string(s)
}

// Match returns a valid Regexp struct that is guaranteed to match any string generated by the
// Matcher's 'String' function.
func (s Matcher) Match() Regexp {
	return s.MatchEx(true)
}

// String parses this MatchString value and will preform any replacements and fill any variables contained.
func (s Matcher) String() string {
	if len(s) == 0 {
		return string(s)
	}
	m := regxBuild.FindAllStringSubmatchIndex(string(s), -1)
	if len(m) == 0 {
		return string(s)
	}
	var (
		l   int
		err error
		b   = builders.Get().(*strings.Builder)
	)
	for x, v, c := 0, 0, ""; x < len(m); x++ {
		if m[x][0] < 0 || m[x][1] < m[x][0] {
			continue
		}
		if m[x][4] > 0 && m[x][5] > m[x][4] {
			if s[m[x][5]-1] == 'f' {
				v, err = strconv.Atoi(string(s[m[x][4] : m[x][5]-1]))
			} else {
				v, err = strconv.Atoi(string(s[m[x][4]:m[x][5]]))
			}
			if err != nil {
				v = -1
			}
		} else {
			v = -1
		}
		switch {
		case s[m[x][1]-1] == 'n' && s[m[x][1]-2] == 'f' && v > 0:
			c = util.Rand.StringNumber(v)
		case s[m[x][1]-1] == 'c' && s[m[x][1]-2] == 'f' && v > 0:
			c = util.Rand.StringCharacters(v)
		case s[m[x][1]-1] == 'u' && s[m[x][1]-2] == 'f' && v > 0:
			c = util.Rand.StringUpper(v)
		case s[m[x][1]-1] == 'l' && s[m[x][1]-2] == 'f' && v > 0:
			c = util.Rand.StringLower(v)
		case s[m[x][1]-1] == 's' && s[m[x][1]-2] == 'f' && v > 0:
			c = util.Rand.String(v)
		case s[m[x][1]-1] == 'd' && s[m[x][1]-2] == 'f' && v >= 0:
			c = strconv.Itoa(v)
		case s[m[x][1]-1] == 'h' && s[m[x][1]-2] == 'f' && v >= 0:
			c = strconv.FormatInt(int64(v), 16)
		case s[m[x][1]-1] == 'd' && v >= 0:
			c = strconv.Itoa(int(util.FastRandN(v)))
		case s[m[x][1]-1] == 'h' && v >= 0:
			c = strconv.FormatInt(int64(util.FastRandN(v)), 16)
		case s[m[x][1]-1] == 'n' && v > 0:
			c = util.Rand.StringNumberRange(1, v)
		case s[m[x][1]-1] == 'c' && v > 0:
			c = util.Rand.StringCharactersRange(1, v)
		case s[m[x][1]-1] == 'u' && v > 0:
			c = util.Rand.StringUpperRange(1, v)
		case s[m[x][1]-1] == 'l' && v > 0:
			c = util.Rand.StringLowerRange(1, v)
		case s[m[x][1]-1] == 's' && v > 0:
			c = util.Rand.StringRange(1, v)
		case s[m[x][1]-1] == 'd':
			c = strconv.Itoa(int(util.FastRand()))
		case s[m[x][1]-1] == 'h':
			c = strconv.FormatInt(int64(util.FastRand()), 16)
		default:
			c = string(s[m[x][0]:m[x][1]])
		}
		b.WriteString(string(s[l:m[x][0]]))
		b.WriteString(c)
		c, l = "", m[x][1]
	}
	if l < len(s) {
		b.WriteString(string(s[l:]))
	}
	o := b.String()
	b.Reset()
	builders.Put(b)
	return o
}

// UnMatch returns a valid Regexp struct that is guaranteed to not match any string generated by
// the Matcher's 'String' function.
func (s Matcher) UnMatch() Regexp {
	return s.MatchEx(false)
}
func (falseRegexp) String() string {
	return "false"
}
func (i inverseRegexp) String() string {
	return string(i)
}

// MatchEx returns a valid Regexp struct that is guaranteed to match any string generated by the
// Matcher's 'String' function. MatchEx returns an inverse matcher if the bool is false.
func (s Matcher) MatchEx(o bool) Regexp {
	if len(s) == 0 {
		return regxFalse
	}
	m := regxBuild.FindAllStringSubmatchIndex(string(s), -1)
	if len(m) == 0 {
		if !o {
			return inverseRegexp(s)
		}
		if r, err := regexp.Compile(`^(` + regexp.QuoteMeta(string(s)) + `)$`); err == nil {
			return r
		}
		return regxFalse
	}
	var (
		l   int
		err error
		d   string
		b   = builders.Get().(*strings.Builder)
	)
	if b.WriteString("^("); !o {
		d = "^"
	}
	for x, v, c, q := 0, 0, "", ""; x < len(m); x++ {
		if m[x][0] < 0 || m[x][1] < m[x][0] {
			continue
		}
		if m[x][4] > 0 && m[x][5] > m[x][4] {
			if s[m[x][5]-1] == 'f' {
				v, err = strconv.Atoi(string(s[m[x][4] : m[x][5]-1]))
			} else {
				v, err = strconv.Atoi(string(s[m[x][4]:m[x][5]]))
			}
			if err != nil {
				v, q = -1, "0"
			} else {
				q = strconv.Itoa(v)
			}
		} else {
			v = -1
		}
		switch {
		case s[m[x][1]-1] == 'd':
			c = `([` + d + `0-9]+)`
		case s[m[x][1]-1] == 'h':
			c = `([` + d + `a-fA-F0-9]+)`
		case s[m[x][1]-1] == 'n' && s[m[x][1]-2] == 'f' && v > 0:
			c = `([` + d + `0-9]{` + q + `})`
		case s[m[x][1]-1] == 'c' && s[m[x][1]-2] == 'f' && v > 0:
			c = `([` + d + `a-zA-Z]{` + q + `})`
		case s[m[x][1]-1] == 'u' && s[m[x][1]-2] == 'f' && v > 0:
			c = `([` + d + `A-Z]{` + q + `})`
		case s[m[x][1]-1] == 'l' && s[m[x][1]-2] == 'f' && v > 0:
			c = `([` + d + `a-z]{` + q + `})`
		case s[m[x][1]-1] == 's' && s[m[x][1]-2] == 'f' && v > 0:
			c = `([` + d + `a-zA-Z0-9]{` + q + `})`
		case s[m[x][1]-1] == 'n' && v > 0:
			c = `([` + d + `0-9]{1,` + q + `})`
		case s[m[x][1]-1] == 'c' && v > 0:
			c = `([` + d + `a-zA-Z]{1,` + q + `})`
		case s[m[x][1]-1] == 'u' && v > 0:
			c = `([` + d + `A-Z]{1,` + q + `})`
		case s[m[x][1]-1] == 'l' && v > 0:
			c = `([` + d + `a-z]{1,` + q + `})`
		case s[m[x][1]-1] == 's' && v > 0:
			c = `([` + d + `a-zA-Z0-9]{1,` + q + `})`
		case s[m[x][1]-1] == 'n' && v > 0:
			c = `([` + d + `0-9]+)`
		case s[m[x][1]-1] == 'c' && v > 0:
			c = `([` + d + `a-zA-Z]+)`
		case s[m[x][1]-1] == 'u' && v > 0:
			c = `([` + d + `A-Z]+)`
		case s[m[x][1]-1] == 'l' && v > 0:
			c = `([` + d + `a-z]+)`
		case s[m[x][1]-1] == 's' && v > 0:
			c = `([` + d + `a-zA-Z0-9]+)`
		default:
			c = string(s[m[x][0]:m[x][1]])
		}
		b.WriteString(strings.ReplaceAll(regexp.QuoteMeta(string(s[l:m[x][0]])), "/", "\\/"))
		b.WriteString(c)
		l = m[x][1]
	}
	if l < len(s) {
		b.WriteString(strings.ReplaceAll(regexp.QuoteMeta(string(s[l:])), "/", "\\/"))
	}
	b.WriteString(")$")
	r, err := regexp.Compile(b.String())
	b.Reset()
	if builders.Put(b); err != nil {
		return regxFalse
	}
	return r
}
func (falseRegexp) Match(_ []byte) bool {
	return false
}
func (i inverseRegexp) Match(b []byte) bool {
	return !bytes.Equal(b, []byte(i))
}
func (falseRegexp) MatchString(_ string) bool {
	return false
}
func (i inverseRegexp) MatchString(s string) bool {
	return string(i) != s
}
