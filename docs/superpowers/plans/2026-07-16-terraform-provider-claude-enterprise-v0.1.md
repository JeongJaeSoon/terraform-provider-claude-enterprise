# terraform-provider-claude-enterprise v0.1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Claude Enterprise Admin API(Spend Limits)를 감싸는 Terraform provider — `claude-enterprise_spend_limit` 리소스와 `claude-enterprise_members` 데이터소스, mock 기반 acceptance test, CI/릴리스 파이프라인까지.

**Architecture:** Go + terraform-plugin-framework. 얇은 net/http 클라이언트(`internal/client`)가 인증 헤더·429 재시도·rate limit·cursor 페이지네이션을 담당하고, `internal/provider`가 리소스/데이터소스를 구현한다. email→user_id 해소는 plan 시점(ModifyPlan)에 provider 레벨 캐시로 수행. 테스트는 `httptest` mock Admin API 서버로 실제 key 없이 전체 라이프사이클 검증.

**Tech Stack:** Go 1.26, terraform-plugin-framework, terraform-plugin-framework-validators, terraform-plugin-testing, golang.org/x/time/rate, GoReleaser, GitHub Actions

**Spec:** `docs/superpowers/specs/2026-07-16-terraform-provider-claude-enterprise-design.md` (승인·보정 완료본)

## Global Constraints

- Module path: `github.com/JeongJaeSoon/terraform-provider-claude-enterprise`
- Provider address: `registry.terraform.io/JeongJaeSoon/claude-enterprise`, provider type name: `claude-enterprise`
- Go 1.26 (`go.mod`의 `go 1.26`), CGO_ENABLED=0
- API base: `https://api.anthropic.com`, 헤더 `x-api-key` + `anthropic-version: 2023-06-01`
- 금액은 항상 string(cents). 부동소수점 금지. `amount`는 nullable(`*string`) — null = 무제한
- `source.type` / `period` / `scope.type`은 open set — 모르는 값에 에러 내지 않는다
- 커밋 메시지·코드 주석·문서(README, docs/)는 영어. 커밋은 각 Task 완료 시점마다
- 커밋 trailer: `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- 라이선스: MPL-2.0
- acceptance test는 `TF_ACC=1` + 로컬 terraform CLI 필요 (Task 1에서 확인)
- 모든 명령은 리포 루트 `/Users/dev-soon/workspace/project/terraform-provider-claude-enterprise`에서 실행

## API 응답 형태 (실 레퍼런스에서 확인, mock·클라이언트가 따라야 하는 계약)

`GET /v1/organizations/spend_limits/effective?limit=20` (cursor: `next_page` 값을 `page` 파라미터로; `next_page: null`이면 끝; `user_ids[]` 필터 지원):

```json
{
  "data": [
    {
      "scope": { "type": "user", "user_id": "user_01AbCdEfGh" },
      "actor": {
        "type": "user_actor",
        "user_id": "user_01AbCdEfGh",
        "name": "Jane Smith",
        "email_address": "jane@example.com",
        "deleted": false
      },
      "amount": "50000",
      "currency": "USD",
      "period": "monthly",
      "source": { "type": "seat_tier", "seat_tier": "enterprise_standard" },
      "spend_limit_id": "spl_01XyZaBcDeFgHiJkLmNoPq",
      "period_to_date_spend": "31402.5"
    }
  ],
  "next_page": "page_..."
}
```

`POST /v1/organizations/spend_limits` (upsert, 키 = `(scope, period)`, `scope.type: "user"`만 허용) — 요청 `{"scope": {"type": "user", "user_id": "..."}, "amount": "75000", "period": "monthly"}`, 응답:

```json
{
  "type": "spend_limit",
  "id": "spl_01RsTuVwXyZaBcDeFgHiJk",
  "created_at": "2026-05-11T10:02:44Z",
  "updated_at": "2026-05-11T10:02:44Z",
  "scope": { "type": "user", "user_id": "user_01AbCdEfGh" },
  "amount": "75000",
  "currency": "USD",
  "period": "monthly"
}
```

`GET /v1/organizations/spend_limits/{id}` → 위 spend_limit 객체와 동일 형태 (`source`/`period_to_date_spend` 없음). `DELETE /v1/organizations/spend_limits/{id}` → 성공 시 2xx.

에러 응답 (모든 4xx/5xx): `{"type": "error", "error": {"type": "not_found_error", "message": "..."}, "request_id": "req_..."}`. 레이트리밋: 조직당 60 req/min, 초과 시 429 (+`Retry-After` 헤더 가능).

## File Structure (최종 상태)

```
terraform-provider-claude-enterprise/
├── main.go
├── go.mod / go.sum
├── Makefile
├── .gitignore
├── LICENSE                                  # MPL-2.0
├── README.md
├── terraform-registry-manifest.json
├── .goreleaser.yml
├── .github/workflows/ci.yml
├── .github/workflows/release.yml
├── internal/client/
│   ├── client.go          # Client, Option, do(), APIError, IsNotFound
│   ├── retry.go           # 429/5xx 재시도, Retry-After, 지수백오프
│   ├── spend_limits.go    # 타입 + List/Get/Upsert/Delete + 페이지네이션
│   ├── client_test.go
│   ├── retry_test.go
│   └── spend_limits_test.go
├── internal/provider/
│   ├── provider.go            # provider 스키마·Configure·providerData
│   ├── resolver.go            # email→user_id 캐시
│   ├── resource_spend_limit.go
│   ├── data_source_members.go
│   ├── provider_test.go       # 팩토리 + Configure 에러 테스트
│   ├── resolver_test.go
│   ├── resource_spend_limit_test.go
│   ├── data_source_members_test.go
│   └── testutil/mockserver.go # httptest mock Admin API
├── examples/
│   ├── provider/provider.tf
│   ├── resources/claude-enterprise_spend_limit/resource.tf
│   ├── data-sources/claude-enterprise_members/data-source.tf
│   └── full/main.tf           # 실환경 검증용
└── docs/
    ├── index.md               # Registry provider 문서
    ├── resources/spend_limit.md
    ├── data-sources/members.md
    └── RELEASING.md
```

**의존 관계:** Task 1 → 2 → 3 → 4 → 5 → 6 → (7, 8) → 9 → 10 → 11 → (12, 13). 7과 8은 서로 독립, 12와 13도 서로 독립.

---

### Task 1: 프로젝트 스캐폴드 (go.mod, main.go, provider 골격, Makefile)

**Files:**
- Create: `go.mod` (go mod init으로), `main.go`, `internal/provider/provider.go`, `internal/provider/provider_test.go`, `Makefile`, `.gitignore`

**Interfaces:**
- Produces: `provider.New(version string) func() provider.Provider` — 이후 모든 Task가 사용. provider TypeName `claude-enterprise`. `providerModel{AdminAPIKey, BaseURL types.String}` (tfsdk: `admin_api_key`, `base_url`).

- [ ] **Step 1: 전제 도구 확인**

Run: `go version && terraform version && git status --porcelain`
Expected: go 1.26+, Terraform v1.8+ 출력. terraform이 없으면 `brew install hashicorp/tap/terraform` 후 재확인.

- [ ] **Step 2: Go 모듈 초기화 + 의존성 추가**

```bash
go mod init github.com/JeongJaeSoon/terraform-provider-claude-enterprise
go get github.com/hashicorp/terraform-plugin-framework@latest
go get github.com/hashicorp/terraform-plugin-framework-validators@latest
go get github.com/hashicorp/terraform-plugin-go@latest
go get github.com/hashicorp/terraform-plugin-testing@latest
go get golang.org/x/time@latest
```

- [ ] **Step 3: `.gitignore` 작성**

```gitignore
# Binaries and release artifacts
terraform-provider-claude-enterprise
dist/

# Terraform working files (examples)
**/.terraform/
**/.terraform.lock.hcl
*.tfstate
*.tfstate.backup
crash.log

# OS / editor
.DS_Store
```

- [ ] **Step 4: 실패하는 테스트 작성 — `internal/provider/provider_test.go`**

```go
package provider_test

import (
	"context"
	"testing"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider"
)

func TestProviderMetadata(t *testing.T) {
	p := provider.New("test")()

	resp := &fwprovider.MetadataResponse{}
	p.Metadata(context.Background(), fwprovider.MetadataRequest{}, resp)

	if resp.TypeName != "claude-enterprise" {
		t.Fatalf("expected type name %q, got %q", "claude-enterprise", resp.TypeName)
	}
	if resp.Version != "test" {
		t.Fatalf("expected version %q, got %q", "test", resp.Version)
	}
}

func TestProviderSchemaValid(t *testing.T) {
	p := provider.New("test")()

	resp := &fwprovider.SchemaResponse{}
	p.Schema(context.Background(), fwprovider.SchemaRequest{}, resp)

	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("provider schema invalid: %v", diags)
	}
	for _, name := range []string{"admin_api_key", "base_url"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("expected schema attribute %q", name)
		}
	}
}
```

주의: `ValidateImplementation`이 현재 framework 버전에 없으면 그 호출 한 줄은 제거하고 속성 존재 확인만 남긴다.

- [ ] **Step 5: 테스트 실패 확인**

Run: `go test ./internal/provider/ -run TestProvider -v`
Expected: FAIL (package provider가 없어 컴파일 에러)

- [ ] **Step 6: `internal/provider/provider.go` 구현**

```go
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// claudeEnterpriseProvider implements a Terraform provider for the Claude
// Enterprise Admin API (spend limits).
type claudeEnterpriseProvider struct {
	version string
}

type providerModel struct {
	AdminAPIKey types.String `tfsdk:"admin_api_key"`
	BaseURL     types.String `tfsdk:"base_url"`
}

// New returns a provider factory. version is set by goreleaser at build time.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &claudeEnterpriseProvider{version: version}
	}
}

func (p *claudeEnterpriseProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "claude-enterprise"
	resp.Version = p.version
}

func (p *claudeEnterpriseProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Claude Enterprise per-user spend limits via the Admin API. " +
			"This is a community provider, not an official Anthropic product.",
		Attributes: map[string]schema.Attribute{
			"admin_api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "Scoped Admin API key (`sk-ant-admin...`) with `read:spend_limits` " +
					"and `write:spend_limits` scopes. Defaults to the `ANTHROPIC_ADMIN_KEY` environment " +
					"variable. Prefer the environment variable so the key never lands in configuration or state.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "API base URL. Defaults to `https://api.anthropic.com`. Override for testing or proxies.",
			},
		},
	}
}

// Configure is completed in Task 6; for now it does nothing so the provider compiles.
func (p *claudeEnterpriseProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

func (p *claudeEnterpriseProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *claudeEnterpriseProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
```

- [ ] **Step 7: `main.go` 구현**

```go
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider"
)

// version is set by goreleaser via ldflags at release time.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/JeongJaeSoon/claude-enterprise",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 8: `Makefile` 작성**

```makefile
default: build

build:
	go build ./...

install:
	go install .

fmt:
	gofmt -w .

test:
	go test ./... -count=1

testacc:
	TF_ACC=1 go test ./internal/provider/ -v -count=1 -timeout 10m
```

- [ ] **Step 9: 빌드·테스트 통과 확인**

Run: `go mod tidy && go build ./... && go test ./internal/provider/ -run TestProvider -v`
Expected: PASS 2건

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat: scaffold provider skeleton with framework plugin server"
```

---

### Task 2: API 클라이언트 코어 (인증 헤더, 에러 매핑)

**Files:**
- Create: `internal/client/client.go`, `internal/client/client_test.go`

**Interfaces:**
- Produces:
  - `client.New(baseURL, apiKey string, opts ...Option) (*Client, error)`
  - `Option`: `WithHTTPClient(*http.Client)`, `WithMaxRetries(int)`, `WithRequestsPerMinute(int)`
  - `(c *Client) do(ctx, method, path string, query url.Values, body, out any) error` (비공개, Task 4가 사용)
  - `APIError{StatusCode int; Type, Message, RequestID string}` + `IsNotFound(error) bool`
  - `DefaultBaseURL = "https://api.anthropic.com"`

- [ ] **Step 1: 실패하는 테스트 작성 — `internal/client/client_test.go`**

```go
package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDoSendsAuthHeaders(t *testing.T) {
	var gotAPIKey, gotVersion, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotContentType = r.Header.Get("content-type")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "sk-ant-admin-test")
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.do(context.Background(), http.MethodPost, "/v1/test", nil, map[string]string{"a": "b"}, &out); err != nil {
		t.Fatal(err)
	}
	if gotAPIKey != "sk-ant-admin-test" {
		t.Fatalf("x-api-key = %q", gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", gotVersion)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q", gotContentType)
	}
	if !out.OK {
		t.Fatal("response not decoded")
	}
}

func TestDoQueryParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	q := url.Values{}
	q.Set("limit", "100")
	q.Add("user_ids[]", "user_01A")
	q.Add("user_ids[]", "user_01B")
	if err := c.do(context.Background(), http.MethodGet, "/v1/test", q, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotQuery.Get("limit") != "100" {
		t.Fatalf("limit = %q", gotQuery.Get("limit"))
	}
	if ids := gotQuery["user_ids[]"]; len(ids) != 2 || ids[0] != "user_01A" || ids[1] != "user_01B" {
		t.Fatalf("user_ids[] = %v", ids)
	}
}

func TestDoAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"not_found_error","message":"spend limit not found"},"request_id":"req_123"}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	err := c.do(context.Background(), http.MethodGet, "/v1/missing", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 || apiErr.Type != "not_found_error" || apiErr.RequestID != "req_123" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
	if !IsNotFound(err) {
		t.Fatal("IsNotFound should be true")
	}
	if IsNotFound(errors.New("plain")) {
		t.Fatal("IsNotFound(plain) should be false")
	}
}

func TestDoNonJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`upstream exploded`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", WithMaxRetries(0))
	err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 502 {
		t.Fatalf("expected 502 APIError, got %v", err)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New("://bad url", "k"); err == nil {
		t.Fatal("expected error for invalid base URL")
	}
	if _, err := New(DefaultBaseURL, ""); err == nil {
		t.Fatal("expected error for empty api key")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/client/ -v`
Expected: FAIL (컴파일 에러 — client.go 미구현)

- [ ] **Step 3: `internal/client/client.go` 구현**

```go
// Package client is a minimal HTTP client for the Claude Enterprise Admin
// API (spend limits endpoints).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultBaseURL is the production Admin API endpoint.
	DefaultBaseURL = "https://api.anthropic.com"

	anthropicVersion = "2023-06-01"

	defaultMaxRetries = 5
	// The API allows 60 requests/minute per organization; stay under it by
	// default so Terraform's parallelism doesn't trip the limit.
	defaultRequestsPerMinute = 50
)

// Client talks to the Claude Enterprise Admin API.
type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
	limiter    *rate.Limiter
	maxRetries int
	// sleep is indirected so retry tests don't wait in real time.
	sleep func(context.Context, time.Duration) error
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithMaxRetries sets how many times a 429/5xx response is retried.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithRequestsPerMinute adjusts the client-side rate limit.
func WithRequestsPerMinute(n int) Option {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Limit(float64(n)/60.0), 1)
	}
}

// New builds a Client. apiKey is required; baseURL falls back to
// DefaultBaseURL when empty.
func New(baseURL, apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("admin API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q", baseURL)
	}
	c := &Client{
		baseURL:    u,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(defaultRequestsPerMinute/60.0), 1),
		maxRetries: defaultMaxRetries,
		sleep:      sleepCtx,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// APIError is a non-2xx response from the Admin API.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
	RequestID  string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	s := fmt.Sprintf("claude enterprise admin API error (HTTP %d, %s): %s", e.StatusCode, e.Type, msg)
	if e.RequestID != "" {
		s += fmt.Sprintf(" (request_id: %s)", e.RequestID)
	}
	return s
}

// IsNotFound reports whether err is an APIError with HTTP 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// do executes one API call with auth headers, rate limiting, retries on
// 429/5xx, and JSON (de)serialization.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}

	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}

		var reqBody io.Reader
		if payload != nil {
			reqBody = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
		if err != nil {
			return err
		}
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", anthropicVersion)
		if payload != nil {
			req.Header.Set("content-type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}

		if shouldRetry(resp.StatusCode) && attempt < c.maxRetries {
			if err := c.sleep(ctx, retryDelay(resp, attempt)); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode >= 400 {
			return parseAPIError(resp.StatusCode, respBody)
		}
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decoding response: %w", err)
			}
		}
		return nil
	}
}

func parseAPIError(status int, body []byte) error {
	apiErr := &APIError{StatusCode: status}
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		apiErr.Type = envelope.Error.Type
		apiErr.Message = envelope.Error.Message
		apiErr.RequestID = envelope.RequestID
	}
	if apiErr.Message == "" {
		apiErr.Message = strings.TrimSpace(string(body))
	}
	return apiErr
}
```

`shouldRetry`/`retryDelay`는 Task 3에서 구현하므로, 이 Task를 컴파일시키기 위해 임시 스텁을 같은 파일 하단에 둔다 (Task 3에서 `retry.go`로 이동):

```go
// Temporary stubs; replaced by retry.go in the next task.
func shouldRetry(status int) bool { return status == http.StatusTooManyRequests || status >= 500 }

func retryDelay(_ *http.Response, _ int) time.Duration { return 0 }
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/client/ -v`
Expected: PASS 5건 (TestDoNonJSONError는 `WithMaxRetries(0)`이라 즉시 에러)

- [ ] **Step 5: Commit**

```bash
git add internal/client/
git commit -m "feat: add admin API client core with auth headers and error mapping"
```

---

### Task 3: 재시도·레이트리밋 (`retry.go`)

**Files:**
- Create: `internal/client/retry.go`, `internal/client/retry_test.go`
- Modify: `internal/client/client.go` (하단의 임시 스텁 `shouldRetry`/`retryDelay` 삭제)

**Interfaces:**
- Consumes: `Client.do()`가 `shouldRetry(status int) bool`, `retryDelay(resp *http.Response, attempt int) time.Duration` 호출 (Task 2에서 연결 완료)
- Produces: 위 두 함수의 최종 구현. 정책 — 429와 5xx만 재시도, `Retry-After`(초) 헤더 우선, 없으면 `1s * 2^attempt` (상한 30s)

- [ ] **Step 1: 실패하는 테스트 작성 — `internal/client/retry_test.go`**

```go
package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns a client whose sleep records delays instead of waiting.
func newTestClient(t *testing.T, baseURL string, opts ...Option) (*Client, *[]time.Duration) {
	t.Helper()
	c, err := New(baseURL, "k", opts...)
	if err != nil {
		t.Fatal(err)
	}
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	return c, &slept
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, slept := newTestClient(t, srv.URL)
	if err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
	if len(*slept) != 2 || (*slept)[0] != 3*time.Second || (*slept)[1] != 3*time.Second {
		t.Fatalf("expected two 3s Retry-After sleeps, got %v", *slept)
	}
}

func TestRetryExponentialBackoffWithoutHeader(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, slept := newTestClient(t, srv.URL)
	if err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	if len(*slept) != len(want) {
		t.Fatalf("expected %d sleeps, got %v", len(want), *slept)
	}
	for i := range want {
		if (*slept)[i] != want[i] {
			t.Fatalf("sleep[%d] = %v, want %v", i, (*slept)[i], want[i])
		}
	}
}

func TestRetryExhaustionReturnsAPIError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"nope"}}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv.URL, WithMaxRetries(2))
	err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 APIError, got %v", err)
	}
	if calls.Load() != 3 { // initial + 2 retries
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv.URL)
	if err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("400 must not retry; got %d calls", calls.Load())
	}
}

func TestRetryDelayCap(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	if d := retryDelay(resp, 10); d != 30*time.Second {
		t.Fatalf("expected 30s cap, got %v", d)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/client/ -run TestRetry -v`
Expected: FAIL — 스텁 `retryDelay`가 0을 반환하므로 sleep 시간 검증(3s, 1s/2s/4s, 30s cap)이 실패

- [ ] **Step 3: `internal/client/retry.go` 구현 + client.go 스텁 삭제**

```go
package client

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	backoffBase = 1 * time.Second
	backoffCap  = 30 * time.Second
)

// shouldRetry reports whether the HTTP status is worth retrying: rate
// limits (429) and transient server errors (5xx).
func shouldRetry(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// retryDelay honors a Retry-After header (in seconds) when present and
// otherwise falls back to capped exponential backoff.
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	d := time.Duration(float64(backoffBase) * math.Pow(2, float64(attempt)))
	if d > backoffCap || d <= 0 {
		return backoffCap
	}
	return d
}
```

`client.go` 하단의 `// Temporary stubs` 블록(두 함수)을 삭제한다.

- [ ] **Step 4: 전체 클라이언트 테스트 통과 확인**

Run: `go test ./internal/client/ -v`
Expected: PASS (Task 2 + Task 3 테스트 전부)

- [ ] **Step 5: Commit**

```bash
git add internal/client/
git commit -m "feat: add retry with Retry-After support and capped backoff"
```

---

### Task 4: Spend Limits API 메서드 + 페이지네이션

**Files:**
- Create: `internal/client/spend_limits.go`, `internal/client/spend_limits_test.go`

**Interfaces:**
- Consumes: `Client.do()` (Task 2)
- Produces (이후 provider 계층 전체가 사용):
  - `Scope{Type, UserID string}`, `Source{Type, SeatTier string}`, `Actor{Type, UserID, Name, EmailAddress string; Deleted bool}`
  - `SpendLimit{Type, ID, CreatedAt, UpdatedAt string; Scope Scope; Amount *string; Currency, Period string}`
  - `EffectiveSpendLimit{Scope Scope; Actor Actor; Amount *string; Currency, Period string; Source Source; SpendLimitID, PeriodToDateSpend string}`
  - `(c *Client) ListEffectiveSpendLimits(ctx context.Context, userIDs []string) ([]EffectiveSpendLimit, error)` — nil이면 전체 순회, userIDs는 `user_ids[]` 필터
  - `(c *Client) GetSpendLimit(ctx context.Context, id string) (*SpendLimit, error)`
  - `(c *Client) UpsertSpendLimit(ctx context.Context, userID, amount, period string) (*SpendLimit, error)`
  - `(c *Client) DeleteSpendLimit(ctx context.Context, id string) error`

- [ ] **Step 1: 실패하는 테스트 작성 — `internal/client/spend_limits_test.go`**

```go
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListEffectiveSpendLimitsPaginates(t *testing.T) {
	// Three members served two per page; the cursor from next_page must come
	// back as the page parameter with other parameters unchanged.
	pages := map[string]string{
		"": `{"data":[
			{"scope":{"type":"user","user_id":"user_01A"},
			 "actor":{"type":"user_actor","user_id":"user_01A","name":"A","email_address":"a@example.com","deleted":false},
			 "amount":"50000","currency":"USD","period":"monthly",
			 "source":{"type":"user"},"spend_limit_id":"spl_01A","period_to_date_spend":"100"},
			{"scope":{"type":"user","user_id":"user_01B"},
			 "actor":{"type":"user_actor","user_id":"user_01B","name":"B","email_address":"b@example.com","deleted":false},
			 "amount":null,"currency":"USD","period":"monthly",
			 "source":{"type":"seat_tier","seat_tier":"enterprise_standard"},"spend_limit_id":"spl_01T","period_to_date_spend":"0"}
		],"next_page":"page_2"}`,
		"page_2": `{"data":[
			{"scope":{"type":"user","user_id":"user_01C"},
			 "actor":{"type":"user_actor","user_id":"user_01C","name":"C","email_address":"c@example.com","deleted":true},
			 "amount":"0","currency":"USD","period":"monthly",
			 "source":{"type":"organization"},"spend_limit_id":"spl_01O","period_to_date_spend":"41280.125"}
		],"next_page":null}`,
	}
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/organizations/spend_limits/effective" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "100" {
			t.Errorf("limit must stay constant across pages, got %q", r.URL.Query().Get("limit"))
		}
		cursor := r.URL.Query().Get("page")
		cursors = append(cursors, cursor)
		body, ok := pages[cursor]
		if !ok {
			t.Errorf("unexpected cursor %q", cursor)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	rows, err := c.ListEffectiveSpendLimits(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if len(cursors) != 2 || cursors[0] != "" || cursors[1] != "page_2" {
		t.Fatalf("unexpected cursor sequence %v", cursors)
	}
	if rows[0].Actor.EmailAddress != "a@example.com" || *rows[0].Amount != "50000" || rows[0].Source.Type != "user" {
		t.Fatalf("row 0 decoded wrong: %+v", rows[0])
	}
	if rows[1].Amount != nil {
		t.Fatalf("row 1 amount should be nil (unlimited), got %v", *rows[1].Amount)
	}
	if !rows[2].Actor.Deleted || rows[2].PeriodToDateSpend != "41280.125" {
		t.Fatalf("row 2 decoded wrong: %+v", rows[2])
	}
}

func TestListEffectiveSpendLimitsUserIDsFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := r.URL.Query()["user_ids[]"]
		if len(ids) != 2 || ids[0] != "user_01A" || ids[1] != "user_01B" {
			t.Errorf("user_ids[] = %v", ids)
		}
		_, _ = w.Write([]byte(`{"data":[],"next_page":null}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	if _, err := c.ListEffectiveSpendLimits(context.Background(), []string{"user_01A", "user_01B"}); err != nil {
		t.Fatal(err)
	}
}

func TestGetSpendLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/organizations/spend_limits/spl_01X" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"type":"spend_limit","id":"spl_01X","created_at":"2026-05-11T10:02:44Z","updated_at":"2026-05-11T10:02:44Z","scope":{"type":"user","user_id":"user_01A"},"amount":"75000","currency":"USD","period":"monthly"}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	sl, err := c.GetSpendLimit(context.Background(), "spl_01X")
	if err != nil {
		t.Fatal(err)
	}
	if sl.ID != "spl_01X" || sl.Scope.UserID != "user_01A" || *sl.Amount != "75000" || sl.Currency != "USD" {
		t.Fatalf("decoded wrong: %+v", sl)
	}
}

func TestUpsertSpendLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/organizations/spend_limits" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Scope  Scope  `json:"scope"`
			Amount string `json:"amount"`
			Period string `json:"period"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Scope.Type != "user" || body.Scope.UserID != "user_01A" || body.Amount != "75000" || body.Period != "monthly" {
			t.Errorf("unexpected body: %+v", body)
		}
		fmt.Fprint(w, `{"type":"spend_limit","id":"spl_01N","created_at":"2026-05-11T10:02:44Z","updated_at":"2026-05-11T10:02:44Z","scope":{"type":"user","user_id":"user_01A"},"amount":"75000","currency":"USD","period":"monthly"}`)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	sl, err := c.UpsertSpendLimit(context.Background(), "user_01A", "75000", "monthly")
	if err != nil {
		t.Fatal(err)
	}
	if sl.ID != "spl_01N" {
		t.Fatalf("decoded wrong: %+v", sl)
	}
}

func TestDeleteSpendLimit(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/organizations/spend_limits/spl_01X" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	if err := c.DeleteSpendLimit(context.Background(), "spl_01X"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("DELETE not sent")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/client/ -v`
Expected: FAIL (컴파일 에러 — 타입·메서드 미구현)

- [ ] **Step 3: `internal/client/spend_limits.go` 구현**

```go
package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Scope identifies what a spend limit applies to. Only type "user" is
// writable through this API; treat other types as an open set.
type Scope struct {
	Type   string `json:"type"`
	UserID string `json:"user_id,omitempty"`
}

// Source describes which hierarchy level an effective limit resolved from:
// user, seat_tier, rbac_group, or organization (open set).
type Source struct {
	Type     string `json:"type"`
	SeatTier string `json:"seat_tier,omitempty"`
}

// Actor is the member a spend limit row applies to.
type Actor struct {
	Type         string `json:"type"`
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	EmailAddress string `json:"email_address"`
	Deleted      bool   `json:"deleted"`
}

// SpendLimit is a configured spend-limit row (per-user override).
type SpendLimit struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Scope     Scope   `json:"scope"`
	Amount    *string `json:"amount"`
	Currency  string  `json:"currency"`
	Period    string  `json:"period"`
}

// EffectiveSpendLimit is one member's resolved limit as returned by the
// /effective listing. A nil Amount means unlimited.
type EffectiveSpendLimit struct {
	Scope             Scope   `json:"scope"`
	Actor             Actor   `json:"actor"`
	Amount            *string `json:"amount"`
	Currency          string  `json:"currency"`
	Period            string  `json:"period"`
	Source            Source  `json:"source"`
	SpendLimitID      string  `json:"spend_limit_id"`
	PeriodToDateSpend string  `json:"period_to_date_spend"`
}

const (
	spendLimitsBasePath      = "/v1/organizations/spend_limits"
	spendLimitsEffectivePath = spendLimitsBasePath + "/effective"
	effectivePageLimit       = "100"
)

type effectivePage struct {
	Data     []EffectiveSpendLimit `json:"data"`
	NextPage *string               `json:"next_page"`
}

// ListEffectiveSpendLimits walks every page of the /effective listing.
// Pass userIDs to filter server-side; nil fetches the whole organization.
// Cursors are bound to their query parameters, so nothing but the cursor
// may change between pages.
func (c *Client) ListEffectiveSpendLimits(ctx context.Context, userIDs []string) ([]EffectiveSpendLimit, error) {
	q := url.Values{}
	q.Set("limit", effectivePageLimit)
	for _, id := range userIDs {
		q.Add("user_ids[]", id)
	}

	var all []EffectiveSpendLimit
	for {
		var page effectivePage
		if err := c.do(ctx, http.MethodGet, spendLimitsEffectivePath, q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if page.NextPage == nil || *page.NextPage == "" {
			return all, nil
		}
		q.Set("page", *page.NextPage)
	}
}

// GetSpendLimit fetches one configured spend limit by ID.
func (c *Client) GetSpendLimit(ctx context.Context, id string) (*SpendLimit, error) {
	var sl SpendLimit
	if err := c.do(ctx, http.MethodGet, spendLimitsBasePath+"/"+url.PathEscape(id), nil, nil, &sl); err != nil {
		return nil, err
	}
	return &sl, nil
}

// UpsertSpendLimit creates or overwrites the per-user override keyed on
// (scope, period).
func (c *Client) UpsertSpendLimit(ctx context.Context, userID, amount, period string) (*SpendLimit, error) {
	body := map[string]any{
		"scope":  Scope{Type: "user", UserID: userID},
		"amount": amount,
		"period": period,
	}
	var sl SpendLimit
	if err := c.do(ctx, http.MethodPost, spendLimitsBasePath, nil, body, &sl); err != nil {
		return nil, err
	}
	if sl.ID == "" {
		return nil, fmt.Errorf("upsert response missing spend limit id")
	}
	return &sl, nil
}

// DeleteSpendLimit removes a per-user override; the member falls back to
// the inherited limit.
func (c *Client) DeleteSpendLimit(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, spendLimitsBasePath+"/"+url.PathEscape(id), nil, nil, nil)
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/client/ -v`
Expected: PASS 전부

- [ ] **Step 5: Commit**

```bash
git add internal/client/
git commit -m "feat: add spend limits endpoints with cursor pagination"
```

---

### Task 5: Mock Admin API 서버 (`testutil`)

**Files:**
- Create: `internal/provider/testutil/mockserver.go`, `internal/provider/testutil/mockserver_test.go`

**Interfaces:**
- Consumes: 없음 (net/http만). 단, 응답 JSON은 Task 4의 클라이언트 타입과 정확히 같은 계약을 따라야 한다 (플랜 서두의 "API 응답 형태" 참조)
- Produces (acceptance test 전체가 사용):
  - `testutil.Member{UserID, Email, Name string; Deleted bool}`
  - `testutil.NewMockAdminAPI(apiKey string, members []Member) *MockAdminAPI` — 시작된 `httptest.Server` 포함
  - `(m *MockAdminAPI) URL() string`, `Close()`, `EffectiveRequestCount() int`, `SetPageSize(n int)`
  - `(m *MockAdminAPI) SeedOverride(userID, amount string) string` — 테스트 사전 상태 주입, 생성된 spend_limit_id 반환
  - `(m *MockAdminAPI) OverrideCount() int` — destroy 검증용
- 동작 계약:
  - 모든 요청에서 `x-api-key`가 일치하지 않으면 401 에러 JSON
  - `GET /effective`: 멤버당 1행. override 있으면 `amount`=override 값·`source:{"type":"user"}`·`spend_limit_id`=override id, 없으면 `amount:null`·`source:{"type":"seat_tier","seat_tier":"enterprise_standard"}`·`spend_limit_id:"spl_inherited_<userID>"`. `user_ids[]` 필터 지원. 페이지 크기 `PageSize`(기본 2 — 멤버 3명이면 반드시 페이지네이션 발생), cursor는 `"page_<offset>"` 형식, 마지막 페이지는 `next_page: null`
  - `POST /spend_limits`: `scope.type != "user"` → 400. 멤버에 없는 user_id → 404. `(user_id, period)` upsert — 기존 row가 있으면 **동일 id 유지**하고 amount만 갱신. period 미지정 시 `"monthly"`. 새 id는 `fmt.Sprintf("spl_mock%04d", n)` 순번
  - `GET /spend_limits/{id}`: 저장된 override만 조회 가능, 없으면 404
  - `DELETE /spend_limits/{id}`: 삭제, 없으면 404

- [ ] **Step 1: 실패하는 테스트 작성 — `internal/provider/testutil/mockserver_test.go`**

Task 4의 실제 클라이언트로 mock을 검증한다 (계약 일치를 컴파일 타임 + 런타임으로 보장):

