package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

const maxRange = 30 * 24 * time.Hour

type TimeRange struct {
	Start time.Time
	End   time.Time
}

type QueryParams struct {
	Namespace string
	Domain    string
	Method    string
	Status    string
	Keyword   string
	Sort      string
	Page      int
	PageSize  int
	Step      string
	Range     TimeRange
}

type QueryClient struct {
	baseURL string
	client  *http.Client
}

func NewQueryClient() *QueryClient {
	timeout := facade.Config.GetDuration("logs.timeout")
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &QueryClient{
		baseURL: strings.TrimRight(facade.Config.GetString("logs.base_url"), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func ParseTimeRange(start, end string, now time.Time) (TimeRange, error) {
	endTime := now
	if strings.TrimSpace(end) != "" {
		parsed, err := parseTime(end)
		if err != nil {
			return TimeRange{}, fmt.Errorf("end时间格式错误")
		}
		endTime = parsed
	}
	startTime := endTime.Add(-time.Hour)
	if strings.TrimSpace(start) != "" {
		parsed, err := parseTime(start)
		if err != nil {
			return TimeRange{}, fmt.Errorf("start时间格式错误")
		}
		startTime = parsed
	}
	if !startTime.Before(endTime) {
		return TimeRange{}, fmt.Errorf("start必须早于end")
	}
	if endTime.Sub(startTime) > maxRange {
		return TimeRange{}, fmt.Errorf("查询时间范围不能超过30天")
	}
	return TimeRange{Start: startTime, End: endTime}, nil
}

func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if number, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(number, 0), nil
	}
	return time.Parse(time.RFC3339, value)
}

func NormalizeParams(params *QueryParams) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	if params.Step == "" {
		params.Step = defaultStep(params.Range.End.Sub(params.Range.Start))
	}
}

func defaultStep(duration time.Duration) string {
	switch {
	case duration <= 6*time.Hour:
		return "5m"
	case duration <= 48*time.Hour:
		return "30m"
	case duration <= 7*24*time.Hour:
		return "2h"
	default:
		return "12h"
	}
}

func (q *QueryClient) Health(ctx context.Context) error {
	if q.baseURL == "" {
		return fmt.Errorf("VictoriaLogs地址未配置")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("VictoriaLogs状态码%d", resp.StatusCode)
	}
	return nil
}

func (q *QueryClient) Summary(ctx context.Context, params QueryParams) ([]map[string]any, error) {
	query := buildFilter(params) + ` | stats by (status_code) count() as requests, sum(bytes_received) as bytes_received, sum(bytes_sent) as bytes_sent, avg(duration_ms) as avg_duration_ms, quantile(0.95, duration_ms) as p95_duration_ms`
	return q.query(ctx, query, params.Range)
}

func (q *QueryClient) Pods(ctx context.Context, params QueryParams) ([]map[string]any, error) {
	query := buildFilter(params) + ` upstream_ip:!"" | stats by (upstream_ip, upstream_service, upstream_namespace, status_code) count() as requests, sum(bytes_received) as bytes_received, sum(bytes_sent) as bytes_sent, avg(duration_ms) as avg_duration_ms, quantile(0.95, duration_ms) as p95_duration_ms | sort by (requests desc) | limit 1000`
	return q.query(ctx, query, params.Range)
}

func (q *QueryClient) Domains(ctx context.Context, params QueryParams) ([]map[string]any, error) {
	query := buildFilter(params) + ` | stats by (authority, status_code) count() as requests, sum(bytes_received) as bytes_received, sum(bytes_sent) as bytes_sent, avg(duration_ms) as avg_duration_ms, quantile(0.95, duration_ms) as p95_duration_ms | sort by (requests desc) | limit 1000`
	return q.query(ctx, query, params.Range)
}

func (q *QueryClient) URLs(ctx context.Context, params QueryParams) ([]map[string]any, error) {
	query := buildFilter(params) + ` | stats by (authority, method, path, status_code) count() as requests, sum(bytes_received) as bytes_received, sum(bytes_sent) as bytes_sent, avg(duration_ms) as avg_duration_ms, quantile(0.95, duration_ms) as p95_duration_ms | sort by (requests desc) | limit 2000`
	return q.query(ctx, query, params.Range)
}

func (q *QueryClient) Series(ctx context.Context, params QueryParams, dimension string) ([]map[string]any, error) {
	group := "authority"
	if dimension == "pod" {
		group = "upstream_ip"
	}
	step := sanitizeStep(params.Step)
	query := fmt.Sprintf(`%s | stats by (_time:%s, %s, status_code) count() as requests, sum(bytes_received) as bytes_received, sum(bytes_sent) as bytes_sent`, buildFilter(params), step, group)
	return q.query(ctx, query, params.Range)
}

