package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	values := url.Values{}
	values.Set("query", query)
	values.Set("limit", strconv.Itoa(params.PageSize))
	values.Set("offset", strconv.Itoa((params.Page-1)*params.PageSize))
	endpoint := q.baseURL + "/select/logsql/query?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return QueryResult{}, err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return QueryResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return QueryResult{}, err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return QueryResult{}, fmt.Errorf("victorialogs query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	list := decodeQueryBody(body)
	return QueryResult{
		List:     list,
		Total:    len(list),
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
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
