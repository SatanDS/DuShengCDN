package security

import "regexp"

const RedactedValue = "<redacted>"

var (
	basicAuthExpectedHashPattern = regexp.MustCompile(`(?i)(local\s+expected_hash\s*=\s*)["'][^"']*["']`)
	basicAuthHashJSONPattern     = regexp.MustCompile(`(?i)("(?:basic_auth_password_hash|expected_hash)"\s*:\s*")[^"]*(")`)
	proxySetHeaderPattern        = regexp.MustCompile(`(?im)^(\s*proxy_set_header\s+(?:authorization|cookie|x-api-key|x-auth-token|x-access-token|x-secret|x-token|api-key|apikey)\s+)([^;]*)(;\s*)$`)
	bearerPattern                = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	basicPattern                 = regexp.MustCompile(`(?i)\bBasic\s+[A-Za-z0-9._~+/=-]+`)
	cookiePattern                = regexp.MustCompile(`(?i)\bCookie:\s*[^\r\n;]+(?:;[^\r\n]*)?`)
	keyValueSecretPattern        = regexp.MustCompile(`(?i)\b([A-Za-z0-9_.-]*(?:token|secret|password|passwd|pwd|api[_-]?key|access[_-]?key|session|signature|authorization|credential)[A-Za-z0-9_.-]*\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&]+)`)
	querySecretPattern           = regexp.MustCompile(`(?i)([?&](?:access_token|auth|authorization|code|credential|key|password|passwd|pwd|secret|session|signature|state|token|api_key|apikey|x-api-key)=)[^&#\s]+`)
)

func RedactSensitiveText(value string) string {
	if value == "" {
		return ""
	}
	value = basicAuthExpectedHashPattern.ReplaceAllString(value, `${1}"`+RedactedValue+`"`)
	value = basicAuthHashJSONPattern.ReplaceAllString(value, `${1}`+RedactedValue+`${2}`)
	value = proxySetHeaderPattern.ReplaceAllString(value, `${1}"`+RedactedValue+`"${3}`)
	value = bearerPattern.ReplaceAllString(value, "Bearer "+RedactedValue)
	value = basicPattern.ReplaceAllString(value, "Basic "+RedactedValue)
	value = cookiePattern.ReplaceAllString(value, "Cookie: "+RedactedValue)
	value = keyValueSecretPattern.ReplaceAllString(value, `${1}`+RedactedValue)
	value = querySecretPattern.ReplaceAllString(value, `${1}`+RedactedValue)
	return value
}
