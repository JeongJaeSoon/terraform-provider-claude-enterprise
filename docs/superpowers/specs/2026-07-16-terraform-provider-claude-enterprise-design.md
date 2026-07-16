# terraform-provider-claude-enterprise 설계 사양

- 날짜: 2026-07-16
- 상태: 승인됨 (브레인스토밍 완료, 구현 계획 수립 전)
- 원본 초안: 사용자 제공 설계 메모 (2026-07-16 대화)

Claude Enterprise 조직의 멤버별 사용량 제한(spend limits)을 Terraform으로 선언적으로
관리하는 커뮤니티 provider. 사용량 제한은 SCIM/IdP로 연동되지 않으므로 Claude
Enterprise Admin API를 직접 호출하며, diff가 있을 때만 API를 호출한다. OSS로 공개한다.

## 1. 확정된 결정 사항

초안의 미결정 사항(원본 10장)에 대한 결정:

| 항목 | 결정 | 근거 |
|---|---|---|
| 인증 방식 | Admin API key (`x-api-key`) 단일 | GitHub Actions에서 plan/apply 시 Actions secret으로 주입. WIF는 v0.2+ |
| 리소스 입력 | `user_id` / `user_email` 둘 다 지원 (정확히 하나) | 사용자의 기존 email 기반 멤버 맵을 그대로 활용 |
| 네임스페이스 | `JeongJaeSoon/claude-enterprise` (GitHub 개인 계정) | 리포: `github.com/JeongJaeSoon/terraform-provider-claude-enterprise` |
| 배포 | 로컬 dev_overrides로 검증 → 공식 Terraform Registry publish까지 v0.1 범위에 포함 | GoReleaser + GPG 서명 + GitHub Actions release |
| tier/group 기본값 | 콘솔(claude.ai) 소관으로 유지 | API가 per-user override만 쓰기 지원 |
| 검증 | mock 기반 acceptance test + 실환경(실제 org, Admin API key 보유) 검증 | 검증 후 org에 실제 적용 |

## 2. 대상 API — Claude Enterprise Admin API (Spend Limits)

### 인증

- Base URL: `https://api.anthropic.com`
- 헤더: `x-api-key: sk-ant-admin...` (claude.ai에서 발급하는 스코프 지정 Admin API key)
- 헤더: `anthropic-version: 2023-06-01`
- 스코프: 읽기 `read:spend_limits` / 쓰기 `write:spend_limits`

### 엔드포인트

- `GET /v1/organizations/spend_limits/effective` — 멤버별 실효 한도, `source`(상속원,
  객체 `{type: user|seat_tier|rbac_group|organization, ...}` — open set),
  `period_to_date_spend`, `spend_limit_id`, `actor`(`user_id`, `email_address`, `name`,
  `deleted`). `user_ids[]` 필터와 `limit` 파라미터 지원. opaque cursor
  페이지네이션(`next_page` → `page`, `next_page`가 `null`이면 끝), 시퀀스 중간에 쿼리
  파라미터 변경 시 400.
- `GET /v1/organizations/spend_limits/{spend_limit_id}` — 개별 한도 조회.
- `POST /v1/organizations/spend_limits` — per-user override upsert (`(scope, period)`가 키).
- `DELETE /v1/organizations/spend_limits/{spend_limit_id}` — override 삭제, 상속값 복귀.

증액 요청(approve/deny) 계열은 IaC 관리 대상에서 제외.

### 제약

- 쓰기는 `scope.type: "user"`(per-user override)만 가능. seat-tier / group / organization
  기본값은 claude.ai 콘솔에서만 설정.
- 쓰기 시 사용자 식별자는 `user_id`(`user_01Ab...`)만 허용. email → user_id 해소는
  `/effective` 응답으로 수행.
- 금액은 조직 청구통화 최소 단위(cents)의 문자열. `"50000"` = 500.00 USD. `null` =
  무제한, `"0"` = 플랜 포함분만.
- `period`는 현재 `monthly`만 지원하나 open set으로 취급. 스펜드 리밋의 키는
  `(scope, period)` 쌍.