```go
package testutil

import (
	"context"
	"testing"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
)

func newMock(t *testing.T) (*MockAdminAPI, *client.Client) {
	t.Helper()
	m := NewMockAdminAPI("test-key", []Member{
		{UserID: "user_01A", Email: "alice@example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
		{UserID: "user_01C", Email: "carol@example.com", Name: "Carol", Deleted: true},
	})
	t.Cleanup(m.Close)
	c, err := client.New(m.URL(), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	return m, c
}

func TestMockEffectivePaginationAndInheritance(t *testing.T) {
	m, c := newMock(t)

	rows, err := c.ListEffectiveSpendLimits(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Page size 2 with 3 members forces two requests.
	if m.EffectiveRequestCount() != 2 {
		t.Fatalf("expected 2 paginated requests, got %d", m.EffectiveRequestCount())
	}
	if rows[0].Amount != nil || rows[0].Source.Type != "seat_tier" {
		t.Fatalf("expected inherited row, got %+v", rows[0])
	}
	if !rows[2].Actor.Deleted {
		t.Fatal("carol should be flagged deleted")
	}
}

func TestMockUpsertLifecycle(t *testing.T) {
	_, c := newMock(t)
	ctx := context.Background()

	created, err := c.UpsertSpendLimit(ctx, "user_01A", "50000", "monthly")
	if err != nil {
		t.Fatal(err)
	}
	if created.Scope.UserID != "user_01A" || *created.Amount != "50000" || created.Currency != "USD" {
		t.Fatalf("created wrong: %+v", created)
	}

	// Upsert again: same id, new amount.
	updated, err := c.UpsertSpendLimit(ctx, "user_01A", "75000", "monthly")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || *updated.Amount != "75000" {
		t.Fatalf("upsert must keep id and update amount: %+v vs %+v", created, updated)
	}

	got, err := c.GetSpendLimit(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Amount != "75000" {
		t.Fatalf("get after upsert: %+v", got)
	}

	// Effective row now reflects the override.
	rows, err := c.ListEffectiveSpendLimits(ctx, []string{"user_01A"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Source.Type != "user" || *rows[0].Amount != "75000" || rows[0].SpendLimitID != created.ID {
		t.Fatalf("effective row wrong: %+v", rows)
	}

	if err := c.DeleteSpendLimit(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSpendLimit(ctx, created.ID); !client.IsNotFound(err) {
		t.Fatalf("expected 404 after delete, got %v", err)
	}
	if err := c.DeleteSpendLimit(ctx, created.ID); !client.IsNotFound(err) {
		t.Fatalf("second delete should 404, got %v", err)
	}
}

func TestMockRejectsUnknownUserAndBadScope(t *testing.T) {
	_, c := newMock(t)
	if _, err := c.UpsertSpendLimit(context.Background(), "user_ghost", "1", "monthly"); !client.IsNotFound(err) {
		t.Fatalf("expected 404 for unknown user, got %v", err)
	}
}

func TestMockAuth(t *testing.T) {
	m := NewMockAdminAPI("right-key", []Member{{UserID: "user_01A", Email: "a@example.com", Name: "A"}})
	defer m.Close()
	c, _ := client.New(m.URL(), "wrong-key")
	_, err := c.ListEffectiveSpendLimits(context.Background(), nil)
	if err == nil {
		t.Fatal("expected 401 error")
	}
}

func TestMockSeedOverride(t *testing.T) {
	m, c := newMock(t)
	id := m.SeedOverride("user_01B", "12300")
	got, err := c.GetSpendLimit(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope.UserID != "user_01B" || *got.Amount != "12300" {
		t.Fatalf("seeded wrong: %+v", got)
	}
	if m.OverrideCount() != 1 {
		t.Fatalf("OverrideCount = %d", m.OverrideCount())
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/provider/testutil/ -v`
Expected: FAIL (컴파일 에러)

- [ ] **Step 3: `internal/provider/testutil/mockserver.go` 구현**

```go
// Package testutil provides an in-memory mock of the Claude Enterprise
// Admin API spend limits endpoints for acceptance tests.
package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// Member is an organization member served by the mock.
type Member struct {
	UserID  string
	Email   string
	Name    string
	Deleted bool
}

type storedLimit struct {
	ID     string
	UserID string
	Amount string
	Period string
}

// MockAdminAPI is a stateful fake of the spend limits endpoints.
type MockAdminAPI struct {
	server *httptest.Server
	apiKey string

	mu                sync.Mutex
	pageSize          int
	members           []Member
	overrides         map[string]*storedLimit // key: userID + "\x00" + period
	byID              map[string]*storedLimit
	nextID            int
	effectiveRequests int
}

// NewMockAdminAPI starts the mock server. Callers must Close it.
func NewMockAdminAPI(apiKey string, members []Member) *MockAdminAPI {
	m := &MockAdminAPI{
		apiKey:    apiKey,
		pageSize:  2,
		members:   members,
		overrides: map[string]*storedLimit{},
		byID:      map[string]*storedLimit{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/organizations/spend_limits/effective", m.handleEffective)
	mux.HandleFunc("/v1/organizations/spend_limits", m.handleUpsert)
	mux.HandleFunc("/v1/organizations/spend_limits/", m.handleByID)
	m.server = httptest.NewServer(mux)
	return m
}

// URL returns the mock's base URL for the provider's base_url attribute.
func (m *MockAdminAPI) URL() string { return m.server.URL }

// Close shuts the underlying server down.
func (m *MockAdminAPI) Close() { m.server.Close() }

// SetPageSize adjusts how many effective rows are served per page.
func (m *MockAdminAPI) SetPageSize(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pageSize = n
}

// EffectiveRequestCount reports how many times /effective was called.
func (m *MockAdminAPI) EffectiveRequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.effectiveRequests
}

// OverrideCount reports how many per-user overrides currently exist.
func (m *MockAdminAPI) OverrideCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.byID)
}

// SeedOverride injects a pre-existing override and returns its id.
func (m *MockAdminAPI) SeedOverride(userID, amount string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertLocked(userID, amount, "monthly").ID
}

func (m *MockAdminAPI) upsertLocked(userID, amount, period string) *storedLimit {
	key := userID + "\x00" + period
	if existing, ok := m.overrides[key]; ok {
		existing.Amount = amount
		return existing
	}
	m.nextID++
	sl := &storedLimit{
		ID:     fmt.Sprintf("spl_mock%04d", m.nextID),
		UserID: userID,
		Amount: amount,
		Period: period,
	}
	m.overrides[key] = sl
	m.byID[sl.ID] = sl
	return sl
}

func (m *MockAdminAPI) authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("x-api-key") != m.apiKey {
		writeError(w, http.StatusUnauthorized, "authentication_error", "invalid x-api-key")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "error",
		"error":      map[string]string{"type": errType, "message": msg},
		"request_id": "req_mock",
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func spendLimitJSON(sl *storedLimit) map[string]any {
	return map[string]any{
		"type":       "spend_limit",
		"id":         sl.ID,
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-01T00:00:00Z",
		"scope":      map[string]string{"type": "user", "user_id": sl.UserID},
		"amount":     sl.Amount,
		"currency":   "USD",
		"period":     sl.Period,
	}
}

func (m *MockAdminAPI) effectiveRow(member Member) map[string]any {
	row := map[string]any{
		"scope": map[string]string{"type": "user", "user_id": member.UserID},
		"actor": map[string]any{
			"type":          "user_actor",
			"user_id":       member.UserID,
			"name":          member.Name,
			"email_address": member.Email,
			"deleted":       member.Deleted,
		},
		"currency":             "USD",
		"period":               "monthly",
		"period_to_date_spend": "0",
	}
	if ov, ok := m.overrides[member.UserID+"\x00monthly"]; ok {
		row["amount"] = ov.Amount
		row["source"] = map[string]string{"type": "user"}
		row["spend_limit_id"] = ov.ID
	} else {
		row["amount"] = nil
		row["source"] = map[string]string{"type": "seat_tier", "seat_tier": "enterprise_standard"}
		row["spend_limit_id"] = "spl_inherited_" + member.UserID
	}
	return row
}

func (m *MockAdminAPI) handleEffective(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(w, r) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.effectiveRequests++

	filter := map[string]bool{}
	for _, id := range r.URL.Query()["user_ids[]"] {
		filter[id] = true
	}
	var rows []map[string]any
	for _, member := range m.members {
		if len(filter) > 0 && !filter[member.UserID] {
			continue
		}
		rows = append(rows, m.effectiveRow(member))
	}

	offset := 0
	if cursor := r.URL.Query().Get("page"); cursor != "" {
		n, err := strconv.Atoi(strings.TrimPrefix(cursor, "page_"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request_error", "cursor does not match current query parameters")
			return
		}
		offset = n
	}
	end := offset + m.pageSize
	if end > len(rows) {
		end = len(rows)
	}
	var nextPage any
	if end < len(rows) {
		nextPage = fmt.Sprintf("page_%d", end)
	}
	page := rows[offset:end]
	if page == nil {
		page = []map[string]any{}
	}
	writeJSON(w, map[string]any{"data": page, "next_page": nextPage})
}

func (m *MockAdminAPI) handleUpsert(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	var body struct {
		Scope struct {
			Type   string `json:"type"`
			UserID string `json:"user_id"`
		} `json:"scope"`
		Amount string `json:"amount"`
		Period string `json:"period"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "malformed JSON body")
		return
	}
	if body.Scope.Type != "user" {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "only scope.type \"user\" can be written")
		return
	}
	if body.Period == "" {
		body.Period = "monthly"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	known := false
	for _, member := range m.members {
		if member.UserID == body.Scope.UserID {
			known = true
			break
		}
	}
	if !known {
		writeError(w, http.StatusNotFound, "not_found_error", "user not found: "+body.Scope.UserID)
		return
	}
	writeJSON(w, spendLimitJSON(m.upsertLocked(body.Scope.UserID, body.Amount, body.Period)))
}

