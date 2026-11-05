package audit

import "time"

const (
	TypeLogin     = "login"
	TypeOperation = "operation"
)

type UserContext struct {
	Tenant       string
	Username     string
	UserMode     string
	K3kName      string
	K3kNamespace string
	IsAdmin      bool
}

type LoginLog struct {
	Time        time.Time `json:"time"`
	AuditType   string    `json:"audit_type"`
	Tenant      string    `json:"tenant"`
	Username    string    `json:"username"`
	UserMode    string    `json:"user_mode"`
	LoginMethod string    `json:"login_method"`
	Success     bool      `json:"success"`
	Reason      string    `json:"reason,omitempty"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	Message     string    `json:"message"`
}

type OperationLog struct {
	Time       time.Time `json:"time"`
	AuditType  string    `json:"audit_type"`
	Tenant     string    `json:"tenant"`
	Username   string    `json:"username"`
	UserMode   string    `json:"user_mode"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Route      string    `json:"route"`
	Params     string    `json:"params,omitempty"`
	StatusCode int       `json:"status_code"`
	Success    bool      `json:"success"`
	DurationMs int64     `json:"duration_ms"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
}

type QueryParams struct {
	Page      int
	PageSize  int
	Username  string
	Tenant    string
	Success   string
	Method    string
	Path      string
	StartTime string
	EndTime   string
}

type QueryResult struct {
	List     any `json:"list"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

type Status struct {
	Enabled   bool   `json:"enabled"`
	Installed bool   `json:"installed"`
	BaseURL   string `json:"baseUrl"`
	Message   string `json:"message,omitempty"`
}
