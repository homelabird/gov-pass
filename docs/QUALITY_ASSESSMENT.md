# 프로젝트 품질 평가

_평가 기준 시점: 2026-03-07_

이 문서는 현재 `gov-pass` 저장소를 기준으로 구조, 문서화, 테스트, 보안 관점에서
프로젝트 전반의 품질을 요약 평가한 결과다. 수치는 별도 명시가 없는 한
`cmd/`와 `internal/`의 first-party 코드 기준으로 해석했다.

## 평가 방법

- 문서 검토: `README.md`, `docs/INDEX.md`, `docs/DESIGN*.md`, `SECURITY.md`,
  `CONTRIBUTING.md`
- 기본 검증 실행:
  - `make test`
  - `make vet`
  - `make build`
  - `go test -cover ./...`
- 정량 지표 수집:
  - Go 소스 파일 수
  - 테스트 파일 및 테스트 함수 수
  - first-party 기준 라인 수
  - TODO/FIXME/XXX/BUG 토큰 존재 여부

## 요약 평가

| 항목 | 평가 | 근거 |
| --- | --- | --- |
| 아키텍처/모듈화 | 좋음 | `cmd/`, `internal/`, 플랫폼별 build tag 분리가 명확함 |
| 테스트 기반 | 보통 이상 | 핵심 패키지 테스트가 존재하고 전체 검증 명령이 안정적으로 통과함 |
| 문서화 | 좋음 | 설치/설계/패키징/보안 문서가 분리되어 있고 진입점이 정리되어 있음 |
| 보안/운영 안정성 | 좋음 | `SECURITY.md`와 fail-open, bounded shutdown 지침이 명문화되어 있음 |
| 이식성/플랫폼 대응 | 보통 이상 | Windows/Linux/BSD 경로가 분리되어 있으나 Linux/BSD는 상대적으로 성숙도 편차가 있음 |

**종합 판단:**  
현재 프로젝트는 **구조적 완성도와 운영 관점의 설계 품질이 높은 편**이다. 특히
패킷 처리처럼 실패 비용이 큰 영역에서 fail-open, bounded shutdown, resource cap
같은 운영 원칙이 코드와 문서 양쪽에 반영되어 있다. 반면 품질 지표를 더
객관적으로 유지하려면 커버리지 가시화와 일부 저커버리지 패키지 보강이 다음
단계의 우선 과제다.

## 정량 지표

### 검증 결과

- `make test`: 통과
- `make vet`: 통과
- `make build`: 통과
- `go test -cover ./...`: 통과

### 코드 및 테스트 규모

- first-party Go 파일: **64개**
- first-party 테스트 파일: **26개**
- first-party 테스트 함수: **105개**
- first-party 총 라인 수(`cmd/` + `internal/`): **11,407 lines**
- `cmd/`, `internal/`, 주요 문서에서 TODO/FIXME/XXX/BUG 토큰: **0개**

### 패키지별 커버리지

| 패키지 | 커버리지 |
| --- | ---: |
| `cmd/gov-pass-tui` | 26.7% |
| `cmd/splitter` | 42.5% |
| `internal/adapter` | 6.8% |
| `internal/driver` | 94.1% |
| `internal/engine` | 35.1% |
| `internal/flow` | 93.9% |
| `internal/packet` | 58.0% |
| `internal/reassembly` | 80.6% |
| `internal/safecast` | 73.3% |
| `internal/tls` | 78.9% |

## 강점

### 1. 구조 분리가 명확하다

- 실행 진입점은 `cmd/` 아래에, 핵심 로직은 `internal/` 아래에 정리되어 있다.
- Windows/Linux/FreeBSD 전용 경로가 build tag와 패키지 분리로 표현되어 있어
  공통 코드 오염이 적다.
- `docs/DESIGN_COMMON.md`와 플랫폼별 설계 문서가 역할을 잘 분리한다.

### 2. 운영 안정성을 염두에 둔 설계가 잘 드러난다

- 공통 설계 문서에 fail-open, bounded shutdown, packet cap 원칙이 직접 기술되어
  있다.
