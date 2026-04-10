package holiday

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/model"
)

const defaultBaseURL = "http://apis.data.go.kr/B090041/openapi/service/SpcdeInfoService"

type Client struct {
	baseURL    string
	serviceKey string
	httpClient *http.Client
}

type SpecialDayKind struct {
	Endpoint  string
	Code      string
	Label     string
	URLSuffix string
}

var SupportedKinds = []SpecialDayKind{
	{Endpoint: "anniversary", Code: "anniv", Label: "기념일", URLSuffix: "getAnniversaryInfo"},
	{Endpoint: "rest_de", Code: "restde", Label: "공휴일", URLSuffix: "getRestDeInfo"},
	{Endpoint: "holi_de", Code: "holide", Label: "국경일", URLSuffix: "getHoliDeInfo"},
	{Endpoint: "divisions_24", Code: "div24", Label: "24절기", URLSuffix: "get24DivisionsInfo"},
	{Endpoint: "sundry_day", Code: "sundry", Label: "잡절", URLSuffix: "getSundryDayInfo"},
}

type apiResponse struct {
	Header struct {
		ResultCode string `xml:"resultCode"`
		ResultMsg  string `xml:"resultMsg"`
	} `xml:"header"`
	Body struct {
		Items struct {
			Items []apiItem `xml:"item"`
		} `xml:"items"`
	} `xml:"body"`
}

type apiItem struct {
	DateKind  string `xml:"dateKind"`
	DateName  string `xml:"dateName"`
	IsHoliday string `xml:"isHoliday"`
	LocDate   string `xml:"locdate"`
	Seq       string `xml:"seq"`
}

func NewClient(serviceKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &Client{
		baseURL:    defaultBaseURL,
		serviceKey: strings.TrimSpace(serviceKey),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) SetBaseURL(baseURL string) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	c.baseURL = strings.TrimRight(baseURL, "/")
}

func (c *Client) FetchMonth(ctx context.Context, kind SpecialDayKind, year int, month time.Month) ([]model.SpecialDay, error) {
	if c == nil {
		return nil, fmt.Errorf("holiday client is nil")
	}
	if strings.TrimSpace(c.serviceKey) == "" {
		return nil, fmt.Errorf("holiday service key is empty")
	}

	reqURL, err := c.buildURL(kind, year, month)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("holiday api status=%d body=%q", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed apiResponse
	if err := xml.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("decode holiday xml: %w", err)
	}

	if code := strings.TrimSpace(parsed.Header.ResultCode); code != "" && code != "00" {
		return nil, fmt.Errorf("holiday api result=%s message=%s", code, strings.TrimSpace(parsed.Header.ResultMsg))
	}

	days := make([]model.SpecialDay, 0, len(parsed.Body.Items.Items))
	for _, item := range parsed.Body.Items.Items {
		day, err := toSpecialDay(kind, item)
		if err != nil {
			return nil, err
		}
		days = append(days, day)
	}

	return days, nil
}

func (c *Client) buildURL(kind SpecialDayKind, year int, month time.Month) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}

	u, err := url.Parse(base + "/" + kind.URLSuffix)
	if err != nil {
		return "", err
	}

	query := u.Query()
	query.Set("ServiceKey", normalizeServiceKey(c.serviceKey))
	query.Set("pageNo", "1")
	query.Set("numOfRows", "100")
	query.Set("solYear", strconv.Itoa(year))
	query.Set("solMonth", fmt.Sprintf("%02d", int(month)))
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func normalizeServiceKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	decoded, err := url.QueryUnescape(value)
	if err == nil && strings.TrimSpace(decoded) != "" {
		return decoded
	}
	return value
}

func toSpecialDay(kind SpecialDayKind, item apiItem) (model.SpecialDay, error) {
	day, err := parseLocDate(item.LocDate)
	if err != nil {
		return model.SpecialDay{}, err
	}

	seq, err := parseSeq(item.Seq)
	if err != nil {
		return model.SpecialDay{}, err
	}

	payload, err := json.Marshal(item)
	if err != nil {
		return model.SpecialDay{}, err
	}

	major, group := classifyMajorHoliday(item.DateName)
	return model.SpecialDay{
		Day:               day,
		Name:              strings.TrimSpace(item.DateName),
		KindCode:          kind.Code,
		KindLabel:         kind.Label,
		IsHoliday:         strings.EqualFold(strings.TrimSpace(item.IsHoliday), "Y"),
		Seq:               seq,
		IsMajorHoliday:    major,
		MajorHolidayGroup: group,
		SourcePayload:     payload,
		FetchedAt:         time.Now().UTC(),
	}, nil
}

func parseLocDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("holiday locdate is empty")
	}

	day, err := time.Parse("20060102", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse holiday locdate %q: %w", raw, err)
	}
	return day.UTC(), nil
}

func parseSeq(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse holiday seq %q: %w", raw, err)
	}
	return value, nil
}

func classifyMajorHoliday(name string) (bool, string) {
	trimmed := strings.TrimSpace(name)
	switch {
	case strings.Contains(trimmed, "설날"), strings.Contains(trimmed, "설연휴"):
		return true, "seollal"
	case strings.Contains(trimmed, "추석"), strings.Contains(trimmed, "추석연휴"):
		return true, "chuseok"
	default:
		return false, ""
	}
}
