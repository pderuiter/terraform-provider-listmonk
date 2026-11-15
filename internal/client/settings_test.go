package client

import (
	"encoding/json"
	"testing"
)

func TestIsMasked(t *testing.T) {
	cases := map[string]bool{"": false, "•": true, "••••••": true, "pa••": false, "secret": false}
	for in, want := range cases {
		if got := IsMasked(in); got != want {
			t.Errorf("IsMasked(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSettingMatches(t *testing.T) {
	cases := []struct {
		name               string
		configured, server string
		want               bool
	}{
		{"equal string", `"My list"`, `"My list"`, true},
		{"different string", `"My list"`, `"Other"`, false},
		{"bool", `true`, `true`, true},
		{"bool changed", `true`, `false`, false},
		{"number formats", `10`, `10.0`, true},
		{"number changed", `10`, `11`, false},
		{"type change", `"10"`, `10`, false},
		{"masked secret", `{"password":"hunter22"}`, `{"password":"••••••••"}`, true},
		{"server adds fields", `[{"host":"smtp.example.com","port":587}]`,
			`[{"uuid":"abc","host":"smtp.example.com","port":587,"enabled":true}]`, true},
		{"array length differs", `[{"host":"a"}]`, `[{"host":"a"},{"host":"b"}]`, false},
		{"missing field on server", `{"host":"a","port":25}`, `{"host":"a"}`, false},
		{"null", `null`, `null`, true},
		{"empty array", `[]`, `[]`, true},
		{"invalid configured json", `{`, `{}`, false},
	}
	for _, tc := range cases {
		if got := SettingMatches(json.RawMessage(tc.configured), json.RawMessage(tc.server)); got != tc.want {
			t.Errorf("%s: SettingMatches(%s, %s) = %v, want %v", tc.name, tc.configured, tc.server, got, tc.want)
		}
	}
}

func TestCheckSMTPDurations(t *testing.T) {
	v5 := `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s"}]`
	v6 := `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s","msg_retry_delay":""}]`
	cases := []struct {
		name               string
		configured, server string
		wantErr            bool
	}{
		{"v6 complete", `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s","msg_retry_delay":"10s"}]`, v6, false},
		{"v6 missing msg_retry_delay", `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s"}]`, v6, true},
		{"v6 empty msg_retry_delay", `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s","msg_retry_delay":""}]`, v6, true},
		{"v5 without msg_retry_delay", `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s"}]`, v5, false},
		{"missing idle_timeout", `[{"host":"a","wait_timeout":"5s"}]`, v5, true},
		{"invalid duration", `[{"host":"a","idle_timeout":"15 seconds","wait_timeout":"5s"}]`, v5, true},
		{"second server checked", `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s"},{"host":"b","idle_timeout":"15s"}]`, v5, true},
		{"no server entries", `[{"host":"a","idle_timeout":"15s","wait_timeout":"5s"}]`, `[]`, false},
		{"not a list", `"x"`, v6, false},
	}
	for _, tc := range cases {
		err := CheckSMTPDurations(json.RawMessage(tc.configured), json.RawMessage(tc.server))
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: CheckSMTPDurations(%s, %s) = %v, wantErr %v", tc.name, tc.configured, tc.server, err, tc.wantErr)
		}
	}
}
