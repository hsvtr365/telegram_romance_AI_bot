# 챗봇 하네스 개선 후보

## 목적
- 현재 남은 병목만 짧게 관리한다.
- 완료 이력은 `docs/archive/chatbot-refactoring-completed.md`로 이동했다.

## 최근 반영
- recent conversation prompt에 절대 시각을 넣어 어제/오늘 경계를 모델이 구분할 수 있게 함
- history summary에 stale guardrail을 붙여 과거 일회성 화제가 현재 사실처럼 재사용되는 문제를 줄임
- memory summary에 topic `last_seen_at`, state `updated_at`를 포함해 시간축을 노출함
- Telegram raw update를 `channel.InboundMessage`로 표준화해 chat core에서 Telegram 타입 의존을 제거함
- bot config에 `channels` 배열과 persona prompt file 분리를 반영함
- DB identity 기준을 `bot_id + channel + external_user_id/chat_id`로 확장함
- Telegram adapter가 `SendText`, `SendTyping`, `SendAudio` channel messenger contract를 구현함
- 이진혁용 `reward_tts_enabled`와 Gemini TTS 보상 audio 경로를 추가함

## 우선 후보
- Discord adapter MVP
  - Gateway DM/mention 수신을 `channel.Runner`로 구현
  - Discord 전송을 `channel.Messenger`로 구현
- reward TTS 운영 하네스
  - 실제 Gemini API 호출은 비용/쿼터가 있으므로 별도 smoke command 또는 dry-run 옵션 필요
  - TTS 실패율, latency, audio size 계측 필요
- bot policy file
  - `policy_path`는 config에 열려 있지만 실제 정책 엔진은 아직 없음
  - 서태규/이진혁 기능 조건을 JSON 정책으로 분리하는 후속 작업 필요

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
- `proactive.feedback.total_ms`
- `proactive.reward_tts.generate_ms`
- `proactive.reward_tts.send_ms`
- `proactive.reward_tts.error`
- `proactive.reward_tts.audio_bytes`
- `structured_extract.total_ms`
- `memory_slot.sync_ms`
- `memory_slot.async_ms`
- `channel.delivery.channel`
- `channel.delivery.kind`
- `channel.delivery.error`