- 레이트리밋: 조직당 60 req/min, 초과 시 429.
- 식별자 형식: `spend_limit_id`는 `spl_01...`, 사용자는 `user_01...`. `currency`는
  `"USD"` 같은 대문자 코드. `POST` 응답은 `{type: "spend_limit", id, created_at,
  updated_at, scope, amount, currency, period}`.
- `GET /{spend_limit_id}` 응답(설정된 row)에는 `source`와 `period_to_date_spend`가
  없다 — 이 둘은 `/effective` 행에서만 온다.

## 3. 아키텍처

- Go 1.26 + `terraform-plugin-framework` (+ 테스트는 `terraform-plugin-testing`)
- 디렉토리 구조:

```
terraform-provider-claude-enterprise/
├── main.go                        # provider server 엔트리포인트
├── internal/
│   ├── client/                    # 얇은 Admin API 클라이언트 (net/http)
│   │   ├── client.go              # 인증 헤더 주입, 요청 실행
│   │   ├── retry.go               # 429 Retry-After / 지수백오프, rate limiter
│   │   ├── spend_limits.go        # 엔드포인트별 메서드 + 페이지네이션 헬퍼
│   │   └── *_test.go
│   └── provider/
│       ├── provider.go            # provider 스키마·Configure
│       ├── resource_spend_limit.go
│       ├── data_source_members.go
│       ├── resolver.go            # email→user_id 해소 캐시 (apply 1회 내 공유)
│       ├── testutil/mockserver.go # httptest 기반 mock Admin API
│       └── *_test.go
├── examples/                      # Registry 문서용 + 실사용 예시
├── docs/                          # tfplugindocs 생성 문서
├── .github/workflows/             # ci.yml (build/lint/test), release.yml (GoReleaser)
├── .goreleaser.yml
└── terraform-registry-manifest.json  # protocol 6.0
```

## 4. API 클라이언트 (`internal/client`)

- 모든 요청에 `x-api-key`, `anthropic-version: 2023-06-01` 주입.
- 429 응답: `Retry-After` 헤더 존중, 없으면 지수백오프(+jitter). 최대 재시도 횟수 설정
  가능(기본 5회).
- 클라이언트 측 rate limiter(token bucket)로 조직당 60 req/min 이내 유지. Terraform의
  기본 병렬도 10과 대량 초기 apply를 견디도록 기본값 보수적으로 설정(예: 50 req/min).
- 페이지네이션 헬퍼: `next_page` cursor를 `page` 파라미터로 전달, 순회 중 다른 파라미터
  불변 보장.
- 금액은 `*string`(cents)으로 취급, 부동소수점 사용 금지. `null` = 무제한.
- `source` / `period` / `scope.type`은 open set — 모르는 값은 에러 없이 통과.

## 5. Provider 설정 스키마

```hcl
terraform {
  required_providers {
    claude-enterprise = {
      source  = "JeongJaeSoon/claude-enterprise"
      version = "~> 0.1"
    }
  }
}

provider "claude-enterprise" {
  # admin_api_key = env ANTHROPIC_ADMIN_KEY (권장: env로만 주입)
  # base_url      = "https://api.anthropic.com" (기본값, 테스트·프록시용 override)
}
```

- `admin_api_key`: Optional + Sensitive. 미지정 시 `ANTHROPIC_ADMIN_KEY` env에서 읽음.
  둘 다 없으면 Configure에서 명확한 에러. tfvars/state에 평문 저장 금지 안내.
- `base_url`: Optional. 기본 `https://api.anthropic.com`. acceptance test에서 mock
  서버 주소로 override.

## 6. 리소스 `claude-enterprise_spend_limit`

한 사용자의 per-user override 1건을 관리한다.

### 스키마

| 속성 | 타입 | 구분 | 설명 |
|---|---|---|---|
| `user_id` | string | Optional + Computed | API의 실제 키. `user_email`과 정확히 하나만 지정 |
| `user_email` | string | Optional | 지정 시 provider가 `/effective`로 user_id 해소 |
| `amount` | string | Required | 최소 단위(cents) 문자열. `"0"` = 포함분만 |
| `period` | string | Optional (기본 `"monthly"`) | open set |
| `id` | string | Computed | `spend_limit_id` (`spl_01...`) |
| `currency` | string | Computed | 조직 청구통화 (예: `"USD"`) |

