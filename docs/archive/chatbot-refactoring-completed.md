# 챗봇 리팩터링 완료 이력

## 목적
- 현재 활성 문서에서 제거한 완료 이력을 짧게 보관한다.
- 지금 시스템 설명은 `docs/chatbot-harness-*.md`를 우선 참고한다.

## 완료 항목
### P1. 비동기 분석 실행기 통합
- 결과
  - `internal/chat/async_runner.go`
  - structured extraction, memory slot analyzer가 같은 queue/worker/supersede 규칙을 사용
- 효과
  - background LLM 후처리 실행 방식이 하나로 정리됨

### P2. `proactiveRepository` 책임 분리
- 결과
  - `internal/app/proactive_mapper.go`
  - `internal/app/proactive_codec.go`
- 효과
  - repository는 orchestration 중심으로 얇아지고, DTO 변환과 JSON decode/helper가 밖으로 이동

### P3. 소형 공통 유틸 정리
- 결과
  - `internal/textutil/strings.go`
  - proactive 전용 `textutil.go` 제거
  - proactive가 chat의 `PostProcess`, `SplitReplyForTelegram`을 재사용
- 효과
  - 텍스트 후처리/분할 중복 제거
  - fallback 문자열 규칙 단순화

### P4. prompt section 렌더링 공통화
- 결과
  - `internal/promptutil/sections.go`
- 효과
  - chat/proactive/memory 분석기의 section 포맷 규칙이 한곳에 모임

### P1. 일반 대화 1턴당 연속 I/O 과다
- 결과
  - `internal/store/store.go`, `internal/chat/service.go`
  - `BootstrapContext`에서 각 조회(recent, profile, traits, turns)를 `errgroup` 패턴으로 병렬화
  - `CountUserTurns` 제거하고 묶음 처리한 값을 사용
- 효과
  - 응답 Latency 대폭 단축, 동기 대기시간 최소화

### P1. 선톡 스캐너의 세션별 N+1 조회
- 결과
  - `internal/proactive/scanner.go`
  - 프로필 등 개별 조회를 지연 평가(lazy loading) 및 메모리 캐싱 패턴으로 전환
  - `ScannedCandidate` 생성 성공 시에만 세부 정보를 Load
- 효과
  - 스캐너 부하 개선, 불필요한 N+1 조회 제거

### P2. 후보 선정 뒤 적격성/점수 재계산
- 결과
  - `internal/proactive/scheduler.go`, `internal/proactive/types.go`
  - `ScannedCandidate`에 `Eligibility` 및 `CandidateScore` 저장
  - 첫 계산한 점수/적격성 값을 유지하고 그대로 재사용
- 효과
  - 판단 근거 연속성 확보 및 CPU 재계산 방지

### P2. recent cache warm-up 시 Redis append 반복
- 결과
  - `internal/store/redis/recent_chat.go`, `internal/store/store.go`
  - `SetRecent` 메서드(파이프라인) 추가
  - warm-up 루프 안의 `AppendRecent` 개별 호출 제거
- 효과
  - cold start 시 DB to Redis warm-up 부하 감소
