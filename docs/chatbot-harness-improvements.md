# 챗봇 하네스 개선 후보

## 목적
- 현재 남은 병목만 짧게 관리한다.
- 완료 이력은 `docs/archive/chatbot-refactoring-completed.md`로 이동했다.

## 우선 후보
### P1. 일반 대화 1턴당 연속 I/O 과다
- 문제: `HandleUpdate` 한 번에 `BootstrapContext`, `SaveTurn(user)`, `CountUserTurns`, holiday 조회, `SaveTurn(assistant)`가 연속 수행된다.
- 영향: 대화 1턴 latency가 저장소 round-trip 수에 크게 묶인다.
- 개선 방향: `ConversationSnapshot` 조회 묶기, `CountUserTurns` 대체, 보조 저장 비동기화 검토
- 선행 계측값: handle total latency, DB/Redis 호출 수, `BootstrapContext` latency, `CountUserTurns` 호출 비율

### P1. 선톡 스캐너의 세션별 N+1 조회
- 문제: `Scanner.scanAt`가 세션 목록 뒤에 각 세션마다 profile, recent messages, recent proactives를 개별 조회한다.
- 영향: active session 수가 늘수록 scan latency와 DB 부하가 선형 이상으로 증가한다.
- 개선 방향: scan용 projection 조회, profile/recent/proactive batch load
- 선행 계측값: scan session 수, 세션당 쿼리 수, scan total latency, hot query 상위 3개

### P2. 후보 선정 뒤 적격성/점수 재계산
- 문제: `selectSessionWinners`와 `handleScannedCandidate`에서 eligibility/score를 다시 계산한다.
- 영향: CPU 낭비보다 판단 근거가 두 번 생겨 trace 해석이 어려워진다.
- 개선 방향: scan 결과에 decision basis를 함께 싣거나, winner 계산값 재사용
- 선행 계측값: candidate 수, winner 수, score 계산 횟수, decision trace 중복률

### P2. recent cache warm-up 시 Redis append 반복
- 문제: cache miss 시 Postgres recent messages를 읽은 뒤 각 메시지를 `AppendRecent`로 다시 밀어 넣는다.
- 영향: cold session에서 Redis write가 turn 수만큼 발생한다.
- 개선 방향: recent list bulk set 또는 pipeline warm-up
- 선행 계측값: recent cache hit ratio, warm-up 발생률, warm-up 당 Redis command 수

## 운영 우선 계측 항목
- `chat.handle.total_ms`
- `chat.store.bootstrap_ms`
- `chat.store.query_count`
- `chat.llm.generate_ms`
- `chat.fallback.reason`
- `proactive.scan.total_ms`
- `proactive.scan.session_count`
- `proactive.scan.query_count`
- `proactive.compose.total_ms`
- `structured_extract.total_ms`
- `memory_slot.sync_ms`
- `memory_slot.async_ms`
