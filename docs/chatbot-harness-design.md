# 챗봇 하네스 설계

## 목표
- 일반 대화, 선톡, 백그라운드 분석을 같은 관점에서 읽을 수 있게 만든다.
- 현재 동작 골격만 요약하고, 남은 과제는 별도 문서에서 관리한다.

## 1. 공통 골격

### `HarnessInput`
- routing meta: `bot_id`, `channel`, `external_chat_id`, `external_user_id`
- message meta: `external_update_id`, `external_message_id`, `chat_type`, `message_text`, `sent_at`
- channel compatibility meta: Telegram numeric ids are optional compatibility fields, not the canonical identity
- session snapshot: mode, recent turn limit, conversation phase
- conversation context: timestamped recent conversation, profile, traits
- memory context: topic slots with temporal metadata, conversation state, stale-aware history summary, memory summary
- extra context: holiday, event, proactive candidate, custom slots (user-defined rules/settings)
- output target: `channel.OutboundTarget`
- optional reward context: feedback result, reward TTS enabled flag, audio profile/model/voice

### `HarnessStage`
- `normalize`
  - raw channel event를 `channel.InboundMessage`로 표준화
  - Telegram은 `internal/telegram/polling.go`, Discord future adapter도 같은 contract를 구현한다
- `enrich`
  - `bot_id + channel + external_user_id` 기준 session/profile/recent/holiday/memory context를 조립
- `background_analyze`
  - structured extraction, memory slot async analyze
- `prompt_build`
  - section formatter와 공용 시간 formatter로 prompt 조립
  - recent conversation에 절대 시각을 넣고, history summary에는 stale guardrail을 붙인다
- `generate`
  - main/reminder model 호출 또는 fallback 선택
- `postprocess`
  - text sanitize, sentence split
- `deliver`
  - text/typing/audio를 `channel.Messenger`로 전송
- `persist`
  - message/proactive/session state 저장
- `reward_audio`
  - proactive feedback에서 보상 조건을 만족하면 TTS transcript 생성
  - Gemini TTS audio를 생성하고 channel audio attachment로 전송

### `HarnessOutput`
- `final_text`
- `final_audio`
- `used_model`
- `used_tts_model`
- `fallback_used`
- `memory_overlay_used`
- `persist_result`
- `delivery_result`

## 2. 현재 구현 매핑
- chat harness
  - `HandleMessage` 안에서 `context -> memory overlay -> prompt -> generate -> postprocess -> deliver -> persist`가 실제로 구현되어 있다.
- proactive harness
  - `scan -> decide -> compose -> send -> persist -> feedback -> optional reward_audio` 흐름이 유지된다.
  - compose 전에 memory summary가 주입된다.
- background harness
  - `structured extraction`과 `memory slot analyzer`가 같은 실행기 형태를 쓴다.
- channel harness
  - `internal/channel`이 core contract다.
  - Telegram adapter는 raw update를 generic input으로 변환하고, text/typing/audio 전송을 구현한다.
- bot harness
  - `configs/personas/*.md`와 `secrets/bots.local.json`의 bot/channel config로 persona와 채널을 분리한다.
  - bot별 `reward_tts_enabled`로 기능 조건을 다르게 켤 수 있다.

## 3. 운영 체크리스트
- timeout
- bounded worker
- supersede
- fallback text/seed
- memory summary 재사용
- timestamped recent conversation
- stale history guardrail
- bot/channel identity isolation
- token/config secret isolation
- reward TTS quota/cost guard
- audio delivery fallback: TTS 실패 시 text flow는 유지

## 4. 테스트 하네스 시나리오
- 채팅
  - 첫 대화/리셋 직후 false familiarity 차단
  - 최근 대화에 어제/오늘 경계가 드러나도록 timestamp 유지
  - history summary의 일회성 과거 화제가 현재 사실처럼 재주입되지 않음
  - profile extraction 성공/실패
  - memory slot sync parse/merge/prompt gate
  - memory summary에 `last_seen_at` / `updated_at`가 포함됨
  - `PostProcess` / `SplitReplyForTelegram` 회귀
- 채널
  - Telegram update가 `InboundMessage`로 변환됨
  - 같은 external user라도 bot/channel이 다르면 별도 user/session으로 분리됨
  - `SendText`, `SendTyping`, `SendAudio` adapter contract가 유지됨
- 선톡
  - reminder / reconnect / event_followup / mood_repair
  - memory summary prompt 주입
  - recent conversation timestamp 재사용
  - dedupe, opt-in, quiet hours, threshold
  - feedback sweep가 이미 replied 처리된 proactive를 다시 보상하지 않음
  - 이진혁 `reward_tts_enabled=true`일 때 warm reply/follow-up 보상 audio가 생성됨
  - Gemini raw PCM 응답이 WAV로 변환됨
- 실행기
  - queue full drop
  - supersede된 stale job skip
  - timeout 시 fail-open
