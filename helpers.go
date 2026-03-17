package salamoonder

import (
	"fmt"
	"regexp"
)

var chromeVersionRe = regexp.MustCompile(`Chrome/(\d+)`)

func ExtractSecChUa(userAgent string) string {
	match := chromeVersionRe.FindStringSubmatch(userAgent)
	if match == nil {
		return `"Chromium";v="122", "Google Chrome";v="122", "Not?A_Brand";v="99"`
	}
	v := match[1]
	return fmt.Sprintf(`"Chromium";v="%s", "Google Chrome";v="%s", "Not?A_Brand";v="99"`, v, v)
}
