package typescriptify

import (
	"strings"
	"unicode"
)

func indentLines(str string, i int) string {
	lines := strings.Split(str, "\n")
	for n := range lines {
		lines[n] = strings.Repeat("\t", i) + lines[n]
	}
	return strings.Join(lines, "\n")
}

type CamelCaseOptions struct {
	PreserveConsecutiveUppercase bool
}

// convert from PascalCase to camelCase
func CamelCase(s string, options *CamelCaseOptions) string {
	preserveConsecutiveUppercase := false
	if options != nil {
		preserveConsecutiveUppercase = options.PreserveConsecutiveUppercase
	}

	var result []rune

	isPrevLower := false
	isPrevDigit := false
	for i, r := range s {
		if i == 0 {
			result = append(result, unicode.ToLower(r))
		} else {
			if isPrevDigit {
				r = unicode.ToUpper(r)
			}
			// If previous character was lower case or the current character is not upper case,
			// append it as it is.
			if isPrevLower || !unicode.IsUpper(r) {
				result = append(result, r)
			} else {
				if !preserveConsecutiveUppercase {
					// prev is Upper and current is also upper
					// Check if next rune is also upper
					if i+1 < len(s) && unicode.IsUpper(rune(s[i+1])) {
						result = append(result, unicode.ToLower(r))
					} else {
						result = append(result, r)
					}
				} else {
					result = append(result, r)
				}
			}
		}
		isPrevLower = !unicode.IsUpper(r)
		isPrevDigit = unicode.IsDigit(r)
	}
	return string(result)
}