func sanitizeStep(step string) string {
	allowed := map[string]bool{"1m": true, "5m": true, "15m": true, "30m": true, "1h": true, "2h": true, "6h": true, "12h": true, "1d": true}
	if allowed[step] {
		return step
	}
	return "5m"
}

func buildFilter(params QueryParams) string {
	parts := []string{`log_type:"higress_access"`}
	if params.Namespace != "*" {
		parts = append(parts, fmt.Sprintf(`route_namespace:%q`, params.Namespace))
	}
	if params.Domain != "" {
		parts = append(parts, fmt.Sprintf(`authority:%q`, params.Domain))
	}
	if params.Method != "" {
		parts = append(parts, fmt.Sprintf(`method:%q`, strings.ToUpper(params.Method)))
	}
	if params.Status != "" {
		status := strings.ToLower(params.Status)
		if len(status) == 3 && status[1:] == "xx" && status[0] >= '1' && status[0] <= '5' {
			parts = append(parts, fmt.Sprintf(`status_code:%c*`, status[0]))
		} else {
			parts = append(parts, fmt.Sprintf(`status_code:%q`, params.Status))
		}
	}
	if params.Keyword != "" {
		parts = append(parts, fmt.Sprintf(`path:~%q`, ".*"+regexp.QuoteMeta(params.Keyword)+".*"))
	}
	return strings.Join(parts, " ")
}

func (q *QueryClient) query(ctx context.Context, query string, timerange TimeRange) ([]map[string]any, error) {
	if q.baseURL == "" {
		return nil, fmt.Errorf("VictoriaLogs地址未配置")
	}
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", timerange.Start.UTC().Format(time.RFC3339))
	values.Set("end", timerange.End.UTC().Format(time.RFC3339))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/select/logsql/query?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("VictoriaLogs查询失败: %s", strings.TrimSpace(string(body)))
	}
	return decodeRows(body), nil
}

func decodeRows(body []byte) []map[string]any {
	var array []map[string]any
	if json.Unmarshal(body, &array) == nil {
		return array
	}
	rows := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) == nil {
			rows = append(rows, row)
		}
	}
	return rows
}

func FoldRows(rows []map[string]any, keys ...string) []map[string]any {
	groups := map[string]map[string]any{}
	for _, row := range rows {
		values := make([]string, len(keys))
		for i, key := range keys {
			values[i] = fmt.Sprint(row[key])
		}
		id := strings.Join(values, "\x00")
		item := groups[id]
		if item == nil {
			item = map[string]any{}
			for _, key := range keys {
				item[key] = row[key]
			}
			groups[id] = item
		}
		requests := number(row["requests"])
		item["requests"] = number(item["requests"]) + requests
		item["bytes_received"] = number(item["bytes_received"]) + number(row["bytes_received"])
		item["bytes_sent"] = number(item["bytes_sent"]) + number(row["bytes_sent"])
		status, _ := strconv.Atoi(fmt.Sprint(row["status_code"]))
		if status >= 400 {
			item["errors"] = number(item["errors"]) + requests
		}
		item["duration_weight"] = number(item["duration_weight"]) + number(row["avg_duration_ms"])*requests
		item["p95_duration_ms"] = max(number(item["p95_duration_ms"]), number(row["p95_duration_ms"]))
	}
	result := make([]map[string]any, 0, len(groups))
	for _, item := range groups {
		requests := number(item["requests"])
		if requests > 0 {
			item["error_rate"] = number(item["errors"]) / requests
			item["avg_duration_ms"] = number(item["duration_weight"]) / requests
		}
		delete(item, "duration_weight")
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return number(result[i]["requests"]) > number(result[j]["requests"]) })
	return result
}

func Paginate(rows []map[string]any, page, pageSize int) ([]map[string]any, int) {
	total := len(rows)
	start := (page - 1) * pageSize
	if start >= total {
		return []map[string]any{}, total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return rows[start:end], total
}

func SortRows(rows []map[string]any, field string) {
	key := "requests"
	switch field {
	case "traffic":
		key = "traffic"
		for _, row := range rows {
			row[key] = number(row["bytes_received"]) + number(row["bytes_sent"])
		}
	case "errors":
		key = "errors"
	case "latency":
		key = "p95_duration_ms"
	}
	sort.SliceStable(rows, func(i, j int) bool { return number(rows[i][key]) > number(rows[j][key]) })
}

func number(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		n, _ := v.Float64()
		return n
	default:
		n, _ := strconv.ParseFloat(fmt.Sprint(value), 64)
		return n
	}
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
