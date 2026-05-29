package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

type VictoriaLogsQuery struct {
	baseURL string
	client  *http.Client
}

func NewVictoriaLogsQuery() *VictoriaLogsQuery {
	timeout := facade.Config.GetDuration("logs.timeout")
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &VictoriaLogsQuery{
		baseURL: strings.TrimRight(facade.Config.GetString("logs.base_url"), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func CheckStatus(ctx context.Context) Status {
	baseURL := strings.TrimRight(facade.Config.GetString("logs.base_url"), "/")
	status := Status{
		Enabled: facade.Config.GetBool("logs.enabled"),
		BaseURL: baseURL,
	}
	if !status.Enabled {
		status.Message = "logs disabled"
		return status
	}
	if baseURL == "" {
		status.Message = "logs base_url empty"
		return status
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		status.Message = err.Error()
		return status
	}
	defer resp.Body.Close()
	status.Installed = resp.StatusCode < http.StatusMultipleChoices
	if !status.Installed {
		status.Message = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return status
}

func QueryLoginLogs(ctx context.Context, params QueryParams, current UserContext) (QueryResult, error) {
	return NewVictoriaLogsQuery().Query(ctx, TypeLogin, params, current)
}

func QueryOperationLogs(ctx context.Context, params QueryParams, current UserContext) (QueryResult, error) {
	return NewVictoriaLogsQuery().Query(ctx, TypeOperation, params, current)
}

func (q *VictoriaLogsQuery) Query(ctx context.Context, auditType string, params QueryParams, current UserContext) (QueryResult, error) {
	normalizeQueryParams(&params)
	query := buildLogsQL(auditType, params, current)
	offset := (params.Page - 1) * params.PageSize
	list, err := q.queryLogEntries(ctx, query, params.PageSize, offset)
	if err != nil {
		return QueryResult{}, err
	}
	sortLogsByTimeDesc(list)
	total, err := q.queryLogCount(ctx, query)
	if err != nil {
		return QueryResult{}, err
	}
	return QueryResult{
		List:     list,
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

func (q *VictoriaLogsQuery) queryLogEntries(ctx context.Context, query string, limit int, offset int) ([]map[string]any, error) {
	values := url.Values{}
	values.Set("query", query)
	values.Set("limit", strconv.Itoa(limit))
	values.Set("offset", strconv.Itoa(offset))
	endpoint := q.baseURL + "/select/logsql/query?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("victorialogs query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return decodeQueryBody(body), nil
}

func (q *VictoriaLogsQuery) queryLogCount(ctx context.Context, query string) (int, error) {
	values := url.Values{}
	values.Set("query", query+" | stats count() as total")
	endpoint := q.baseURL + "/select/logsql/query?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("victorialogs count query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return decodeTotalCount(body), nil
}

func normalizeQueryParams(params *QueryParams) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 200 {
		params.PageSize = 200
	}
}

func buildLogsQL(auditType string, params QueryParams, current UserContext) string {
	parts := []string{fmt.Sprintf(`audit_type:%q`, auditType)}
	if current.IsAdmin {
		if params.Tenant != "" {
			parts = append(parts, fmt.Sprintf(`tenant:%q`, params.Tenant))
		}
		if params.Username != "" {
			parts = append(parts, fmt.Sprintf(`username:%q`, params.Username))
		}
	} else {
		if current.Tenant != "" {
			parts = append(parts, fmt.Sprintf(`tenant:%q`, current.Tenant))
		}
		if current.Username != "" {
			parts = append(parts, fmt.Sprintf(`username:%q`, current.Username))
		}
	}
	if params.Success != "" {
		if params.Success == "true" || params.Success == "false" {
			parts = append(parts, "success:"+params.Success)
		} else {
			parts = append(parts, fmt.Sprintf(`success:%q`, params.Success))
		}
	}
	if params.Method != "" {
		parts = append(parts, fmt.Sprintf(`method:%q`, params.Method))
	}
	if params.Path != "" {
		parts = append(parts, fmt.Sprintf(`path:%q`, params.Path))
	}
	if params.StartTime != "" {
		parts = append(parts, fmt.Sprintf(`time:>=%q`, params.StartTime))
	}
	if params.EndTime != "" {
		parts = append(parts, fmt.Sprintf(`time:<=%q`, params.EndTime))
	}
	return strings.Join(parts, " ")
}

func decodeQueryBody(body []byte) []map[string]any {
	var array []map[string]any
	if err := json.Unmarshal(body, &array); err == nil {
		return array
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err == nil {
		if data, ok := object["data"].([]any); ok {
			return anyArrayToMapArray(data)
		}
		if data, ok := object["result"].([]any); ok {
			return anyArrayToMapArray(data)
		}
		return []map[string]any{object}
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	result := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			result = append(result, item)
		}
	}
	return result
}

func anyArrayToMapArray(data []any) []map[string]any {
	result := make([]map[string]any, 0, len(data))
	for _, item := range data {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

func decodeTotalCount(body []byte) int {
	for _, item := range decodeQueryBody(body) {
		for _, key := range []string{"total", "logs_total", "count"} {
			if total, ok := intFromAny(item[key]); ok {
				return total
			}
		}
	}
	return 0
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	case string:
		n, err := strconv.Atoi(v)
		return n, err == nil
	default:
		return 0, false
	}
}

func sortLogsByTimeDesc(list []map[string]any) {
	sort.SliceStable(list, func(i, j int) bool {
		return logTime(list[i]).After(logTime(list[j]))
	})
}

func logTime(item map[string]any) time.Time {
	for _, key := range []string{"time", "_time"} {
		if value, ok := item[key]; ok {
			if t, ok := parseLogTime(value); ok {
				return t
			}
		}
	}
	return time.Time{}
}

func parseLogTime(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case string:
		t, err := time.Parse(time.RFC3339Nano, v)
		if err == nil {
			return t, true
		}
		t, err = time.Parse(time.RFC3339, v)
		return t, err == nil
	default:
		return time.Time{}, false
	}
}