func (m *MockAdminAPI) handleByID(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/organizations/spend_limits/")

	m.mu.Lock()
	defer m.mu.Unlock()
	sl, ok := m.byID[id]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found_error", "spend limit not found: "+id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, spendLimitJSON(sl))
	case http.MethodDelete:
		delete(m.byID, id)
		delete(m.overrides, sl.UserID+"\x00"+sl.Period)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/provider/testutil/ -v`
Expected: PASS 5건

- [ ] **Step 5: Commit**

```bash
git add internal/provider/testutil/
git commit -m "test: add in-memory mock admin API server for acceptance tests"
```

---

### Task 6: Provider Configure + acceptance test 기반

**Files:**
- Modify: `internal/provider/provider.go` (Configure 구현, providerData 추가)
- Modify: `internal/provider/provider_test.go` (acceptance 팩토리 + Configure 테스트 추가)

**Interfaces:**
- Consumes: `client.New`, `client.DefaultBaseURL` (Task 2)
- Produces:
  - `providerData{Client *client.Client; Resolver *Resolver}` — 리소스/데이터소스가 `req.ProviderData.(*providerData)`로 수신. **주의: Resolver 타입은 Task 8에서 구현되므로, 이 Task에서는 필드 없이 `providerData{Client *client.Client}`로 만들고 Task 8에서 필드를 추가한다.**
  - env 규약: `admin_api_key` 미지정 시 `ANTHROPIC_ADMIN_KEY` 사용, 둘 다 없으면 에러
  - 테스트 헬퍼 `protoV6Factories()` + `providerConfig(mockURL string) string` — 이후 모든 acceptance test가 사용

- [ ] **Step 1: 실패하는 테스트 추가 — `internal/provider/provider_test.go`에 append**

```go
// --- append to provider_test.go ---

import 추가:
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

// protoV6Factories wires the in-process provider into terraform-plugin-testing.
func protoV6Factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"claude-enterprise": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// providerConfig points the provider at a mock server.
func providerConfig(mockURL string) string {
	return fmt.Sprintf(`
provider "claude-enterprise" {
  admin_api_key = "test-key"
  base_url      = %q
}
`, mockURL)
}

func TestAccProviderMissingAPIKey(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	t.Setenv("ANTHROPIC_ADMIN_KEY", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "claude-enterprise" {}

data "claude-enterprise_members" "all" {}
`,
				ExpectError: regexp.MustCompile(`(?s)Missing Admin API key`),
			},
		},
	})
}
```

주의: `data "claude-enterprise_members"`는 Task 7에서 등록되므로 이 시점에는 "Invalid data source" 에러가 먼저 난다. **이 Task에서는 임시로 data source 없이 provider만 있는 config는 Configure를 트리거하지 않으므로**, 이 테스트는 이 Task에서 작성하되 `t.Skip("enabled in Task 7")`를 첫 줄에 넣고, Task 7에서 skip을 제거한다. 대신 이 Task에서 검증 가능한 unit test를 추가한다:

```go
func TestConfigureResolvesAPIKeyFromEnv(t *testing.T) {
	// Unit-level check of the key resolution helper (no Terraform involved).
	t.Setenv("ANTHROPIC_ADMIN_KEY", "env-key")
	if got := provider.ResolveAPIKeyForTest("", ""); got != "env-key" {
		t.Fatalf("expected env fallback, got %q", got)
	}
	if got := provider.ResolveAPIKeyForTest("explicit", "ignored"); got != "explicit" {
		t.Fatalf("config value must win, got %q", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/provider/ -run 'TestConfigure|TestAccProvider' -v`
Expected: FAIL (컴파일 에러 — ResolveAPIKeyForTest 없음)

- [ ] **Step 3: Configure 구현 — `internal/provider/provider.go` 수정**

기존의 빈 Configure를 교체하고 providerData·헬퍼를 추가한다:

```go
import 추가: "os", "github.com/hashicorp/terraform-plugin-framework/path",
	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"

// providerData is handed to every resource and data source via
// resp.ResourceData / resp.DataSourceData.
type providerData struct {
	Client *client.Client
}

// resolveAPIKey returns the effective key: explicit config wins over the
// ANTHROPIC_ADMIN_KEY environment variable.
func resolveAPIKey(configValue, envValue string) string {
	if configValue != "" {
		return configValue
	}
	return envValue
}

// ResolveAPIKeyForTest exposes resolveAPIKey to the external test package.
func ResolveAPIKeyForTest(configValue, envValue string) string {
	if envValue == "" {
		envValue = os.Getenv("ANTHROPIC_ADMIN_KEY")
	}
	return resolveAPIKey(configValue, envValue)
}

func (p *claudeEnterpriseProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := resolveAPIKey(config.AdminAPIKey.ValueString(), os.Getenv("ANTHROPIC_ADMIN_KEY"))
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("admin_api_key"),
			"Missing Admin API key",
			"Set the admin_api_key provider attribute or the ANTHROPIC_ADMIN_KEY environment variable. "+
				"The key must be a scoped Admin API key (sk-ant-admin...) with read:spend_limits and write:spend_limits scopes.",
		)
		return
	}

	baseURL := client.DefaultBaseURL
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}

	c, err := client.New(baseURL, apiKey)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API client", err.Error())
		return
	}

	data := &providerData{Client: c}
	resp.ResourceData = data
	resp.DataSourceData = data
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go build ./... && go test ./internal/provider/ -v`
Expected: PASS (acceptance test는 skip)

- [ ] **Step 5: Commit**

```bash
git add internal/provider/
git commit -m "feat: configure provider with API key resolution and client wiring"
```

---

### Task 7: `claude-enterprise_members` 데이터소스

**Files:**
- Create: `internal/provider/data_source_members.go`, `internal/provider/data_source_members_test.go`
- Modify: `internal/provider/provider.go` (DataSources에 등록)
- Modify: `internal/provider/provider_test.go` (`TestAccProviderMissingAPIKey`의 `t.Skip` 제거)

**Interfaces:**
- Consumes: `providerData.Client`, `client.ListEffectiveSpendLimits`, `testutil.NewMockAdminAPI`, `protoV6Factories()`, `providerConfig()`
- Produces: 데이터소스 `claude-enterprise_members` — 출력 `by_email` / `by_user_id` (map of object: `user_id, email, name, effective_amount(nullable), currency, period, source_type, spend_limit_id, period_to_date_spend`). `NewMembersDataSource() datasource.DataSource` 팩토리. `memberAttrTypes`(object 속성 정의)는 리소스 쪽에서 재사용하지 않음(리소스는 별도 스키마)

- [ ] **Step 1: 실패하는 acceptance test 작성 — `internal/provider/data_source_members_test.go`**

```go
package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func TestAccMembersDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := testutil.NewMockAdminAPI("test-key", []testutil.Member{
		{UserID: "user_01A", Email: "Alice@Example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
		{UserID: "user_01C", Email: "carol@example.com", Name: "Carol", Deleted: true},
	})
	defer mock.Close()
	mock.SeedOverride("user_01B", "50000")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(mock.URL()) + `
data "claude-enterprise_members" "all" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Emails are normalized to lower case as map keys.
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.user_id", "user_01A"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.name", "Alice"),
					// Inherited (no override): effective_amount is null, source seat_tier.
					resource.TestCheckNoResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.effective_amount"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.source_type", "seat_tier"),
					// Overridden member.
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.bob@example.com.effective_amount", "50000"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.bob@example.com.source_type", "user"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_user_id.user_01B.email", "bob@example.com"),
					// Deleted members are excluded.
					resource.TestCheckNoResourceAttr("data.claude-enterprise_members.all", "by_email.carol@example.com"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.%", "2"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_user_id.%", "2"),
				),
			},
		},
	})
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `TF_ACC=1 go test ./internal/provider/ -run TestAccMembersDataSource -v`
Expected: FAIL — "Invalid data source ... claude-enterprise_members" (미등록)

- [ ] **Step 3: `internal/provider/data_source_members.go` 구현**

```go
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NewMembersDataSource registers claude-enterprise_members.
func NewMembersDataSource() datasource.DataSource {
	return &membersDataSource{}
}

type membersDataSource struct {
	data *providerData
}

type memberModel struct {
	UserID            types.String `tfsdk:"user_id"`
	Email             types.String `tfsdk:"email"`
	Name              types.String `tfsdk:"name"`
	EffectiveAmount   types.String `tfsdk:"effective_amount"`
	Currency          types.String `tfsdk:"currency"`
	Period            types.String `tfsdk:"period"`
	SourceType        types.String `tfsdk:"source_type"`
	SpendLimitID      types.String `tfsdk:"spend_limit_id"`
	PeriodToDateSpend types.String `tfsdk:"period_to_date_spend"`
}

type membersModel struct {
	ByEmail  map[string]memberModel `tfsdk:"by_email"`
	ByUserID map[string]memberModel `tfsdk:"by_user_id"`
}

func (d *membersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_members"
}

func memberAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"user_id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Member user ID (`user_01...`)."},
		"email":                schema.StringAttribute{Computed: true, MarkdownDescription: "Member email address (lower-cased)."},
		"name":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Member display name."},
		"effective_amount":     schema.StringAttribute{Computed: true, MarkdownDescription: "Effective spend limit in minor units (cents). Null means unlimited; `\"0\"` means included usage only."},
		"currency":             schema.StringAttribute{Computed: true, MarkdownDescription: "Billing currency code, e.g. `USD`."},
		"period":               schema.StringAttribute{Computed: true, MarkdownDescription: "Limit period, currently `monthly`."},
		"source_type":          schema.StringAttribute{Computed: true, MarkdownDescription: "Where the limit resolved from: `user`, `seat_tier`, `rbac_group`, or `organization` (open set)."},
		"spend_limit_id":       schema.StringAttribute{Computed: true, MarkdownDescription: "ID of the spend limit row the effective limit resolved from."},
		"period_to_date_spend": schema.StringAttribute{Computed: true, MarkdownDescription: "Spend accrued this period, informational only (may temporarily read \"0\")."},
	}
}

func (d *membersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	memberObject := schema.NestedAttributeObject{Attributes: memberAttributes()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every current organization member with their effective spend limit. " +
			"Use `by_email` to resolve emails to user IDs. Deleted members are excluded.",
		Attributes: map[string]schema.Attribute{
			"by_email": schema.MapNestedAttribute{
				Computed:            true,
				NestedObject:        memberObject,
				MarkdownDescription: "Members keyed by lower-cased email address.",
			},
			"by_user_id": schema.MapNestedAttribute{
				Computed:            true,
				NestedObject:        memberObject,
				MarkdownDescription: "Members keyed by user ID.",
			},
		},
	}
}

func (d *membersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.data = data
}

