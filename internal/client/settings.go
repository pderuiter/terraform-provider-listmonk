package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// maskRune is what listmonk replaces each character of a secret with in
// GET /api/settings (cmd/settings.go pwdMask).
const maskRune = '•'

// IsMasked reports whether s is a secret masked by listmonk: a non-empty
// string made only of mask characters.
func IsMasked(s string) bool {
	if s == "" {
		return false
	}
	return strings.Trim(s, string(maskRune)) == ""
}

// SettingMatches reports whether the server value of a setting still
// satisfies the configured value, so Terraform shows no diff.
//
// The server value may legitimately differ from what was written:
//   - secrets (SMTP passwords, API keys, ...) come back masked;
//   - objects gain server-side fields (e.g. a uuid on every SMTP server).
//
// So the configured value matches when every field it sets is present on the
// server with the same value, a masked server string matches any configured
// string, and arrays match element by element with equal length.
func SettingMatches(configured, server json.RawMessage) bool {
	var c, s any
	if err := decodeJSON(configured, &c); err != nil {
		return false
	}
	if err := decodeJSON(server, &s); err != nil {
		return false
	}
	return subsetMatch(c, s)
}

func subsetMatch(c, s any) bool {
	switch cv := c.(type) {
	case map[string]any:
		sv, ok := s.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range cv {
			sub, ok := sv[k]
			if !ok || !subsetMatch(v, sub) {
				return false
			}
		}
		return true
	case []any:
		sv, ok := s.([]any)
		if !ok || len(sv) != len(cv) {
			return false
		}
		for i := range cv {
			if !subsetMatch(cv[i], sv[i]) {
				return false
			}
		}
		return true
	case string:
		sv, ok := s.(string)
		if !ok {
			return false
		}
		return sv == cv || IsMasked(sv)
	case json.Number:
		sv, ok := s.(json.Number)
		if !ok {
			return false
		}
		if sv == cv {
			return true
		}
		// 10 and 10.0 are the same number.
		cf, err1 := cv.Float64()
		sf, err2 := sv.Float64()
		return err1 == nil && err2 == nil && cf == sf
	default:
		// bool, nil
		return c == s
	}
}

// smtpDurationFields are the duration strings of an SMTP server entry.
// idle_timeout and wait_timeout exist in every supported listmonk version;
// msg_retry_delay was added in v6 and is only checked when the server knows it.
var smtpDurationFields = []string{"idle_timeout", "wait_timeout", "msg_retry_delay"}

// CheckSMTPDurations reports an error when a configured SMTP server leaves out
// a duration field that the server has, or sets one that is not a duration.
//
// PUT /api/settings/smtp stores the value as given, so a missing field is not
// noticed at first. The next full settings save (PUT /api/settings) fills it
// in as "", and listmonk then fails to start on its next reload with
// "time: invalid duration". The fields the server has are read from its
// current entries; with no entries only the fields every version has are
// checked.
func CheckSMTPDurations(configured, server json.RawMessage) error {
	var cfg []map[string]any
	if err := json.Unmarshal(configured, &cfg); err != nil {
		// Not a list of servers; listmonk itself rejects or ignores it.
		return nil
	}
	fields := knownSMTPDurations(server)
	for i, entry := range cfg {
		for _, f := range fields {
			if err := checkSMTPDuration(i, entry, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// knownSMTPDurations returns the duration fields that the server's SMTP
// entries carry, always including those every version has.
func knownSMTPDurations(server json.RawMessage) []string {
	var srv []map[string]any
	_ = json.Unmarshal(server, &srv)

	known := []string{"idle_timeout", "wait_timeout"}
	for _, f := range smtpDurationFields[len(known):] {
		for _, entry := range srv {
			if _, ok := entry[f]; ok {
				known = append(known, f)
				break
			}
		}
	}
	return known
}

// checkSMTPDuration checks one duration field of SMTP server i.
func checkSMTPDuration(i int, entry map[string]any, field string) error {
	v, _ := entry[field].(string)
	if v == "" {
		return fmt.Errorf("SMTP server %d (host %v) does not set %s. listmonk stores a missing duration as \"\" "+
			"on the next full settings save and then fails to start (\"time: invalid duration\"); "+
			"set it to a duration such as \"10s\"", i, entry["host"], field)
	}
	if _, err := time.ParseDuration(v); err != nil {
		return fmt.Errorf("SMTP server %d (host %v): %s %q is not a duration such as \"10s\"", i, entry["host"], field, v)
	}
	return nil
}

// blankMasked returns v with every masked secret string replaced by "".
func blankMasked(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = blankMasked(e)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = blankMasked(e)
		}
		return t
	case string:
		if IsMasked(t) {
			return ""
		}
		return t
	default:
		return v
	}
}

func decodeJSON(raw json.RawMessage, out *any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(out)
}
