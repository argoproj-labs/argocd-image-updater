package tag

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// DefaultCalVerLayout is the layout assumed when none was configured. It is
// the canonical scheme of https://calver.org: zero padded and dot separated.
// The padded tokens are deliberate, because the lenient ones decode "2026.1.3"
// and "2026.01.03" to the same version, and a repository carrying both
// spellings would then see its image rewritten back and forth between them.
const DefaultCalVerLayout = "YYYY.0M.0D"

// calVerField identifies a single segment of a CalVer scheme. The date fields
// are declared first and in the order of their significance, which is the
// order they are compared in regardless of how the layout spells them out.
// Every other field is compared in the position the layout gives it.
type calVerField int

const (
	fieldYear calVerField = iota
	fieldMonth
	fieldWeek
	fieldDay
	fieldMajor
	fieldMinor
	fieldMicro
	fieldModifier
	numCalVerFields
)

// lastDateField is the last field that forms part of the date.
const lastDateField = fieldDay

// calVerToken maps a layout token to the field it denotes and to what it may
// consume from a tag. The token set is the one described at https://calver.org:
// the zero prefixed tokens require padding, their unprefixed counterparts do
// not.
type calVerToken struct {
	token string
	field calVerField
	// shortYear marks the tokens that count years from 2000, i.e. YY and 0Y.
	shortYear bool
	// text marks MODIFIER, the only token that matches something other than a
	// run of digits.
	text bool
	// optionalTail marks the fields that a tag may leave out when the layout
	// writes them at its very end. CalVer schemes routinely drop a trailing
	// minor, micro or modifier segment, so "YYYY.0M.MICRO" has to accept both
	// "2026.01" and "2026.01.3".
	optionalTail bool
	// minDigits and maxDigits bound the field when a literal or the end of the
	// tag marks where it ends. A maxDigits of 0 means unbounded.
	minDigits int
	maxDigits int
	// adjacentWidth is the exact number of digits consumed when the token is
	// directly followed by another token. Zero means the token is of variable
	// width and may not be placed next to another field at all.
	adjacentWidth int
}

// calVerTokens holds the recognized layout tokens. A token that is a prefix of
// another one must come after it, so that YYYY wins over YY.
var calVerTokens = []calVerToken{
	{token: "YYYY", field: fieldYear, minDigits: 4, maxDigits: 4, adjacentWidth: 4},
	{token: "YY", field: fieldYear, shortYear: true, minDigits: 1, maxDigits: 3},
	// A short year may run to three digits from 2100 on, but the adjacent form
	// has to commit to a width, so 0Y written next to another field caps at
	// two digits and a tag like "10601" is left unmatched rather than guessed.
	{token: "0Y", field: fieldYear, shortYear: true, minDigits: 2, maxDigits: 3, adjacentWidth: 2},
	{token: "0M", field: fieldMonth, minDigits: 2, maxDigits: 2, adjacentWidth: 2},
	{token: "0W", field: fieldWeek, minDigits: 2, maxDigits: 2, adjacentWidth: 2},
	{token: "0D", field: fieldDay, minDigits: 2, maxDigits: 2, adjacentWidth: 2},
	{token: "MAJOR", field: fieldMajor, minDigits: 1},
	{token: "MINOR", field: fieldMinor, minDigits: 1, optionalTail: true},
	{token: "MICRO", field: fieldMicro, minDigits: 1, optionalTail: true},
	{token: "MODIFIER", field: fieldModifier, text: true, optionalTail: true},
	{token: "MM", field: fieldMonth, minDigits: 1, maxDigits: 2},
	{token: "WW", field: fieldWeek, minDigits: 1, maxDigits: 2},
	{token: "DD", field: fieldDay, minDigits: 1, maxDigits: 2},
}

// calVerTokenNames lists the tokens for use in error messages.
const calVerTokenNames = "YYYY, YY, 0Y, MM, 0M, WW, 0W, DD, 0D, MAJOR, MINOR, MICRO and MODIFIER"

// calVerElement is either a literal run of characters or a single token.
type calVerElement struct {
	literal string
	token   *calVerToken
}

// CalVerLayout describes how a version is encoded in an image tag, e.g.
// "vYYYY-0M-0D", "MAJOR.YY.0M" or "YYYY.MINOR.MICRO-MODIFIER". Use
// NewCalVerLayout to construct one.
type CalVerLayout struct {
	raw      string
	elements []calVerElement
	present  [numCalVerFields]bool
	// order lists the fields in the order two tags are compared in. It follows
	// the order the layout writes its segments in, which is what makes a
	// scheme such as "MAJOR.YY.0M" rank the major segment above the date.
	order []calVerField
	// optionalAt marks the element indices a tag is allowed to end at. Only
	// the start of a trailing optional segment is marked, so that ending one
	// element later, with the separator consumed but the segment missing, is
	// still an error.
	optionalAt []bool
	// optionalVPrefix is set when the layout starts with a token rather than
	// with a literal, in which case a leading "v" in the tag is tolerated.
	optionalVPrefix bool
}

