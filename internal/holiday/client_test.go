package holiday

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientFetchMonth_ParsesItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("solYear"); got != "2026" {
			t.Fatalf("expected solYear=2026, got %q", got)
		}
		if got := r.URL.Query().Get("solMonth"); got != "10" {
			t.Fatalf("expected solMonth=10, got %q", got)
		}

		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<response>
  <header>
    <resultCode>00</resultCode>
    <resultMsg>NORMAL SERVICE.</resultMsg>
  </header>
  <body>
    <items>
      <item>
        <dateKind>01</dateKind>
        <dateName>한글날</dateName>
        <isHoliday>Y</isHoliday>
        <locdate>20261009</locdate>
        <seq>1</seq>
      </item>
      <item>
        <dateKind>01</dateKind>
        <dateName>추석연휴</dateName>
        <isHoliday>Y</isHoliday>
        <locdate>20261005</locdate>
        <seq>2</seq>
      </item>
    </items>
  </body>
</response>`))
	}))
	defer server.Close()

	client := NewClient("test-key", 5*time.Second)
	client.SetBaseURL(server.URL)

	items, err := client.FetchMonth(context.Background(), SupportedKinds[1], 2026, time.October)
	if err != nil {
		t.Fatalf("FetchMonth returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].KindCode != "restde" {
		t.Fatalf("expected restde kind code, got %q", items[0].KindCode)
	}
	if !items[1].IsMajorHoliday || items[1].MajorHolidayGroup != "chuseok" {
		t.Fatalf("expected 추석연휴 to be classified as major chuseok, got %+v", items[1])
	}
	if !strings.Contains(string(items[0].SourcePayload), "한글날") {
		t.Fatalf("expected source payload to contain original xml fields")
	}
}

func TestClientFetchMonth_HandlesEmptyMonth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<response>
  <header>
    <resultCode>00</resultCode>
    <resultMsg>NORMAL SERVICE.</resultMsg>
  </header>
  <body>
    <items></items>
  </body>
</response>`))
	}))
	defer server.Close()

	client := NewClient("test-key", 5*time.Second)
	client.SetBaseURL(server.URL)

	items, err := client.FetchMonth(context.Background(), SupportedKinds[0], 2026, time.February)
	if err != nil {
		t.Fatalf("FetchMonth returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %d", len(items))
	}
}
