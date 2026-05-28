package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

type Writer interface {
	WriteLogin(ctx context.Context, log LoginLog) error
	WriteOperation(ctx context.Context, log OperationLog) error
}

type VictoriaLogsWriter struct {
	baseURL string
	client  *http.Client
}

func NewVictoriaLogsWriter() *VictoriaLogsWriter {
	timeout := facade.Config.GetDuration("logs.timeout")
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &VictoriaLogsWriter{
		baseURL: strings.TrimRight(facade.Config.GetString("logs.base_url"), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (w *VictoriaLogsWriter) WriteLogin(ctx context.Context, log LoginLog) error {
	return w.writeJSONLine(ctx, log)
}

func (w *VictoriaLogsWriter) WriteOperation(ctx context.Context, log OperationLog) error {
	return w.writeJSONLine(ctx, log)
}

func (w *VictoriaLogsWriter) writeJSONLine(ctx context.Context, v any) error {
	if w.baseURL == "" {
		return fmt.Errorf("victorialogs base url is empty")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	body := append(data, '\n')
	url := w.baseURL + "/insert/jsonline?_stream_fields=audit_type,tenant,username,user_mode&_time_field=time&_msg_field=message"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/stream+json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("victorialogs write failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