- `CONTRIBUTING.md`에도 동일한 엔지니어링 가이드가 반복되어 있어 유지보수 시
  기준이 흔들릴 가능성이 낮다.
- 네트워크 저수준 도구 특성상 매우 중요한 "멈춤 시 안전성"이 이미 설계 원칙으로
  관리되고 있다.

### 3. 보안 문서화가 충분하다

- `SECURITY.md`가 권한 모델, 외부 다운로드 검증, 규칙 관리 범위, 운영 지침을
  상세히 다룬다.
- Windows 서비스 경로, Linux rule/offload 관리, BSD divert 운용 범위가 모두
  분리 서술되어 있다.
- 고권한 프로그램임에도 운영상 주의점이 명확히 정리된 점은 품질 측면에서 강점이다.

### 4. 핵심 로직 테스트 밀도가 높은 영역이 있다

- `internal/driver`, `internal/flow`, `internal/reassembly`, `internal/tls`는 높은
  커버리지를 보인다.
- 이는 경로 처리, 상태 관리, 재조립, TLS 판별 같은 핵심 안정성 요소가 적어도
  단위 테스트 기준에서는 잘 방어되고 있음을 의미한다.

## 보완이 필요한 부분

### 1. 커버리지 편차가 크다

- `internal/adapter` 6.8%
- `cmd/gov-pass-tui` 26.7%
- `internal/engine` 35.1%
- `cmd/splitter` 42.5%

가장 핵심적인 런타임 접점에 해당하는 계층 일부의 커버리지가 낮다. 특히
`adapter`는 플랫폼/권한/환경 의존성이 크기 때문에 테스트가 어려운 영역이지만,
그만큼 회귀 위험도 큰 편이다.

### 2. 품질 지표가 CI에서 직접 관리되지는 않는다

- 현재 저장소는 테스트/빌드 진입점이 잘 정리되어 있지만, 커버리지 임계치나
  품질 리포트 산출 절차는 기본 문서에 명시되어 있지 않다.
- 품질 평가가 사람 중심 리뷰에 의존하기 쉬워 장기적으로는 추세 관찰이 어렵다.

### 3. 플랫폼 성숙도 차이가 존재한다

- README 기준 Windows는 Stable, Linux는 Beta, FreeBSD/pfSense는 Experimental이다.
- 아키텍처는 잘 준비되어 있지만, 실제 운영 성숙도는 플랫폼별로 동일하지 않다.
- 따라서 "프로젝트 전체 품질"을 평가할 때는 Windows 경로와 Linux/BSD 경로를
  동일 수준으로 간주하면 안 된다.

## 우선순위별 개선 권장 사항

1. **`internal/adapter`와 `internal/engine` 테스트 보강**
   - 모의 패킷/플로우 기반 테스트를 늘려 회귀 탐지력을 높이는 것이 가장 효과적이다.
2. **커버리지 산출을 CI에 포함**
   - `go test -cover ./...` 결과를 아티팩트나 요약으로 남기면 품질 추세 관리가 쉬워진다.
3. **플랫폼별 품질 기준 분리**
   - Stable/Beta/Experimental 상태에 맞춰 테스트 기대치와 릴리즈 체크리스트를
     구분하면 품질 메시지가 더 명확해진다.
4. **TUI 경로 테스트 확대**
   - 서비스 제어와 사용자 가시성이 연결되는 영역이라, 회귀 시 체감 영향이 크다.

## 결론

`gov-pass`는 저수준 네트워크 패킷 처리 프로젝트임에도 불구하고 **문서 구조,
운영 안전성 원칙, 보안 지침, 모듈 분리**가 잘 정리되어 있어 전반적인 품질이
양호하다. 특히 "무엇을 지원하는가"보다 "문제가 생겼을 때 어떻게 안전하게
동작을 유지할 것인가"에 대한 설계 흔적이 분명하다는 점이 강점이다.

다만 품질 수준을 한 단계 더 끌어올리려면, 이미 잘 갖춰진 구조 위에
**저커버리지 영역의 테스트 보강**과 **지표 자동화**를 덧붙이는 것이 가장 큰
효과를 낼 것으로 판단된다.