// NewCalVerLayout parses a CalVer layout definition. An empty layout yields
// DefaultCalVerLayout.
func NewCalVerLayout(layout string) (*CalVerLayout, error) {
	if layout == "" {
		layout = DefaultCalVerLayout
	}

	l := &CalVerLayout{raw: layout}
	var literal strings.Builder
	// claimedBy records which token claimed a field, so that a collision can
	// name both sides of it.
	var claimedBy [numCalVerFields]string

	flushLiteral := func() {
		if literal.Len() > 0 {
			l.elements = append(l.elements, calVerElement{literal: literal.String()})
			literal.Reset()
		}
	}

	for pos := 0; pos < len(layout); {
		var matched *calVerToken
		for i := range calVerTokens {
			if strings.HasPrefix(layout[pos:], calVerTokens[i].token) {
				matched = &calVerTokens[i]
				break
			}
		}

		if matched == nil {
			// A digit in a literal would make it impossible to tell where a
			// variable width field ends, and an uppercase Y, M, W or D is
			// almost certainly a mistyped token rather than something the tag
			// is meant to spell out. Accepting either would produce a layout
			// that validates but matches no tag at all.
			if c := layout[pos]; isDigit(c) || c == 'Y' || c == 'M' || c == 'W' || c == 'D' {
				return nil, fmt.Errorf("calver layout '%s' contains '%c' outside of a token: valid tokens are %s", l.raw, c, calVerTokenNames)
			}
			literal.WriteByte(layout[pos])
			pos++
			continue
		}

		if l.present[matched.field] {
			return nil, fmt.Errorf("calver layout '%s' defines the same field twice, with %s and %s", l.raw, claimedBy[matched.field], matched.token)
		}

		l.present[matched.field] = true
		claimedBy[matched.field] = matched.token
		flushLiteral()
		l.elements = append(l.elements, calVerElement{token: matched})
		pos += len(matched.token)
	}
	flushLiteral()

	if err := l.validate(); err != nil {
		return nil, err
	}

	l.order = l.comparisonOrder()
	l.optionalAt = l.optionalTailStarts()
	l.optionalVPrefix = l.elements[0].token != nil

	return l, nil
}

// validate rejects the layouts that cannot be matched or cannot be ordered.
func (l *CalVerLayout) validate() error {
	switch {
	case len(l.elements) == 0:
		return fmt.Errorf("calver layout '%s' is empty", l.raw)
	case !l.present[fieldYear]:
		return fmt.Errorf("calver layout '%s' must contain a year field (YYYY, YY or 0Y)", l.raw)
	case l.present[fieldWeek] && (l.present[fieldMonth] || l.present[fieldDay]):
		return fmt.Errorf("calver layout '%s' must not combine a week field with a month or day field", l.raw)
	case l.present[fieldDay] && !l.present[fieldMonth]:
		return fmt.Errorf("calver layout '%s' contains a day but no month, which cannot be ordered", l.raw)
	}

	// The date fields are compared by significance rather than in the order
	// they are written, because a date means the same thing whether it is
	// spelled YYYY-0M-0D or 0D-0M-YYYY. That only holds while the date is one
	// uninterrupted run: in a layout such as "YY.MINOR.0M" it is impossible to
	// say whether the minor segment outranks the month or the other way round.
	var dateSeen, dateDone bool
	for _, el := range l.elements {
		if el.token == nil {
			continue
		}
		if el.token.field <= lastDateField {
			if dateDone {
				return fmt.Errorf("calver layout '%s' splits the date around %s: write the date fields next to each other", l.raw, el.token.token)
			}
			dateSeen = true
			continue
		}
		if dateSeen {
			dateDone = true
		}
	}

	// Where the numbered segments sit relative to the date is the layout's
	// choice, but MAJOR, MINOR and MICRO are names for the first, second and
	// third number of a scheme, so their order among themselves is not. A
	// layout spelling them the other way round is a typo, and is turned away
	// for the same reason a lone day is.
	var lastNumbered *calVerToken
	for _, el := range l.elements {
		if el.token == nil || el.token.field < fieldMajor || el.token.field > fieldMicro {
			continue
		}
		if lastNumbered != nil && el.token.field < lastNumbered.field {
			return fmt.Errorf("calver layout '%s' places %s before %s, but %s is the more significant of the two", l.raw, lastNumbered.token, el.token.token, el.token.token)
		}
		lastNumbered = el.token
	}

	for i, el := range l.elements {
		if el.token == nil {
			continue
		}
		// MODIFIER matches text rather than digits, so nothing could mark
		// where it ends.
		if el.token.field == fieldModifier && i != len(l.elements)-1 {
			return fmt.Errorf("calver layout '%s' places MODIFIER before the end of the layout: MODIFIER matches the remainder of the tag and must come last", l.raw)
		}
		// Two fields written next to each other can only be told apart when
		// the first one has a fixed width, which is what the zero padded
		// tokens give.
		if i+1 < len(l.elements) && l.elements[i+1].token != nil && el.token.adjacentWidth == 0 {
			return fmt.Errorf("calver layout '%s' places the variable width %s directly before another field: separate them or use the zero padded form", l.raw, el.token.token)
		}
	}

	return nil
}

