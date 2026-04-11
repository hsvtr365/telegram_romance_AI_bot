# 챗봇 하네스 설계

## 목표
- 일반 대화, 선톡, 백그라운드 분석을 같은 관점에서 읽을 수 있게 만든다.
- 현재 동작 골격만 요약하고, 남은 과제는 별도 문서에서 관리한다.

## 1. 공통 골격

### `HarnessInput`
- message meta: `update_id`, `telegram_chat_id`, `telegram_user_id`, `message_text`
- session snapshot: mode, recent turn limit, conversation phase
- conversation context: recent conversation, profile, traits
- memory context: topic slots, conversation state, memory summary
- extra context: holiday, event, proactive candidate

### `HarnessStage`
- `normalize`
  - raw update/event를 내부 입력으로 표준화
- `enrich`
  - session/profile/recent/holiday/memory context를 조립
- `background_analyze`
  - structured extraction, memory slot async analyze
- `prompt_build`
  - section formatter로 prompt 조립
- `generate`
  - main/reminder model 호출 또는 fallback 선택
- `postprocess`
  - text sanitize, sentence split
- `persist`
  - message/proactive/session state 저장

### `HarnessOutput`
- `final_text`
- `used_model`
- `fallback_used`
- `memory_overlay_used`
- `persist_result`

## 2. 현재 구현 매핑
- chat harness
  - `HandleUpdate` 안에서 `context -> memory overlay -> prompt -> generate -> postprocess -> persist`가 실제로 구현되어 있다.
- proactive harness
  - `scan -> decide -> compose -> send -> persist` 흐름이 유지된다.
  - compose 전에 memory summary가 주입된다.
- background harness
  - `structured extraction`과 `memory slot analyzer`가 같은 실행기 형태를 쓴다.

## 3. 운영 체크리스트
- timeout
- bounded worker
- supersede
- fallback text/seed
- memory summary 재사용

## 4. 테스트 하네스 시나리오
- 채팅
  - 첫 대화/리셋 직후 false familiarity 차단
  - profile extraction 성공/실패
  - memory slot sync parse/merge/prompt gate
  - `PostProcess` / `SplitReplyForTelegram` 회귀
- 선톡
  - reminder / reconnect / event_followup / mood_repair
  - memory summary prompt 주입
  - dedupe, opt-in, quiet hours, threshold
- 실행기
  - queue full drop
  - supersede된 stale job skip
  - timeout 시 fail-open