`source`와 `period_to_date_spend`는 `GET /{id}` 응답에 없으므로 리소스에서는 제외하고
members 데이터소스에서만 노출한다(리소스 Read를 O(1)로 유지).

- `user_id` XOR `user_email`: `resourcevalidator.ExactlyOneOf`로 강제.
- **email 해소는 plan 시점(ModifyPlan)에 수행**: `user_email`이 known이면 resolver로
  user_id를 해소해 planned `user_id`에 반영하고, state의 user_id와 다르면
  `RequiresReplace`를 마킹한다. 따라서 재생성 판정은 항상 `user_id` 기준이고,
  `user_email` 속성 자체는 replace를 강제하지 않는다(user_id로 import한 리소스에
  나중에 `user_email` 설정을 붙여도 동일 사용자면 replace가 발생하지 않음).
- `user_id` 직접 지정 변경과 `period` 변경은 RequiresReplace. `amount` 변경은 in-place
  update(upsert).
- `user_email` 사용 시 user_id가 바뀌면(탈퇴 후 재가입 등) 리소스 재생성 — 의도된 동작.
- 기존 리소스에서 `user_email`이 plan 시점에 unknown이면 명확한 에러(정체성 변경을
  감지할 수 없으므로). 신규 리소스는 unknown 허용(apply 시 해소).

### CRUD 매핑

- **Create / Update**: `POST /v1/organizations/spend_limits` (upsert). 응답에서 `id`,
  `currency`, `source` 등 computed 채움.
- **Read**: 저장된 `id`로 `GET /{spend_limit_id}`. 404 또는 `scope.type != "user"`면
  override 소멸로 판단하고 state에서 제거. 전체 목록 순회 없음 → refresh 비용
  O(1)/리소스.
- **Delete**: `DELETE /{spend_limit_id}`. 404는 이미 삭제된 것으로 간주(성공 처리).
- **Import**: `spend_limit_id` 직접 지정 또는 `user_id`(`user_` 접두사로 판별 →
  `/effective?user_ids[]=<id>` 1회 조회, `source.type == "user"`인 행의
  `spend_limit_id` 사용; override가 없으면 명확한 에러) 지원.

### email → user_id 해소 (`resolver.go`)

- provider 인스턴스 레벨 캐시: 최초 해소 요청 시 `/effective` 전체를 1회 순회해
  email → user_id 맵을 구축하고 plan/apply 세션 동안 재사용(mutex로 동시 해소 직렬화).
  리소스가 100개여도 목록 순회는 1번 → 레이트리밋 보호.
- `actor.deleted == true`인 행은 맵에서 제외.
- 해소 실패(email이 조직에 없음)는 명확한 에러로 plan/apply 중단.
- email 비교는 대소문자 무시(normalize to lower).

## 7. 데이터소스 `claude-enterprise_members`

- `/effective`를 전체 페이지네이션 순회해 조직 멤버 목록 노출. `actor.deleted`인
  행은 제외.
- 출력: `by_email`, `by_user_id` — 각각 map of object `{ user_id, email, name,
  effective_amount (null = 무제한), currency, period, source_type, spend_limit_id,
  period_to_date_spend }`.
- 용도: email 기준 설정에서 user_id 해소(리소스의 `user_email` 대신 명시적 경유를
  원할 때), 현재 실효 한도 확인.

### 사용 예

```hcl
# 방법 A: user_email 직접 (권장 — 기존 email 기반 멤버 맵 재사용)
locals {
  overrides = {
    "alice@example.com" = "75000"
    "bob@example.com"   = "0"
  }
}

resource "claude-enterprise_spend_limit" "override" {
  for_each   = local.overrides
  user_email = each.key
  amount     = each.value
}

# 방법 B: data source 경유 (plan 시점에 user_id 확인 가능)
data "claude-enterprise_members" "all" {}

resource "claude-enterprise_spend_limit" "override_b" {
  for_each = local.overrides
  user_id  = data.claude-enterprise_members.all.by_email[each.key].user_id
  amount   = each.value
}
```

