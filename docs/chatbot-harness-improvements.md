# 챗봇 하네스 개선 후보

## 목적
- 현재 남은 병목만 짧게 관리한다.
- 완료 이력은 `docs/archive/chatbot-refactoring-completed.md`로 이동했다.

## 최근 반영
- recent conversation prompt에 절대 시각을 넣어 어제/오늘 경계를 모델이 구분할 수 있게 함
- history summary에 stale guardrail을 붙여 과거 일회성 화제가 현재 사실처럼 재사용되는 문제를 줄임
- memory summary에 topic `last_seen_at`, state `updated_at`를 포함해 시간축을 노출함

## 우선 후보
- 현재 남은 최우선 개선 후보 없음 (모두 완료됨)

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
