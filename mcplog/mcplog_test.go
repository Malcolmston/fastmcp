package mcplog

import (
	"encoding/json"
	"testing"
)

func TestLevelStringAndParse(t *testing.T) {
	for _, l := range AllLevels() {
		name := l.String()
		got, ok := ParseLevel(name)
		if !ok || got != l {
			t.Errorf("round-trip %v (%q) failed: got %v ok=%v", l, name, got, ok)
		}
	}
	if _, ok := ParseLevel("nope"); ok {
		t.Error("ParseLevel accepted unknown name")
	}
	if got, _ := ParseLevel("WARNING"); got != Warning {
		t.Errorf("case-insensitive parse = %v", got)
	}
}

func TestSeverityMapping(t *testing.T) {
	// RFC 5424: Emergency=0 ... Debug=7.
	cases := map[Level]int{Emergency: 0, Alert: 1, Critical: 2, Error: 3, Warning: 4, Notice: 5, Info: 6, Debug: 7}
	for lvl, sev := range cases {
		if lvl.Severity() != sev {
			t.Errorf("%v.Severity() = %d, want %d", lvl, lvl.Severity(), sev)
		}
		if LevelFromSeverity(sev) != lvl {
			t.Errorf("LevelFromSeverity(%d) = %v, want %v", sev, LevelFromSeverity(sev), lvl)
		}
	}
}

func TestEnabledThreshold(t *testing.T) {
	if !Error.Enabled(Warning) {
		t.Error("Error should be enabled at Warning threshold")
	}
	if Debug.Enabled(Info) {
		t.Error("Debug should be filtered at Info threshold")
	}
}

func TestLevelJSON(t *testing.T) {
	data, err := json.Marshal(Warning)
	if err != nil || string(data) != `"warning"` {
		t.Fatalf("marshal = %s err=%v", data, err)
	}
	var l Level
	if err := json.Unmarshal([]byte(`"critical"`), &l); err != nil || l != Critical {
		t.Fatalf("unmarshal = %v err=%v", l, err)
	}
	if err := json.Unmarshal([]byte(`"bogus"`), &l); err == nil {
		t.Error("unmarshal accepted bogus level")
	}
}

func TestLoggerFiltersAndEmits(t *testing.T) {
	var got []Message
	lg := NewLogger("test", Info, func(m Message) { got = append(got, m) })

	if lg.Debug("hidden") {
		t.Error("Debug should be filtered at Info threshold")
	}
	if !lg.Info("shown") {
		t.Error("Info should be emitted")
	}
	if !lg.Err("boom") {
		t.Error("Err should be emitted")
	}
	if len(got) != 2 {
		t.Fatalf("emitted %d records, want 2", len(got))
	}
	if got[0].Level != Info || got[0].Logger != "test" || got[0].Data != "shown" {
		t.Errorf("record 0 = %+v", got[0])
	}
	if got[1].Level != Error {
		t.Errorf("record 1 level = %v", got[1].Level)
	}

	lg.SetLevel(Error)
	if lg.Info("now hidden") {
		t.Error("Info should be filtered after raising threshold to Error")
	}
	if lg.Level() != Error {
		t.Errorf("Level() = %v", lg.Level())
	}
}

func TestLoggerNilSink(t *testing.T) {
	lg := NewLogger("x", Debug, nil)
	if lg.Info("noop") {
		t.Error("nil sink should not report emission")
	}
}

func TestLogfAndMessage(t *testing.T) {
	var last Message
	lg := NewLogger("", Debug, func(m Message) { last = m })
	lg.Logf(Warning, "value=%d", 42)
	if last.Data != "value=42" {
		t.Errorf("Logf data = %v", last.Data)
	}
	m := NewMessage(Notice, "svc", map[string]any{"k": "v"})
	if m.Level != Notice || m.Logger != "svc" {
		t.Errorf("NewMessage = %+v", m)
	}
}
