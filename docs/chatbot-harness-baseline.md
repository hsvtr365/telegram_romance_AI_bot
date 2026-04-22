# 챗봇 하네스 기준선 요약

## 목적
- 현재 코드 기준 대화/선톡/백그라운드 분석 흐름을 짧게 고정한다.
- 기준 소스: `internal/channel/types.go`, `internal/telegram/polling.go`, `internal/chat/service.go`, `internal/chat/structured_extraction.go`, `internal/chat/memory_slot_analyzer.go`, `internal/proactive/scheduler.go`, `internal/gemini/tts.go`

## 1. 일반 대화 파이프라인
1. `internal/app/app.go`
   - bot config의 `channels` 배열을 순회하며 bot/channel별 runner와 scheduler를 조립한다.
   - 현재 구현 채널은 Telegram이며, Discord는 adapter 추가를 위한 설정 형태만 열려 있다.
2. `internal/telegram/polling.go`
   - `Poller.Run`이 Telegram update를 수신하고 `channel.InboundMessage`로 변환한다.
   - private/text 필터는 Telegram adapter에서 먼저 처리한다.
3. `internal/chat/service.go`
   - `HandleMessage`가 `bot_id + channel + external_chat_id` lock key로 명령어와 일반 대화를 처리한다.
4. `internal/store/store.go`
   - `BootstrapContext`에서 `(bot_id, channel, external_user_id)` 기준 user/session/timestamped recent/profile/traits를 읽는다.
5. `internal/store/store.go`
   - 사용자 메시지를 `SaveTurnRecord`로 저장하고 generic `channel`, `external_message_id`와 Telegram 호환 id를 함께 남긴다.
6. `internal/chat/service.go`
   - `captureProactiveSignals`, `captureUserSignals`, `CountUserTurns`, phase, holiday context를 계산한다.
   - `captureUserSignals`는 즉시 추출 + 구조화 추출 비동기 enqueue를 함께 수행한다.
7. `internal/chat/service.go`
   - 저장된 topic slots / conversation state를 읽는다.
8. `internal/chat/memory_slot_analyzer.go`
   - 작은 모델 `SyncAnalyze`를 짧은 timeout으로 실행하고, 성공 시 현재 턴 prompt에만 overlay 한다.
9. `internal/chat/prompt_builder.go`
   - current time, stale-aware history summary, timestamped recent conversation, profile, traits, holiday, topic slots, conversation state를 합쳐 prompt를 만든다.
   - recent conversation 각 턴은 절대 시각을 포함하고, history summary에는 "최근 대화 우선" guardrail이 붙는다.
   - memory summary에는 topic `last_seen_at`, state `updated_at` 같은 시간 정보가 함께 들어간다.
   - section formatting은 `internal/promptutil/sections.go`를 재사용한다.
10. `internal/ollama/client.go`
   - 메인 LLM 호출을 수행한다.
11. `internal/chat/postprocess.go`
   - `PostProcess`, `SplitReplyForTelegram`로 응답을 정리한다.
12. `internal/telegram/client.go`
   - `channel.Messenger` 구현체로서 text/typing/audio 전송을 담당한다.
13. `internal/store/store.go`
   - assistant turn 저장과 recent cache 갱신을 수행한다.
14. `internal/chat/memory_slot_analyzer.go`
   - 응답 후 `AsyncEnqueue`로 메모리 슬롯 저장용 비동기 분석을 enqueue 한다.

## 2. 백그라운드 분석 파이프라인
- 구조화 추출
  - `internal/chat/structured_extraction.go`
  - `AsyncRunner` 기반 bounded queue/worker/timeout/supersede 규칙을 사용한다.
- 메모리 슬롯 분석
  - `internal/chat/memory_slot_analyzer.go`
  - 동기 overlay + 비동기 저장의 하이브리드 구조다.
- 공통 실행기
  - `internal/chat/async_runner.go`
  - 작은 모델 후처리 작업의 queue/worker/timeout/supersede를 공통 처리한다.

## 3. 선톡 파이프라인
1. `internal/proactive/scheduler.go`
   - reminder ticker, standard ticker로 주기 실행한다.
2. `internal/proactive/scanner.go`
   - bot/channel별 active session, due memory event를 읽고 candidate를 만든다.
   - 세션별 profile, recent messages, recent proactives를 추가 조회한다.
3. `internal/proactive/eligibility.go`, `scoring.go`, `strategy.go`
   - 적격성, 점수, 발송 전략을 계산한다.
4. `internal/app/proactive_repository.go`
   - compose 전에 `GetMemorySummary`로 topic/state 기반 memory summary를 만든다.
   - summary에는 topic/state 시간 정보가 포함된다.
5. `internal/proactive/composer.go`
   - seed 선택, holiday context 결합, LLM 생성 또는 seed fallback을 수행한다.
   - proactive prompt도 timestamped recent conversation formatter를 재사용한다.
   - postprocess/split은 `internal/chat/postprocess.go` 구현을 재사용한다.
6. `internal/proactive/sender.go`
   - `channel.OutboundTarget`으로 발송, proactive message 저장, conversation turn 저장, session state 갱신을 수행한다.
7. `internal/proactive/scheduler.go`
   - feedback sweep에서 사용자의 reply를 평가한다.
   - `reward_tts_enabled`가 켜진 bot은 warm reply 또는 충분한 follow-up이 감지되면 보상 TTS를 생성해 같은 target으로 전송한다.
8. `internal/gemini/tts.go`
   - Gemini TTS 응답 audio inline data를 추출한다.
   - raw PCM(`audio/L16;rate=...`) 응답은 WAV container로 변환해 channel audio attachment로 넘긴다.

## 4. 현재 공통 모듈
- channel contract: `internal/channel/types.go`
- Telegram adapter/I/O: `internal/telegram/polling.go`, `internal/telegram/client.go`
- LLM 호출: `internal/ollama/client.go`
- Gemini TTS: `internal/gemini/tts.go`
- 저장소 진입점: `internal/store/store.go`
- 휴일 컨텍스트: `internal/holiday/context.go`
- 비동기 분석 실행기: `internal/chat/async_runner.go`
- prompt section/time formatter: `internal/promptutil/sections.go`, `internal/promptutil/time.go`
- 응답 후처리/분할: `internal/chat/postprocess.go`

## 5. 현재 하네스화된 지점
- 메모리 슬롯: sync overlay + async persist
- 구조화 추출: bounded concurrency + supersede
- prompt section formatting: chat/proactive/memory 분석기 공통 규칙
- prompt temporal grounding: chat/proactive/memory 분석기 모두 공용 timestamp 포맷 사용
- proactive memory summary: topic/state를 선톡 compose에도 재사용
- bot/channel 분리: user/session/proactive scan은 `bot_id + channel + external_user_id/chat_id` 기준
- reward TTS: proactive feedback 결과를 audio 보상 트리거로 연결

## 6. 아직 남은 핵심 과제
- stage trace schema와 prompt version 키는 아직 고정되지 않았다.
- `CountUserTurns` 별도 조회는 여전히 남아 있다.
- proactive scan의 세션별 N+1 조회는 아직 남아 있다.
- recent cache warm-up bulk set 경로는 아직 없다.
- Discord adapter는 아직 실제 구현되지 않았다.
- reward TTS는 Gemini 실 API 통합 경로가 열렸지만, 비용/쿼터가 드는 live integration 테스트는 별도 운영 하네스가 필요하다.