func (d *membersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	rows, err := d.data.Client.ListEffectiveSpendLimits(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list effective spend limits", err.Error())
		return
	}

	state := membersModel{
		ByEmail:  map[string]memberModel{},
		ByUserID: map[string]memberModel{},
	}
	for _, row := range rows {
		if row.Actor.Deleted {
			continue
		}
		email := strings.ToLower(row.Actor.EmailAddress)
		m := memberModel{
			UserID:            types.StringValue(row.Actor.UserID),
			Email:             types.StringValue(email),
			Name:              types.StringValue(row.Actor.Name),
			EffectiveAmount:   types.StringPointerValue(row.Amount),
			Currency:          types.StringValue(row.Currency),
			Period:            types.StringValue(row.Period),
			SourceType:        types.StringValue(row.Source.Type),
			SpendLimitID:      types.StringValue(row.SpendLimitID),
			PeriodToDateSpend: types.StringValue(row.PeriodToDateSpend),
		}
		state.ByEmail[email] = m
		state.ByUserID[row.Actor.UserID] = m
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
```

`provider.go`의 DataSources를 교체:

```go
func (p *claudeEnterpriseProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewMembersDataSource}
}
```

`provider_test.go`의 `TestAccProviderMissingAPIKey`에서 `t.Skip` 줄 제거.

- [ ] **Step 4: 테스트 통과 확인**

Run: `TF_ACC=1 go test ./internal/provider/ -run 'TestAccMembersDataSource|TestAccProviderMissingAPIKey' -v`
Expected: PASS 2건. 여기가 **하이픈 포함 타입명(`claude-enterprise_members`)이 실제 terraform CLI를 통과하는지 검증하는 최초 지점** — 만약 terraform이 타입명을 거부하면 즉시 중단하고 provider TypeName을 `claudeenterprise`로 바꾸는 결정을 사용자에게 보고할 것 (스펙 §3의 이름 결정 변경이 필요해지므로).

- [ ] **Step 5: Commit**

```bash
git add internal/provider/
git commit -m "feat: add members data source with by_email and by_user_id maps"
```

---

### Task 8: email→user_id Resolver

**Files:**
- Create: `internal/provider/resolver.go`, `internal/provider/resolver_test.go`
- Modify: `internal/provider/provider.go` (providerData에 Resolver 필드 추가, Configure에서 초기화)

**Interfaces:**
- Consumes: `client.ListEffectiveSpendLimits`, `testutil` mock
- Produces:
  - `NewResolver(c *client.Client) *Resolver`
  - `(r *Resolver) UserIDByEmail(ctx context.Context, email string) (string, error)` — 대소문자 무시, deleted 제외, 최초 1회만 `/effective` 전체 순회 후 캐시. 실패 시 에러 메시지에 email 포함
  - `providerData{Client *client.Client; Resolver *Resolver}` (필드 추가)

- [ ] **Step 1: 실패하는 테스트 작성 — `internal/provider/resolver_test.go`**

주의: 이 테스트는 비공개 API(`NewResolver`)를 쓰므로 **`package provider`** (internal test)로 작성한다. 기존 acceptance test 파일들(`package provider_test`)과 공존 가능.

```go
package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func newResolverFixture(t *testing.T) (*testutil.MockAdminAPI, *Resolver) {
	t.Helper()
	mock := testutil.NewMockAdminAPI("k", []testutil.Member{
		{UserID: "user_01A", Email: "Alice@Example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
		{UserID: "user_01C", Email: "carol@example.com", Name: "Carol", Deleted: true},
	})
	t.Cleanup(mock.Close)
	c, err := client.New(mock.URL(), "k")
	if err != nil {
		t.Fatal(err)
	}
	return mock, NewResolver(c)
}

func TestResolverCaseInsensitiveAndCached(t *testing.T) {
	mock, r := newResolverFixture(t)
	ctx := context.Background()

	id, err := r.UserIDByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if id != "user_01A" {
		t.Fatalf("got %q", id)
	}

	// Page size 2, 3 members -> the initial fill takes exactly 2 requests.
	fills := mock.EffectiveRequestCount()
	if fills != 2 {
		t.Fatalf("expected 2 requests for cache fill, got %d", fills)
	}

	// Subsequent lookups (any case) must not hit the API again.
	if _, err := r.UserIDByEmail(ctx, "BOB@EXAMPLE.COM"); err != nil {
		t.Fatal(err)
	}
	if mock.EffectiveRequestCount() != fills {
		t.Fatalf("cache miss: request count grew to %d", mock.EffectiveRequestCount())
	}
}

func TestResolverUnknownEmail(t *testing.T) {
	_, r := newResolverFixture(t)
	_, err := r.UserIDByEmail(context.Background(), "ghost@example.com")
	if err == nil || !strings.Contains(err.Error(), "ghost@example.com") {
		t.Fatalf("expected error naming the email, got %v", err)
	}
}

func TestResolverExcludesDeletedMembers(t *testing.T) {
	_, r := newResolverFixture(t)
	if _, err := r.UserIDByEmail(context.Background(), "carol@example.com"); err == nil {
		t.Fatal("deleted member must not resolve")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/provider/ -run TestResolver -v`
Expected: FAIL (컴파일 에러)

- [ ] **Step 3: `internal/provider/resolver.go` 구현 + providerData 필드 추가**

```go
package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
)

// Resolver maps member emails to user IDs. The /effective listing is
// walked once per provider instance and cached, so a plan with many
// email-keyed resources costs one listing, not one per resource.
type Resolver struct {
	client *client.Client

	mu      sync.Mutex
	loaded  bool
	byEmail map[string]string
}

// NewResolver builds a Resolver on top of the API client.
func NewResolver(c *client.Client) *Resolver {
	return &Resolver{client: c}
}

// UserIDByEmail resolves an email (case-insensitive) to a user ID.
// Deleted members are excluded. The mutex intentionally serializes
// concurrent first-time fills so the listing runs at most once.
func (r *Resolver) UserIDByEmail(ctx context.Context, email string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.loaded {
		rows, err := r.client.ListEffectiveSpendLimits(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("listing organization members: %w", err)
		}
		r.byEmail = make(map[string]string, len(rows))
		for _, row := range rows {
			if row.Actor.Deleted || row.Actor.EmailAddress == "" {
				continue
			}
			r.byEmail[strings.ToLower(row.Actor.EmailAddress)] = row.Actor.UserID
		}
		r.loaded = true
	}

	id, ok := r.byEmail[strings.ToLower(email)]
	if !ok {
		return "", fmt.Errorf("no organization member found with email %q", email)
	}
	return id, nil
}
```

`provider.go` 수정 — providerData에 필드 추가, Configure에서 초기화:

```go
type providerData struct {
	Client   *client.Client
	Resolver *Resolver
}

// Configure 내부의 조립부:
	data := &providerData{Client: c, Resolver: NewResolver(c)}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/provider/... -v`
Expected: PASS (acceptance는 TF_ACC 없으면 skip)

- [ ] **Step 5: Commit**

```bash
git add internal/provider/
git commit -m "feat: add cached email-to-user-id resolver"
```

---

### Task 9: `claude-enterprise_spend_limit` 리소스 — user_id 경로 (CRUD + import by id)

**Files:**
- Create: `internal/provider/resource_spend_limit.go`, `internal/provider/resource_spend_limit_test.go`
- Modify: `internal/provider/provider.go` (Resources에 등록)

**Interfaces:**
- Consumes: `providerData{Client, Resolver}`, `client.UpsertSpendLimit/GetSpendLimit/DeleteSpendLimit`, `client.IsNotFound`, `testutil` mock, `protoV6Factories()`, `providerConfig()`
- Produces: `NewSpendLimitResource() resource.Resource`. 스키마 — `user_id`(Optional+Computed), `user_email`(Optional), `amount`(Required), `period`(Optional+Computed, 기본 `"monthly"`), `id`/`currency`(Computed). email 해소(`ModifyPlan`)와 user_id import는 Task 10에서 완성 — 이 Task에서는 자리만 두고 user_id 경로를 완성한다.

- [ ] **Step 1: 실패하는 acceptance test 작성 — `internal/provider/resource_spend_limit_test.go`**

```go
package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func newResourceMock(t *testing.T) *testutil.MockAdminAPI {
	t.Helper()
	mock := testutil.NewMockAdminAPI("test-key", []testutil.Member{
		{UserID: "user_01A", Email: "alice@example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
	})
	t.Cleanup(mock.Close)
	return mock
}

func checkNoOverridesLeft(mock *testutil.MockAdminAPI) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if n := mock.OverrideCount(); n != 0 {
			return fmt.Errorf("expected 0 overrides after destroy, found %d", n)
		}
		return nil
	}
}

func TestAccSpendLimitLifecycleByUserID(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	configFor := func(amount string) string {
		return providerConfig(mock.URL()) + fmt.Sprintf(`
resource "claude-enterprise_spend_limit" "test" {
  user_id = "user_01A"
  amount  = %q
}
`, amount)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		CheckDestroy:             checkNoOverridesLeft(mock),
		Steps: []resource.TestStep{
			{ // Create
				Config: configFor("50000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("claude-enterprise_spend_limit.test", "id"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "user_id", "user_01A"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "amount", "50000"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "period", "monthly"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "currency", "USD"),
					resource.TestCheckNoResourceAttr("claude-enterprise_spend_limit.test", "user_email"),
				),
			},
			{ // Update amount in place
				Config: configFor("75000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "amount", "75000"),
				),
			},
			{ // Import by spend_limit_id
				ResourceName:      "claude-enterprise_spend_limit.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSpendLimitDisappearsOutOfBand(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	config := providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_id = "user_01B"
  amount  = "10000"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{Config: config},
			{ // Someone deletes the override in the console; refresh must plan recreation.
				PreConfig: func() {
					// Simulate out-of-band deletion by clearing the stored override.
					mock.DeleteAllOverrides()
				},
				Config:             config,
				ExpectNonEmptyPlan: true,
				PlanOnly:           true,
			},
		},
	})
}

func TestAccSpendLimitRequiresExactlyOneIdentity(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_id    = "user_01A"
  user_email = "alice@example.com"
  amount     = "1"
}
`,
				ExpectError: mustCompile(t, `Exactly one of`),
			},
			{
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  amount = "1"
}
`,
				ExpectError: mustCompile(t, `Exactly one of`),
			},
		},
	})
}
```

파일 상단에 헬퍼 추가:

```go
import "regexp"

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	return regexp.MustCompile(`(?s)` + pattern)
}
```

그리고 mock에 out-of-band 삭제 헬퍼가 필요하다 — `internal/provider/testutil/mockserver.go`에 추가:

```go
// DeleteAllOverrides clears every stored override, simulating out-of-band
// deletion (e.g. an admin removing limits in the claude.ai console).
func (m *MockAdminAPI) DeleteAllOverrides() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overrides = map[string]*storedLimit{}
	m.byID = map[string]*storedLimit{}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `TF_ACC=1 go test ./internal/provider/ -run TestAccSpendLimit -v`
Expected: FAIL — "Invalid resource type ... claude-enterprise_spend_limit"

- [ ] **Step 3: `internal/provider/resource_spend_limit.go` 구현**

```go
package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
)

// NewSpendLimitResource registers claude-enterprise_spend_limit.
func NewSpendLimitResource() resource.Resource {
	return &spendLimitResource{}
}

type spendLimitResource struct {
	data *providerData
}

type spendLimitModel struct {
	ID        types.String `tfsdk:"id"`
	UserID    types.String `tfsdk:"user_id"`
	UserEmail types.String `tfsdk:"user_email"`
	Amount    types.String `tfsdk:"amount"`
	Period    types.String `tfsdk:"period"`
	Currency  types.String `tfsdk:"currency"`
}

func (r *spendLimitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_spend_limit"
}

func (r *spendLimitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one member's per-user spend limit override. " +
			"Creating adopts any override that already exists for the same user and period (the API is an upsert); " +
			"deleting returns the member to their inherited limit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Spend limit ID (`spl_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Member user ID (`user_01...`). Exactly one of `user_id` and `user_email` must be set. " +
					"Changing the target user replaces the resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_email": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Member email address, resolved to `user_id` at plan time. " +
					"Exactly one of `user_id` and `user_email` must be set. " +
					"If the same email later maps to a different user (member removed and re-invited), the resource is replaced.",
			},
			"amount": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Spend limit in minor units of the billing currency, as a string " +
					"(`\"50000\"` is 500.00 USD; `\"0\"` allows included usage only). Updated in place.",
			},
			"period": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("monthly"),
				MarkdownDescription: "Limit period. Currently only `monthly` is supported by the API.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"currency": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Billing currency code, e.g. `USD`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *spendLimitResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot("user_id"), path.MatchRoot("user_email")),
	}
}

func (r *spendLimitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.data = data
}

// resolveUserID returns the effective user ID from a (possibly unknown)
// planned user_id plus the configured email.
func (r *spendLimitResource) resolveUserID(ctx context.Context, plan spendLimitModel) (string, error) {
	if !plan.UserID.IsNull() && !plan.UserID.IsUnknown() {
		return plan.UserID.ValueString(), nil
	}
	return r.data.Resolver.UserIDByEmail(ctx, plan.UserEmail.ValueString())
}

func (r *spendLimitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan spendLimitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userID, err := r.resolveUserID(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("user_email"), "Cannot resolve user_email", err.Error())
		return
	}

	sl, err := r.data.Client.UpsertSpendLimit(ctx, userID, plan.Amount.ValueString(), plan.Period.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create spend limit", err.Error())
		return
	}

	plan.ID = types.StringValue(sl.ID)
	plan.UserID = types.StringValue(sl.Scope.UserID)
	plan.Amount = types.StringPointerValue(sl.Amount)
	plan.Period = types.StringValue(sl.Period)
	plan.Currency = types.StringValue(sl.Currency)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spendLimitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state spendLimitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sl, err := r.data.Client.GetSpendLimit(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read spend limit", err.Error())
		return
	}
	if sl.Scope.Type != "" && sl.Scope.Type != "user" {
		// Not a per-user override anymore; treat as gone.
		resp.State.RemoveResource(ctx)
		return
	}

	state.UserID = types.StringValue(sl.Scope.UserID)
	state.Amount = types.StringPointerValue(sl.Amount)
	state.Period = types.StringValue(sl.Period)
	state.Currency = types.StringValue(sl.Currency)
	// user_email is configuration identity the API cannot return; keep as-is.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *spendLimitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan spendLimitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userID, err := r.resolveUserID(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("user_email"), "Cannot resolve user_email", err.Error())
		return
	}

	sl, err := r.data.Client.UpsertSpendLimit(ctx, userID, plan.Amount.ValueString(), plan.Period.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to update spend limit", err.Error())
		return
	}

	plan.ID = types.StringValue(sl.ID)
	plan.UserID = types.StringValue(sl.Scope.UserID)
	plan.Amount = types.StringPointerValue(sl.Amount)
	plan.Period = types.StringValue(sl.Period)
	plan.Currency = types.StringValue(sl.Currency)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *spendLimitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state spendLimitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.data.Client.DeleteSpendLimit(ctx, state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete spend limit", err.Error())
	}
}

