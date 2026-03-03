# gov-pass 2주 실행 체크리스트 (CLI/GUI 완성도 향상)

기간: 10영업일 (2주)  
목표: CLI/GUI를 운영 가능한 `0.2` 품질선까지 끌어올리기

## 진행 로그

- 2026-03-03: `C1` 1차 구현 (Linux/FreeBSD `--config`), `C2` 1차 구현 (`--check`, `--check-json`), `C3` 1차 구현 (`--print-reloadability`)
- 2026-03-03: `G1` 1차 구현 (Linux TUI 권한 상승 fallback), `G2` 1차 구현 (Linux TUI `reload` 메뉴/액션)
- 2026-03-03: `C4` 1차 구현 (Windows 서비스 로그 size-based 로테이션 플래그/동작)
- 2026-03-03: `G3` 1차 구현 (Linux/Windows TUI 액션 진행/실패 상태 피드백)
- 2026-03-03: `G4` 1차 구현 (Linux TUI 릴리스 tarball 포함, one-touch 설치 옵션/문서 반영)
- 2026-03-03: `G5` 1차 구현 (공통 액션 정규화 계층 + 공통/Windows TUI 테스트 추가)
- 2026-03-03: `G2` 마무리 구현 (Linux TUI auto-start 토글 + Linux/Windows TUI 기능 매트릭스 문서화)
- 2026-03-03: `C5` 구현 (config 병합/reload 제약/preflight 테스트 추가) + `CI` 반영 (`verify_tui_tests` 자동 실행)
- 2026-03-03: GUI 품질 핫픽스 `1~4` 적용 (`--action` help 보정, TUI 액션 동시실행 게이트, Windows auto-start 상태 피드백, 공통 게이트 테스트)
- 2026-03-03: TUI 경고 대응 2단계 적용 (`임시`: `CGO_CFLAGS=-Wno-deprecated-declarations`, `근본`: 시스템 트레이 의존 제거)
- 2026-03-03: 순차 검증 `1->2->3->4` 수행 (`go test` 회귀 통과, 로컬 one-touch 설치/서비스 기동 확인, Linux TUI `reload-or-restart` 보정)

## 1) 최종 완료 기준 (Release Gate)

- [x] Linux/FreeBSD에서 설정 파일 기반 실행(`--config`)이 가능하다.
- [x] `splitter --check`(및 `--check-json`)로 사전 점검 자동화가 가능하다.
- [x] Windows 리로드 가능/불가 항목이 CLI/문서에서 명확히 보인다.
- [x] Windows 서비스 로그 로테이션 정책이 적용된다.
- [x] Linux TUI 권한 상승 실패 시 fallback 경로로 동작한다.
- [x] Linux TUI가 Windows TUI 대비 핵심 기능 패리티(reload, auto-start)를 갖춘다.
- [x] Linux TUI가 CI 릴리스 산출물/설치 경로에 포함된다.
- [x] CLI/GUI 핵심 경로 테스트가 CI에 포함된다.

## 2) 작업 단위 백로그 (ID / 난이도 / 선행작업)

- [x] `C1` Linux/FreeBSD `--config` 도입 (defaults < config < CLI override) / 난이도: 상 / 선행: 없음
- [x] `C2` `preflight/check` 모드 도입 (`--check`, `--check-json`) / 난이도: 중 / 선행: C1
- [x] `C3` Windows 리로드 가능/불가 항목 표준 출력 (`--print-reloadability`) / 난이도: 중 / 선행: 없음
- [x] `C4` Windows 서비스 로그 로테이션 (최소 size-based) / 난이도: 중 / 선행: 없음
- [x] `C5` CLI 테스트 보강 (config 병합, reload 제약, preflight) / 난이도: 상 / 선행: C1, C2, C3
- [x] `G1` Linux TUI 권한 상승 fallback 체인 구현 / 난이도: 상 / 선행: 없음
- [x] `G2` Linux TUI 기능 패리티 1차 (reload, open at login) / 난이도: 중 / 선행: G1
- [x] `G3` TUI 액션 피드백 강화 (실패/진행/즉시 갱신) / 난이도: 중 / 선행: G1
- [x] `G4` Linux TUI 패키징/릴리스 파이프라인 포함 / 난이도: 상 / 선행: G2
- [x] `G5` TUI 테스트 보강 (Linux/Windows 공통 액션 계층 포함) / 난이도: 상 / 선행: G2, G3

## 3) 일정 체크리스트 (Day-by-Day)

### Week 1

- [ ] `D1` C1 설계/스키마 확정
- [ ] `D1` G1 권한 상승 설계 확정
- [x] `D2` C1 구현 1차 완료
- [x] `D3` C1 마무리 및 문서 반영
- [x] `D3` G1 구현 완료
- [x] `D4` C2 구현 완료
- [x] `D4` G3 구현 완료
- [x] `D5` C3 구현 완료
- [x] `D5` C4 착수 및 초안 구현