## 8. 에러 처리·신뢰성

- 401/403: key 미설정·스코프 부족을 구분해 진단 메시지 제공(필요 스코프 명시).
- 429: 클라이언트 재시도로 흡수. 재시도 소진 시 사용자에게 rate limit 초과 에러.
- `period_to_date_spend`는 일시적으로 `"0"`이 될 수 있으므로 members 데이터소스의
  정보용 출력으로만 두고 리소스 drift 판정에는 사용하지 않음.
- 페이지네이션 중간 실패는 전체 재시도(부분 결과 사용 금지).

## 9. 테스트 전략

- **Unit test** (`internal/client`): 헤더 주입, 429 재시도, 페이지네이션, 에러 매핑.
- **Acceptance test** (`internal/provider`): `httptest` 기반 mock Admin API 서버
  (`testutil/mockserver.go`)가 in-memory로 spend limits CRUD + `/effective` 페이지네이션
  구현. `base_url`을 mock으로 지정, `TF_ACC=1`로 전체 라이프사이클(Create → Read →
  Update → Delete → Import) 검증. 실제 key 불필요 → CI에서 항상 실행.
- **실환경 검증**: 별도 절차(11장).

## 10. CI / 릴리스

- `ci.yml` (PR/push): `go build`, `golangci-lint`, `go test ./...`(acceptance 포함,
  mock 기반 — `hashicorp/setup-terraform`으로 terraform CLI 설치).
- 문서는 v0.1에서는 수동 작성(`docs/index.md`, `docs/resources/`, `docs/data-sources/`
  — Registry가 그대로 렌더링). `tfplugindocs` 자동화는 v0.2+.
- `release.yml` (tag `v*` push): GoReleaser로 멀티플랫폼 빌드 + GPG 서명 +
  `terraform-registry-manifest.json` 포함 아카이브 생성 → GitHub Release.
- Registry 등록(수동 1회): GPG 공개키를 Terraform Registry에 등록 → provider publish.
  절차는 `docs/RELEASING.md`에 기록.
- 라이선스: MPL-2.0 (Terraform provider 생태계 관례).
- README에 비공식(community) provider임을 명시.

## 11. 실환경 검증 계획 (구현 완료 후 수행)

1. **로컬 dev override**: `~/.terraformrc`에 `dev_overrides`로
   `JeongJaeSoon/claude-enterprise` → 로컬 빌드 바이너리 매핑.
2. **Read-only 검증**: `ANTHROPIC_ADMIN_KEY` 설정 → `claude-enterprise_members` data
   source만 있는 구성으로 plan (쓰기 0회로 인증·페이지네이션·스키마 검증).
3. **소액 write 검증**: 본인 계정 1명에 소액 override apply → claude.ai 콘솔과
   `/effective` 응답으로 교차 확인 → `amount` 변경 후 재apply(update 경로) →
   `terraform destroy`로 상속값 복귀 확인.
4. **Import 검증**: 콘솔 또는 API로 만든 override를 `terraform import`로 흡수.
5. **org 실적용**: 기존 email 기반 멤버 맵과 결합한 실제 `.tf` 작성 → GitHub Actions에서
   PR 시 plan, merge 시 apply 워크플로 구성 (`ANTHROPIC_ADMIN_KEY`는 Actions secret,
   state backend는 별도 결정).
6. **Registry publish**: 실환경 검증 통과 후 v0.1.0 tag → release → Registry 등록.

## 12. 로드맵

- v0.1: 위 범위 전체 (spend limits resource + members data source + 릴리스 파이프라인).
- v0.2+: Admin API의 members / invites / groups(Enterprise beta) 확장, WIF(OAuth) 인증.

## 참고 (출처)

- Admin API: https://platform.claude.com/docs/en/manage-claude/admin-api
- Spend Limits API: https://platform.claude.com/docs/en/manage-claude/spend-limits-api
- Rate Limits API: https://platform.claude.com/docs/en/manage-claude/rate-limits-api
- Enterprise Admin API reference:
  https://support.claude.com/en/articles/15330651-claude-enterprise-admin-api-reference-guide