// comparisonOrder derives the order two tags are compared in from the order
// the layout writes its segments in. The date counts as a single segment,
// placed where the layout first mentions it and compared by significance.
func (l *CalVerLayout) comparisonOrder() []calVerField {
	order := make([]calVerField, 0, numCalVerFields)
	dateAdded := false
	for _, el := range l.elements {
		if el.token == nil {
			continue
		}
		if el.token.field > lastDateField {
			order = append(order, el.token.field)
			continue
		}
		if dateAdded {
			continue
		}
		dateAdded = true
		for _, f := range []calVerField{fieldYear, fieldMonth, fieldWeek, fieldDay} {
			if l.present[f] {
				order = append(order, f)
			}
		}
	}
	return order
}

// optionalTailStarts marks the element indices a tag may end at. Only a run of
// trailing minor, micro and modifier segments is optional, and only when a
// literal separates each of them from what comes before, so that a dropped
// segment takes its separator with it.
func (l *CalVerLayout) optionalTailStarts() []bool {
	at := make([]bool, len(l.elements))
	for i := len(l.elements) - 1; i >= 1; i -= 2 {
		if l.elements[i].token == nil || !l.elements[i].token.optionalTail {
			break
		}
		if l.elements[i-1].token != nil {
			break
		}
		at[i-1] = true
	}
	return at
}

// String returns the layout as it was configured.
func (l *CalVerLayout) String() string {
	return l.raw
}

// calVerVersion is a tag name decoded into its segments.
type calVerVersion struct {
	tagName  string
	fields   [numCalVerFields]int
	modifier string
}