### Week 2

- [x] `D6` C4 마무리
- [x] `D6` G2 구현 시작
- [x] `D7` G2 마무리
- [x] `D7` G4 착수
- [x] `D8` G4 마무리
- [x] `D8` C5 착수
- [x] `D9` C5 마무리
- [x] `D9` G5 마무리
- [x] `D9` CI 반영 완료
- [ ] `D10` 문서 정리/릴리스 체크리스트 점검/RC 준비

## 4) 작업별 완료 조건 (Definition of Done)

### C1 완료 조건

- [x] Linux/FreeBSD 엔트리에서 `--config` 플래그가 동작한다.
- [x] 설정 파일 파싱 실패 시 명확한 에러 메시지를 제공한다.
- [x] 우선순위(기본값 < 설정파일 < CLI 명시값)가 보장된다.
- [x] 샘플 설정 파일이 문서에 포함된다.

### C2 완료 조건

- [x] `--check` 실행 시 권한/외부툴/핵심 런타임 전제조건을 점검한다.
- [x] `--check-json` 결과를 CI나 자동화에서 파싱 가능하다.
- [x] check 모드는 시스템 변경(룰 설치/오프로딩 변경)을 하지 않는다.

### C3 완료 조건

- [x] 리로드 가능/재시작 필요 항목이 표준 출력으로 제공된다.
- [x] 문서의 Windows service reload 표와 내용이 일치한다.

### C4 완료 조건

- [x] 로그 파일 size 기준 로테이션이 동작한다.
- [x] 로테이션 파라미터(최대 크기/보관 개수)가 설정 가능하다.
- [x] 서비스 중단 없이 로테이션이 수행된다.

### C5 완료 조건

- [x] config 병합 우선순위 테스트가 추가된다.
- [x] reload 제약 관련 테스트가 추가된다.
- [x] preflight 결과 테스트가 추가된다.
- [x] CI에서 신규 테스트가 실행된다.

### G1 완료 조건

- [x] `pkexec` 실패/부재 환경에서 fallback 경로가 동작한다.
- [x] 권한 실패 시 사용자에게 원인/다음 조치가 표시된다.

### G2 완료 조건

- [x] Linux TUI에 reload 액션이 제공된다.
- [x] Linux TUI에 auto-start(로그인 시 실행) 제어가 제공된다.
- [x] Windows TUI 대비 핵심 제어 항목 차이가 문서화된다.

### G3 완료 조건

- [x] TUI 액션 실패 시 사용자 피드백이 즉시 표시된다.
- [x] 진행 상태(Starting/Stopping 등)가 메뉴/툴팁에 반영된다.
- [x] 상태 폴링 전환 시 아이콘/메뉴 상태가 일관된다.

### G4 완료 조건

- [x] Linux TUI 바이너리가 CI 산출물에 포함된다.
- [x] Linux 설치 경로(패키징 또는 설치 스크립트)가 확정된다.
- [x] README/PACKAGING 문서가 최신 흐름과 일치한다.

### G5 완료 조건

- [x] TUI 핵심 액션 테스트(start/stop/restart/reload/toggle)가 추가된다.
- [x] Linux/Windows 공통 액션 계층이 테스트 가능 구조로 정리된다.
- [x] CI에서 TUI 테스트가 자동 실행된다.

## 5) 크리티컬 패스

- [x] `C1 -> C2 -> C5`
- [x] `G1 -> G2 -> G5`
- [x] `G2 -> G4`

## 6) 이슈/차단 항목 트래커

- [x] 차단 이슈 없음 (발생 시 즉시 기록)
- [x] 외부 의존성 이슈 없음 (발생 시 즉시 기록)
- [x] 배포/서명/권한 정책 이슈 없음 (발생 시 즉시 기록)

## 7) GUI 품질 핫픽스 (1~4)

- [x] `1` Linux TUI `--action` 안내문에 `reload` 포함
- [x] `2` Linux/Windows TUI 공통 액션 동시실행 차단 게이트 적용
- [x] `3` Windows TUI auto-start 토글 성공/실패 상태 피드백 추가
- [x] `4` 공통 액션 게이트 단위 테스트 추가

## 8) 순차 실행 결과 (1 -> 2 -> 3 -> 4)

- [x] `1` 회귀 검증: `sudo go test ./cmd/gov-pass-tui/...`, `sudo go test ./cmd/splitter/... ./internal/...`, `GOOS=windows GOARCH=amd64 go test -c ./cmd/gov-pass-tui`
- [x] `2` GUI 런타임 스모크: Linux `--action status/reload/restart/toggle` 검증 (서비스 상태 복원 확인)
- [x] `3` 설치 경로 재확인: `sudo ./scripts/install_one_touch.sh` 실행 후 `gov-pass.service` 활성 상태 확인
- [x] `4` 마감 반영: 체크리스트/로그 업데이트 완료