func (r *spendLimitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by user_id is completed in the next task; spend_limit_id works now.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

`provider.go`의 Resources를 교체:

```go
func (p *claudeEnterpriseProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewSpendLimitResource}
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `TF_ACC=1 go test ./internal/provider/ -run TestAccSpendLimit -v`
Expected: PASS 3건. `ImportStateVerify`가 user_email(양쪽 다 null) 포함 전 속성 일치를 검증한다.

- [ ] **Step 5: 전체 테스트 + 커밋**

Run: `TF_ACC=1 go test ./... -count=1`
Expected: PASS

```bash
git add internal/provider/
git commit -m "feat: add spend_limit resource with CRUD and import by id"
```

---

### Task 10: `spend_limit` 리소스 — user_email 경로 (plan 시점 해소, 교체 판정, user_id import)

**Files:**
- Modify: `internal/provider/resource_spend_limit.go` (ModifyPlan 추가, ImportState 확장)
- Modify: `internal/provider/resource_spend_limit_test.go` (테스트 추가)

**Interfaces:**
- Consumes: `providerData.Resolver.UserIDByEmail` (Task 8), `client.ListEffectiveSpendLimits`의 `user_ids[]` 필터 (Task 4), `EffectiveSpendLimit.Source.Type`/`SpendLimitID`
- Produces: 최종 동작 —
  - plan 시 `user_email`이 known이면 user_id를 해소해 planned `user_id`에 반영, state와 다르면 replace
  - 기존 리소스에서 `user_email`이 unknown이면 명확한 에러 (신규 생성은 apply 시 해소 허용)
  - `terraform import ... user_01Ab...` — `/effective?user_ids[]=`로 override의 `spend_limit_id`를 찾아 import; override가 없으면 에러

- [ ] **Step 1: 실패하는 acceptance test 추가 — `resource_spend_limit_test.go`에 append**

```go
import 추가:
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

func TestAccSpendLimitLifecycleByEmail(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	configFor := func(email, amount string) string {
		return providerConfig(mock.URL()) + fmt.Sprintf(`
resource "claude-enterprise_spend_limit" "test" {
  user_email = %q
  amount     = %q
}
`, email, amount)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		CheckDestroy:             checkNoOverridesLeft(mock),
		Steps: []resource.TestStep{
			{ // Create via email; user_id must be resolved.
				Config: configFor("alice@example.com", "50000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "user_id", "user_01A"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "user_email", "alice@example.com"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "amount", "50000"),
				),
			},
			{ // Amount change is an in-place update, not a replace.
				Config: configFor("alice@example.com", "60000"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("claude-enterprise_spend_limit.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "amount", "60000"),
			},
			{ // Pointing at a different member replaces the resource.
				Config: configFor("bob@example.com", "60000"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("claude-enterprise_spend_limit.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "user_id", "user_01B"),
			},
		},
	})
}

func TestAccSpendLimitSwitchUserIDToEmailSameUser(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_id = "user_01A"
  amount  = "50000"
}
`,
			},
			{ // Same member, now addressed by email: must NOT replace.
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_email = "alice@example.com"
  amount     = "50000"
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("claude-enterprise_spend_limit.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "user_id", "user_01A"),
			},
		},
	})
}

func TestAccSpendLimitImportByUserID(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)
	seededID := mock.SeedOverride("user_01A", "12300")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_id = "user_01A"
  amount  = "12300"
}
`,
				ResourceName:       "claude-enterprise_spend_limit.test",
				ImportState:        true,
				ImportStateId:      "user_01A",
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					if states[0].ID != seededID {
						return fmt.Errorf("expected id %q, got %q", seededID, states[0].ID)
					}
					if states[0].Attributes["amount"] != "12300" {
						return fmt.Errorf("amount = %q", states[0].Attributes["amount"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccSpendLimitImportByUserIDWithoutOverride(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t) // no overrides seeded

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_id = "user_01A"
  amount  = "1"
}
`,
				ResourceName:  "claude-enterprise_spend_limit.test",
				ImportState:   true,
				ImportStateId: "user_01A",
				ExpectError:   mustCompile(t, `no per-user override`),
			},
		},
	})
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `TF_ACC=1 go test ./internal/provider/ -run 'TestAccSpendLimitLifecycleByEmail|TestAccSpendLimitSwitch|TestAccSpendLimitImportByUserID' -v`
Expected: FAIL — email 해소가 plan에 없어 `ExpectResourceAction`/`user_id` 체크 불일치, user_id import는 "spend limit not found: user_01A" (passthrough가 그대로 GetSpendLimit)

- [ ] **Step 3: ModifyPlan + ImportState 구현 — `resource_spend_limit.go` 수정**

import에 `"strings"` 추가. 다음 메서드를 추가:

```go
// ModifyPlan resolves user_email to user_id at plan time so that identity
// changes surface as replacements before apply, regardless of whether the
// configuration addresses members by id or email.
func (r *spendLimitResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy plan
	}
	if r.data == nil {
		return // provider not configured (e.g. terraform validate)
	}

	var email types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("user_email"), &email)...)
	if resp.Diagnostics.HasError() || email.IsNull() {
		return
	}

	isCreate := req.State.Raw.IsNull()
	if email.IsUnknown() {
		if !isCreate {
			resp.Diagnostics.AddAttributeError(
				path.Root("user_email"),
				"user_email must be known at plan time",
				"The planned user_email is unknown, so the provider cannot tell whether the "+
					"resource still targets the same member. Use a literal email or user_id, "+
					"or apply the value's source first.",
			)
		}
		return // create with unknown email resolves at apply time
	}

	userID, err := r.data.Resolver.UserIDByEmail(ctx, email.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("user_email"), "Cannot resolve user_email", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("user_id"), userID)...)

	if !isCreate {
		var stateUserID types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("user_id"), &stateUserID)...)
		if !stateUserID.IsNull() && stateUserID.ValueString() != userID {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root("user_id"))
		}
	}
}
```

기존 ImportState를 교체:

```go
func (r *spendLimitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if strings.HasPrefix(id, "user_") {
		rows, err := r.data.Client.ListEffectiveSpendLimits(ctx, []string{id})
		if err != nil {
			resp.Diagnostics.AddError("Failed to look up spend limit for user", err.Error())
			return
		}
		var overrideID string
		for _, row := range rows {
			if row.Actor.UserID == id && row.Source.Type == "user" {
				overrideID = row.SpendLimitID
				break
			}
		}
		if overrideID == "" {
			resp.Diagnostics.AddError(
				"No per-user override to import",
				fmt.Sprintf("Member %q has no per-user override; their effective limit is inherited. "+
					"Create the override with Terraform instead of importing.", id),
			)
			return
		}
		id = overrideID
	}
	resp.State.SetAttribute(ctx, path.Root("id"), id)
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `TF_ACC=1 go test ./internal/provider/ -v`
Expected: PASS 전부 (Task 9의 기존 테스트 포함 — 회귀 확인)

- [ ] **Step 5: Commit**

```bash
git add internal/provider/
git commit -m "feat: resolve user_email at plan time and support import by user id"
```

---

### Task 11: 문서·예제·라이선스 (README, docs/, examples/, LICENSE)

**Files:**
- Create: `README.md`, `LICENSE`, `docs/index.md`, `docs/resources/spend_limit.md`, `docs/data-sources/members.md`, `examples/provider/provider.tf`, `examples/resources/claude-enterprise_spend_limit/resource.tf`, `examples/data-sources/claude-enterprise_members/data-source.tf`, `examples/full/main.tf`

**Interfaces:**
- Consumes: Task 9~10에서 확정된 스키마 (문서의 속성 표는 실제 스키마와 일치해야 함)
- Produces: Terraform Registry가 렌더링하는 `docs/` 트리, 실환경 검증(Task 14)이 사용하는 `examples/full/main.tf`

- [ ] **Step 1: `LICENSE` 다운로드 (MPL-2.0)**

Run: `curl -fsSL https://www.mozilla.org/media/MPL/2.0/index.txt -o LICENSE && head -3 LICENSE`
Expected: "Mozilla Public License Version 2.0" 출력

- [ ] **Step 2: `README.md` 작성 (영어)**

내용에 반드시 포함: (1) 첫 문단에 community provider disclaimer — "This is a community-maintained provider and is not an official Anthropic product."; (2) 요구 사항 — Claude Enterprise 조직, usage credits 활성화, `read:spend_limits`+`write:spend_limits` 스코프의 Admin API key; (3) Usage 예 (아래 examples/full과 동일 패턴); (4) 인증 — `ANTHROPIC_ADMIN_KEY` env 권장; (5) Development 섹션 — `make test` / `make testacc`(로컬 terraform 필요), dev_overrides 안내:

````markdown
## Local development

```hcl
# ~/.terraformrc (or a dedicated file via TF_CLI_CONFIG_FILE)
provider_installation {
  dev_overrides {
    "JeongJaeSoon/claude-enterprise" = "/Users/<you>/go/bin"
  }
  direct {}
}
```

Run `go install .`, then `terraform plan` in any configuration that
references `JeongJaeSoon/claude-enterprise` — no `terraform init` needed
for the overridden provider.
````

- [ ] **Step 3: `docs/` 작성 (영어)**

`docs/index.md` — provider 소개, disclaimer, 인증(env 우선), `admin_api_key`/`base_url` 속성 표, rate limit 주의(조직당 60 req/min, 클라이언트가 기본 50 req/min으로 스로틀+429 재시도).

`docs/resources/spend_limit.md` — 스키마 표(user_id/user_email/amount/period/id/currency, Task 9의 MarkdownDescription과 동일 문구), upsert-adopt 동작, 교체 규칙(user 변경·period 변경 = replace, amount = in-place), import 두 형식:

```
terraform import claude-enterprise_spend_limit.x spl_01AbC...
terraform import claude-enterprise_spend_limit.x user_01AbC...
```

`docs/data-sources/members.md` — by_email/by_user_id 스키마 표, deleted 제외, `effective_amount` null=무제한 의미, 대량 조직에서 전체 페이지 순회 비용 주의.

- [ ] **Step 4: `examples/` 작성**

`examples/provider/provider.tf`:

```hcl
terraform {
  required_providers {
    claude-enterprise = {
      source  = "JeongJaeSoon/claude-enterprise"
      version = "~> 0.1"
    }
  }
}

# The admin API key is read from the ANTHROPIC_ADMIN_KEY environment
# variable; avoid putting it in configuration.
provider "claude-enterprise" {}
```

`examples/resources/claude-enterprise_spend_limit/resource.tf`:

```hcl
# Address a member directly by email; the provider resolves the user id.
resource "claude-enterprise_spend_limit" "alice" {
  user_email = "alice@example.com"
  amount     = "75000" # 750.00 USD in cents
}

# Or pin by user id.
resource "claude-enterprise_spend_limit" "bob" {
  user_id = "user_01AbCdEfGh"
  amount  = "0" # included plan usage only
}
```

`examples/data-sources/claude-enterprise_members/data-source.tf`:

```hcl
data "claude-enterprise_members" "all" {}

output "alice_user_id" {
  value = data.claude-enterprise_members.all.by_email["alice@example.com"].user_id
}
```

`examples/full/main.tf` (실환경 검증용 — email 맵 기반 실사용 패턴):

```hcl
terraform {
  required_providers {
    claude-enterprise = {
      source  = "JeongJaeSoon/claude-enterprise"
      version = "~> 0.1"
    }
  }
}

provider "claude-enterprise" {}

variable "spend_limit_overrides" {
  description = "Per-member spend limit overrides in cents, keyed by email."
  type        = map(string)
  default     = {}
}

data "claude-enterprise_members" "all" {}

resource "claude-enterprise_spend_limit" "override" {
  for_each   = var.spend_limit_overrides
  user_email = each.key
  amount     = each.value
}

output "members_effective" {
  description = "Current effective limits for every member (read-only smoke check)."
  value = {
    for email, m in data.claude-enterprise_members.all.by_email :
    email => { amount = m.effective_amount, source = m.source_type, spent = m.period_to_date_spend }
  }
}
```

- [ ] **Step 5: 검증 — terraform fmt + 문서·스키마 일치 확인**

Run: `terraform fmt -recursive -check examples/ && go build ./...`
Expected: 출력 없음(fmt 통과). 문서의 속성 이름을 `internal/provider/resource_spend_limit.go`/`data_source_members.go`의 tfsdk 태그와 대조해 오탈자 없는지 확인.

- [ ] **Step 6: Commit**

```bash
git add README.md LICENSE docs/ examples/
git commit -m "docs: add registry docs, usage examples, and MPL-2.0 license"
```

---

### Task 12: CI 워크플로 (`ci.yml`)

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `Makefile`의 `testacc` 타깃, go.mod의 Go 버전
- Produces: PR/푸시 시 build + lint + 전체 테스트(acceptance 포함)를 돌리는 파이프라인

- [ ] **Step 1: `.github/workflows/ci.yml` 작성**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Build
        run: go build ./...
      - name: Check formatting
        run: test -z "$(gofmt -l .)"
      - name: Lint
        uses: golangci/golangci-lint-action@v6

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_wrapper: false
      - name: Unit and acceptance tests (mock API)
        run: go test ./... -count=1 -v
        env:
          TF_ACC: "1"
```

- [ ] **Step 2: 로컬에서 동등 검증**

Run: `go build ./... && test -z "$(gofmt -l .)" && TF_ACC=1 go test ./... -count=1`
Expected: 전부 통과. (golangci-lint가 로컬에 있으면 `golangci-lint run`도 실행; 없으면 CI에 위임)

- [ ] **Step 3: Commit**

```bash
git add .github/
git commit -m "ci: add build, lint, and acceptance test workflow"
```

---

### Task 13: 릴리스 파이프라인 (GoReleaser, release.yml, registry manifest, RELEASING.md)

**Files:**
- Create: `.goreleaser.yml`, `.github/workflows/release.yml`, `terraform-registry-manifest.json`, `docs/RELEASING.md`

**Interfaces:**
- Consumes: `main.go`의 `var version` (ldflags 주입 지점)
- Produces: `v*` tag 푸시 → GPG 서명된 멀티플랫폼 릴리스. Registry publish에 필요한 아티팩트 형식(zip + SHA256SUMS + sig + manifest)

- [ ] **Step 1: `terraform-registry-manifest.json` 작성**

```json
{
  "version": 1,
  "metadata": {
    "protocol_versions": ["6.0"]
  }
}
```

- [ ] **Step 2: `.goreleaser.yml` 작성** (HashiCorp provider 스캐폴딩 표준 형식)

```yaml
version: 2
project_name: terraform-provider-claude-enterprise
before:
  hooks:
    - go mod tidy
builds:
  - env:
      - CGO_ENABLED=0
    mod_timestamp: '{{ .CommitTimestamp }}'
    flags:
      - -trimpath
    ldflags:
      - '-s -w -X main.version={{.Version}}'
    goos: [freebsd, windows, linux, darwin]
    goarch: [amd64, '386', arm, arm64]
    ignore:
      - goos: darwin
        goarch: '386'
    binary: '{{ .ProjectName }}_v{{ .Version }}'
archives:
  - formats: [zip]
    name_template: '{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}'
checksum:
  extra_files:
    - glob: 'terraform-registry-manifest.json'
      name_template: '{{ .ProjectName }}_{{ .Version }}_manifest.json'
  name_template: '{{ .ProjectName }}_{{ .Version }}_SHA256SUMS'
  algorithm: sha256
signs:
  - artifacts: checksum
    args: ['--batch', '--local-user', '{{ .Env.GPG_FINGERPRINT }}', '--output', '${signature}', '--detach-sign', '${artifact}']
release:
  extra_files:
    - glob: 'terraform-registry-manifest.json'
      name_template: '{{ .ProjectName }}_{{ .Version }}_manifest.json'
changelog:
  disable: true
```

- [ ] **Step 3: `.github/workflows/release.yml` 작성**

```yaml
name: Release

on:
  push:
    tags: ['v*']

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Import GPG key
        id: import_gpg
        uses: crazy-max/ghaction-import-gpg@v6
        with:
          gpg_private_key: ${{ secrets.GPG_PRIVATE_KEY }}
          passphrase: ${{ secrets.PASSPHRASE }}
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          args: release --clean
        env:
          GPG_FINGERPRINT: ${{ steps.import_gpg.outputs.fingerprint }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 4: `docs/RELEASING.md` 작성 (영어)** — 다음 절차를 문서화:

1. GPG 키 생성: `gpg --full-generate-key` (RSA 4096, 이메일은 GitHub 계정과 일치) → `gpg --armor --export-secret-keys <key-id>`를 repo secret `GPG_PRIVATE_KEY`로, passphrase를 `PASSPHRASE`로 등록
2. 공개키 `gpg --armor --export <key-id>`를 https://registry.terraform.io/settings/gpg-keys 에 네임스페이스 `JeongJaeSoon`으로 등록
3. `git tag v0.1.0 && git push origin v0.1.0` → release.yml이 서명된 릴리스 생성
4. https://registry.terraform.io/publish/provider 에서 GitHub repo 연결(최초 1회) → 이후 태그는 자동 인식
5. 사전 조건: repo가 public이고 이름이 `terraform-provider-claude-enterprise`일 것

- [ ] **Step 5: GoReleaser 설정 검증 (릴리스 없이)**

Run: `command -v goreleaser >/dev/null && goreleaser check || echo "goreleaser not installed; config check deferred to first release"`
Expected: `goreleaser check` 통과 또는 설치 안내 메시지 (설치되어 있으면 반드시 통과해야 함; `brew install goreleaser`로 설치 가능)

- [ ] **Step 6: Commit**

```bash
git add .goreleaser.yml .github/workflows/release.yml terraform-registry-manifest.json docs/RELEASING.md
git commit -m "build: add goreleaser release pipeline with registry manifest"
```

---

### Task 14: 로컬 dev_overrides 스모크 테스트 (mock 대상 end-to-end)

실환경 검증(스펙 §11) 전에, **배포 경로가 아닌 실제 terraform CLI + 로컬 빌드 바이너리** 조합이 동작하는지 mock으로 확인한다. 여기까지 통과하면 남는 변수는 실제 API뿐이다.

**Files:**
- Create: `examples/smoke/main.tf` (gitignore된 상태 파일 생성 주의 — `.gitignore`가 이미 커버)

**Interfaces:**
- Consumes: `make install`(Task 1), `examples/full/main.tf` 패턴, `testutil` mock을 standalone으로 띄우는 임시 고 프로그램

- [ ] **Step 1: mock 서버를 standalone으로 띄우는 임시 스크립트 작성**

`/private/tmp/claude-501/-Users-dev-soon-workspace-project/a2e88ed0-a769-42d6-9e40-9c5c12eb064f/scratchpad/mockmain/main.go` (프로젝트 밖, 커밋하지 않음):

```go
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func main() {
	m := testutil.NewMockAdminAPI("smoke-key", []testutil.Member{
		{UserID: "user_01A", Email: "alice@example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
	})
	m.SetPageSize(1)
	fmt.Println(m.URL())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
```

주의: scratchpad에서 모듈을 참조하려면 `go.mod`에 replace가 필요하다. 간단히 하기 위해 이 파일을 프로젝트 안 `tools/mockserver/main.go`로 두고 커밋하는 것도 허용 (그 경우 `//go:build ignore` 없이 일반 패키지로, README에 용도 한 줄 추가).

- [ ] **Step 2: `examples/smoke/main.tf` 작성**

```hcl
terraform {
  required_providers {
    claude-enterprise = {
      source = "JeongJaeSoon/claude-enterprise"
    }
  }
}

provider "claude-enterprise" {
  base_url = var.mock_url
}

variable "mock_url" { type = string }

data "claude-enterprise_members" "all" {}

resource "claude-enterprise_spend_limit" "alice" {
  user_email = "alice@example.com"
  amount     = "50000"
}

output "members" {
  value = keys(data.claude-enterprise_members.all.by_email)
}
```

- [ ] **Step 3: dev_overrides로 plan/apply/destroy 실행**

```bash
make install   # $HOME/go/bin에 설치 (go env GOBIN 확인)

cat > /private/tmp/claude-501/-Users-dev-soon-workspace-project/a2e88ed0-a769-42d6-9e40-9c5c12eb064f/scratchpad/dev.tfrc <<'EOF'
provider_installation {
  dev_overrides {
    "JeongJaeSoon/claude-enterprise" = "REPLACE_WITH_GOBIN"
  }
  direct {}
}
EOF
# REPLACE_WITH_GOBIN을 $(go env GOPATH)/bin 실제 경로로 치환

# mock 서버 기동(백그라운드) 후 출력된 URL 확보
cd examples/smoke
TF_CLI_CONFIG_FILE=<scratchpad>/dev.tfrc ANTHROPIC_ADMIN_KEY=smoke-key \
  terraform apply -auto-approve -var mock_url=<MOCK_URL>
TF_CLI_CONFIG_FILE=<scratchpad>/dev.tfrc ANTHROPIC_ADMIN_KEY=smoke-key \
  terraform destroy -auto-approve -var mock_url=<MOCK_URL>
```

Expected: apply — `members` output에 alice/bob, 리소스 1개 생성. destroy — 정상 삭제. dev_overrides 경고("Provider development overrides are in effect")는 정상.

- [ ] **Step 4: Commit** (smoke 예제와 tools를 커밋에 포함한 경우)

```bash
git add examples/smoke/ tools/ 2>/dev/null || true
git commit -m "test: add local dev-override smoke configuration" || echo "nothing to commit"
```

---

## 구현 완료 후: 실환경 검증 절차 (스펙 §11 — 사용자와 함께 수행)

이 부분은 코드 Task가 아니라 사용자에게 전달할 운영 절차다. 구현 완료 보고 시 아래를 단계별 체크리스트로 안내한다:

1. **키 준비**: claude.ai에서 `read:spend_limits` + `write:spend_limits` 스코프의 Admin API key 발급 → `export ANTHROPIC_ADMIN_KEY=sk-ant-admin...`
2. **Read-only 검증**: `examples/full`에서 dev_overrides + 빈 `spend_limit_overrides`로 `terraform plan` → `members_effective` output으로 전 멤버 실효 한도가 올바른지 눈으로 확인 (쓰기 0회)
3. **소액 write 검증**: `spend_limit_overrides = { "<본인 email>" = "1000" }` (10 USD)로 apply → claude.ai 콘솔에서 확인 → `"2000"`으로 변경 후 재apply(in-place update 확인) → `terraform destroy`로 상속 복귀 확인
4. **Import 검증**: 콘솔에서 만든 override를 `terraform import 'claude-enterprise_spend_limit.override["<email>"]' user_01...`로 흡수 → `terraform plan`이 no-op인지 확인
5. **불일치 발견 시**: 실제 API 응답이 mock과 다른 부분(필드명·id 안정성·에러 형식)을 스펙에 기록하고 mock을 실제에 맞춘 뒤 회귀 테스트 추가
6. **org 실적용**: GitHub repo 생성·푸시 → 조직의 TF 구성에서 이 provider 사용, GitHub Actions에서 PR=plan / merge=apply, `ANTHROPIC_ADMIN_KEY`는 Actions secret (state backend는 조직 표준을 따름)
7. **Registry publish**: 검증 통과 후 `docs/RELEASING.md` 절차로 v0.1.0 태그 → Registry 등록

## 실행 주의사항

- **하이픈 타입명 리스크**: Task 7 Step 4가 최초 검증 지점. terraform이 `claude-enterprise_members`를 거부하면 작업을 멈추고 사용자에게 TypeName 대안(`claudeenterprise`)을 보고할 것
- **terraform-plugin-framework API 드리프트**: 플랜의 framework 코드는 v1.x 기준. 시그니처가 다르면 `go doc github.com/hashicorp/terraform-plugin-framework/...`로 현재 버전을 확인해 맞추되, 동작 계약(스키마·plan 수정·교체 규칙)은 유지할 것
- **`TestAccSpendLimitImportByUserID`의 첫 스텝 import**: terraform-plugin-testing 버전에 따라 사전 apply 없는 import 스텝이 거부될 수 있음. 그 경우 step 1에서 `user_01B`용 리소스를 apply하고 step 2에서 `user_01A`를 import하는 형태로 조정
- Create/Update에서 state를 API 응답 값으로 채우므로, 실제 API가 요청 값을 정규화해 되돌려주면 "inconsistent result after apply"가 날 수 있다. mock은 echo하므로 테스트로는 안 잡힌다 — 실환경 검증 3단계에서 확인할 것

## Self-Review 결과

- **Spec coverage**: §4 클라이언트(Task 2–4), §5 provider 설정(Task 6), §6 리소스(Task 9–10), §7 데이터소스(Task 7), resolver(Task 8), §9 테스트(Task 5 + 각 Task), §10 CI/릴리스/docs(Task 11–13), §11 실환경 검증(Task 14 + 절차 섹션) — 전 항목 커버
- **Type consistency**: `providerData{Client, Resolver}`는 Task 6에서 Client만 → Task 8에서 Resolver 추가로 명시. `testutil.MockAdminAPI` 메서드(`URL/Close/SeedOverride/OverrideCount/EffectiveRequestCount/SetPageSize/DeleteAllOverrides`)는 Task 5 정의 + Task 9에서 `DeleteAllOverrides` 추가로 명시. `client` 타입 시그니처는 Task 4 Interfaces 블록과 이후 사용처 일치
- **Placeholder scan**: 코드 스텝은 전부 실제 코드 포함. Task 11의 문서는 포함 항목을 열거(문서 산문은 구현자가 작성)