// compare orders two decoded tags along the layout's comparison order. Tags
// denoting the same version are ordered by name so that the result stays
// stable, which matters for equivalent spellings such as "-1" and "-01".
func (v *calVerVersion) compare(o *calVerVersion, order []calVerField) int {
	for _, f := range order {
		if f == fieldModifier {
			if c := compareCalVerModifier(v.modifier, o.modifier); c != 0 {
				return c
			}
			continue
		}
		if v.fields[f] != o.fields[f] {
			if v.fields[f] < o.fields[f] {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(v.tagName, o.tagName)
}

// compareCalVerModifier orders the MODIFIER segment. A tag carrying one is a
// pre-release of the tag that does not, so the empty modifier ranks highest,
// the same way semver ranks a release above its pre-releases. Two modifiers
// are compared in natural order, so that "rc2" ranks below "rc10".
func compareCalVerModifier(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	return compareNatural(a, b)
}

// parse decodes a tag name according to the layout. It returns an error when
// the tag does not match, which callers are expected to treat as "not a
// version" rather than as a failure.
func (l *CalVerLayout) parse(tagName string) (*calVerVersion, error) {
	rest := tagName

	// A leading "v" is such a widespread convention that we accept it even
	// when the layout does not spell it out.
	if l.optionalVPrefix && len(rest) > 1 && (rest[0] == 'v' || rest[0] == 'V') && isDigit(rest[1]) {
		rest = rest[1:]
	}

	v := &calVerVersion{tagName: tagName}

	for i, el := range l.elements {
		// A trailing minor, micro or modifier segment may be left out, in
		// which case the tag simply ends here.
		if rest == "" && l.optionalAt[i] {
			break
		}

		if el.token == nil {
			if !strings.HasPrefix(rest, el.literal) {
				return nil, fmt.Errorf("expected '%s' at '%s'", el.literal, rest)
			}
			rest = rest[len(el.literal):]
			continue
		}

		if el.token.text {
			// MODIFIER is always the last element, so it takes whatever is
			// left of the tag.
			if !isCalVerModifier(rest) {
				return nil, fmt.Errorf("'%s' is not a valid MODIFIER", rest)
			}
			v.modifier = rest
			rest = ""
			continue
		}

		var digits string
		if i+1 < len(l.elements) && l.elements[i+1].token != nil {
			// The next element is a field as well, so nothing marks the end
			// of this one and it has to be padded to its full width.
			if len(rest) < el.token.adjacentWidth || !isAllDigits(rest[:el.token.adjacentWidth]) {
				return nil, fmt.Errorf("expected %d digits for %s at '%s'", el.token.adjacentWidth, el.token.token, rest)
			}
			digits = rest[:el.token.adjacentWidth]
		} else {
			digits = leadingDigits(rest)
			if len(digits) < el.token.minDigits || (el.token.maxDigits > 0 && len(digits) > el.token.maxDigits) {
				return nil, fmt.Errorf("expected %s to match at '%s'", el.token.token, rest)
			}
		}
		rest = rest[len(digits):]

		n, err := strconv.Atoi(digits)
		if err != nil {
			// The only way Atoi fails here is a run of digits too long to fit
			// an int, which no real segment ever is.
			return nil, fmt.Errorf("%s is %d digits long, which is too large to be a version segment", el.token.token, len(digits))
		}
		if el.token.shortYear {
			// Short years count from 2000, per calver.org, so 6 is 2006 and
			// 106 is 2106.
			n += 2000
		}
		v.fields[el.token.field] = n
	}

	if rest != "" {
		return nil, fmt.Errorf("unexpected trailing '%s'", rest)
	}

	if err := l.validateDate(v); err != nil {
		return nil, err
	}

	return v, nil
}

// validateDate rejects a tag whose date fields do not denote a real date.
func (l *CalVerLayout) validateDate(v *calVerVersion) error {
	month, day := v.fields[fieldMonth], v.fields[fieldDay]

	if l.present[fieldMonth] && (month < 1 || month > 12) {
		return fmt.Errorf("month %d is out of range", month)
	}
	// Week 00 is a real week under the %W and %U conventions, which count the
	// days before the first week day of the year, so a tag built with
	// "date +%Y.%W" in early January must not be dropped.
	if l.present[fieldWeek] && v.fields[fieldWeek] > 53 {
		return fmt.Errorf("week %d is out of range", v.fields[fieldWeek])
	}
	if l.present[fieldDay] {
		if day < 1 || day > 31 {
			return fmt.Errorf("day %d is out of range", day)
		}
		// A day is only ever written together with a month, so the day count
		// of that month is known and a tag such as "2026-02-31" can be turned
		// away instead of being ordered as if February had 31 days.
		d := time.Date(v.fields[fieldYear], time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if d.Day() != day || int(d.Month()) != month {
			return fmt.Errorf("%d-%02d-%02d is not a valid date", v.fields[fieldYear], month, day)
		}
	}

	return nil
}

// SortByCalVer returns a SortableImageTagList sorted by the version encoded in
// the tag names, oldest first. Tags that do not match the layout are left out,
// the same way SortBySemVer drops tags that are not valid semver. A nil layout
// is treated as DefaultCalVerLayout.
func (il *ImageTagList) SortByCalVer(ctx context.Context, layout *CalVerLayout) SortableImageTagList {
	logCtx := log.LoggerFromContext(ctx)

	if layout == nil {
		// The default layout is a constant, so this cannot fail.
		layout, _ = NewCalVerLayout("")
	}

	il.lock.RLock()
	defer il.lock.RUnlock()

	versions := make([]*calVerVersion, 0, len(il.items))
	for _, t := range il.items {
		cv, err := layout.parse(t.TagName)
		if err != nil {
			logCtx.Debugf("could not parse input tag %s using calver layout %s: %v", t.TagName, layout, err)
			continue
		}
		versions = append(versions, cv)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].compare(versions[j], layout.order) < 0
	})

	sil := make(SortableImageTagList, 0, len(versions))
	for _, cv := range versions {
		sil = append(sil, il.items[cv.tagName])
	}
	return sil
}

// compareNatural compares two strings so that embedded numbers are ordered by
// their value rather than by their digits, making "rc2" rank below "rc10".
func compareNatural(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			// Comparing the digits rather than their value keeps a modifier
			// with an absurdly long number from overflowing an int.
			da, db := leadingDigits(a), leadingDigits(b)
			ta, tb := strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if len(ta) != len(tb) {
				if len(ta) < len(tb) {
					return -1
				}
				return 1
			}
			if c := strings.Compare(ta, tb); c != 0 {
				return c
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	return strings.Compare(a, b)
}

// isCalVerModifier reports whether s is a non-empty run of the characters a
// MODIFIER may consist of. The set is the one an image tag allows, minus the
// characters that would make a tag ambiguous to read back.
func isCalVerModifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', isDigit(c):
		case c == '.' || c == '-' || c == '_':
		default:
			return false
		}
	}
	return true
}

// leadingDigits returns the longest run of digits at the start of s.
func leadingDigits(s string) string {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i]
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
