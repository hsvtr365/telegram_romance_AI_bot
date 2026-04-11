# Telegram + Ollama 가상연애 챗봇 MVP 설계서

## 0. 문서 목적

이 문서는 `Telegram Bot API + Go + Ollama + Redis + PostgreSQL` 조합으로 동작하는 1:1 가상연애 챗봇 MVP의 권장 구조를 정의한다.

### 현행화 메모

- 이 문서는 초기 MVP 설계 초안이다. 현재 구현 기준은 [chatbot-harness-baseline.md](/Users/suji/Documents/P2/docs/chatbot-harness-baseline.md), [chatbot-harness-design.md](/Users/suji/Documents/P2/docs/chatbot-harness-design.md), 실제 소스가 우선이다.
- 현재 메모리 구조는 `recent conversation + profile/traits + memory events + topic slots + conversation state` 조합이다.
- 구조화 추출과 메모리 슬롯 분석은 [`internal/chat/async_runner.go`](/Users/suji/Documents/P2/internal/chat/async_runner.go) 기반 비동기 후처리로 동작한다.
- 아래 일부 스키마/패키지 예시는 초기 설계 흔적이므로, 현재 파일명/테이블명과 1:1로 일치하지 않을 수 있다.

핵심 방향은 다음과 같다.

- 초기 수신 방식은 Telegram `long polling`
- LLM은 Ollama 서버를 통해 호출
- 모델명은 `.env`에서 교체 가능
- 최근 대화는 짧게 유지하고, 기억은 구조화 메모리(profile/traits/events/topic slots/state)로 분리
- API 서버는 얇게 유지하고 책임을 명확히 분리

기본 대화 모드는 `spicy`를 기준으로 설계한다.

---

## 1. 전체 시스템 아키텍처

### 1.1 구성 요소

#### Telegram
- 사용자가 메시지를 보내는 클라이언트
- Bot API를 통해 업데이트를 제공

#### Go Bot Server
- `getUpdates` 기반 long polling 수행
- 사용자/세션 식별
- 최근 대화, 메모리, 프로필 조회
- 프롬프트 조립
- Ollama 호출
- Telegram 응답 전송
- 대화 로그 저장

#### Redis
- 최근 대화 캐시
- 세션 락
- 짧은 쿨다운
- 선톡 락/중복 방지 키
- 단기 메모리 캐시

#### PostgreSQL
- 사용자, 세션, 메시지 영속 저장
- 장기 메모리 프로필 저장
- 감정 이벤트/관계 이벤트 저장
- 운영 로그 및 추후 관리자 기능의 기준 저장소

#### Ollama
- 실제 LLM 추론 담당
- 예: `gemma4:4b`
- 나중에 다른 모델로 교체 가능하도록 `.env`에서 관리

### 1.2 데이터 흐름

1. 사용자가 Telegram에서 메시지 전송
2. Go Bot Server가 `getUpdates`로 업데이트 수신
3. 서버가 `users`, `chat_sessions`를 조회/생성
4. Redis에서 최근 대화 조회, Postgres/Redis에서 메모리 조회
5. 서버가 프롬프트 조립 후 Ollama `/api/chat` 호출
6. 응답을 후처리한 뒤 Telegram `sendMessage` 호출
7. 사용자/봇 메시지를 Postgres에 저장
8. 최근 대화 캐시를 Redis에 갱신
9. 구조화 추출과 메모리 슬롯 비동기 분석이 후속 메모리를 갱신

### 1.3 권장 배치

- `telegram-bot-server`: 단일 Go 프로세스
- `ollama`: 별도 프로세스 또는 별도 서버
- `redis`: 원격 단일 인스턴스
- `postgres`: 원격 단일 인스턴스

초기 MVP는 단일 인스턴스로 충분하다. 구조는 이후 webhook/수평 확장 전환이 가능하게 유지한다.

---

## 2. 프로젝트 디렉터리 구조

```text
.
├── cmd/
│   └── bot-api/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── app.go
│   │   ├── proactive_codec.go
│   │   ├── proactive_mapper.go
│   │   └── proactive_repository.go
│   ├── chat/
│   │   ├── async_runner.go
│   │   ├── memory_slot_analyzer.go
│   │   ├── postprocess.go
│   │   ├── prompt_builder.go
│   │   ├── service.go
│   │   └── structured_extraction.go
│   ├── config/
│   │   └── config.go
│   ├── holiday/
│   │   └── ...
│   ├── httpserver/
│   │   └── ...
│   ├── ollama/
│   │   ├── client.go
│   │   └── dto.go
│   ├── proactive/
│   │   ├── composer.go
│   │   ├── prompt_builder.go
│   │   ├── scanner.go
│   │   ├── scheduler.go
│   │   ├── sender.go
│   │   ├── strategy.go
│   │   └── templates.go
│   ├── promptutil/
│   │   └── sections.go
│   ├── store/
│   │   ├── postgres/
│   │   │   ├── memory_slots.go
│   │   │   ├── proactive.go
│   │   │   ├── store.go
│   │   │   ├── user_profile.go
│   │   │   └── user_trait.go
│   │   ├── redis/
│   │   │   ├── proactive.go
│   │   │   └── recent_chat.go
│   │   └── model/
│   │       └── ...
│   ├── telegram/
│   │   ├── client.go
│   │   ├── polling.go
│   │   └── types.go
│   └── textutil/
│       └── strings.go
├── pkg/
│   └── logx/
│       └── logger.go
├── docs/
│   └── telegram-ollama-dating-bot-mvp.md
├── .env.example
├── .gitignore
├── go.mod
└── go.sum
```

### 2.1 디렉터리 역할

#### `cmd/bot-api`
- 프로그램 시작점
- config 로드, DB/Redis/Ollama/Telegram 초기화, polling 시작

#### `internal/app`
- 의존성 조립
- 서비스 인스턴스 생성

#### `internal/config`
- `.env` 및 환경변수 로드
- 설정 struct 정의

#### `internal/telegram`
- Telegram API 래퍼
- long polling / webhook 수신 어댑터 분리
- update, message DTO 정의

#### `internal/chat`
- 사용자 메시지 처리의 핵심 흐름
- 프롬프트 조립
- 응답 후처리
- 구조화 추출과 메모리 슬롯 분석

#### `internal/ollama`
- Ollama HTTP 클라이언트
- 요청/응답 DTO

#### `internal/proactive`
- 선톡 스캔, 적격성, 점수 계산, compose, 발송
- 선톡용 prompt builder와 템플릿 관리

#### `internal/promptutil`
- chat / proactive 공용 section 렌더링

#### `internal/store/postgres`
- Postgres repository 구현

#### `internal/store/redis`
- Redis key 접근 및 캐시/락 처리

#### `internal/store/model`
- 도메인 모델

#### `internal/textutil`
- 소형 문자열 helper

#### `pkg/logx`
- 공용 로깅 유틸

---

## 3. Telegram 연동 구조

### 3.1 초기 방식: Long Polling

Telegram `getUpdates`를 사용한다.

권장 요청 파라미터:

- `timeout=30`
- `limit=50`
- `allowed_updates=["message"]`

MVP에서는 개인 대화만 처리하므로 `message`만 우선 수신한다.

### 3.2 Offset 처리 방식

- 앱 시작 시 `offset`은 0 또는 Redis/메모리에 저장된 마지막 값 사용
- `update_id`를 순차 처리
- 처리 성공 후 `offset = update_id + 1`
- 처리 실패 시 offset을 올리지 않아 재처리 가능하게 유지

권장:

- 기본은 메모리 보관
- 안정성 강화를 원하면 `telegram:offset:{botName}` 키로 Redis 저장

### 3.3 Polling Loop 구조

```text
for {
  getUpdates(offset, timeout, allowed_updates)
  for each update {
    handleUpdate(update)
    if success {
      offset = update.ID + 1
    }
  }
}
```

### 3.4 메시지 수신 후 처리 흐름

1. update에서 `message` 존재 여부 확인
2. private chat 여부 확인
3. text 메시지 여부 확인
4. user/session 식별
5. session lock 획득
6. 최근 대화 + 메모리 조회
7. Ollama 호출
8. 응답 전송
9. 로그 저장
10. lock 해제

### 3.5 Webhook 전환 시 변경점

변경되는 부분은 입력 어댑터뿐이다.

- `internal/telegram/polling.go` 제거 또는 비활성화
- `internal/telegram/webhook.go`에서 HTTP handler로 update 수신
- `chat.Service` 중심 처리 흐름은 그대로 유지

즉, 내부 채팅 처리 서비스는 그대로 두고 수신 방식만 교체한다.

---

## 4. 채팅 처리 플로우

### 4.1 단계별 플로우

1. `getUpdates`로 update 수신
2. Telegram user / chat 식별
3. `users` upsert
4. `chat_sessions` 조회 또는 생성
5. Redis session lock 획득
6. `BootstrapContext`로 recent/profile/traits 조회
7. 사용자 메시지 저장
8. proactive signal / structured extraction / holiday context 계산
9. 저장된 topic slots / conversation state 조회
10. 짧은 timeout의 memory slot sync overlay 시도
11. 프롬프트 조립
12. Telegram `typing` 전송
13. Ollama `/api/chat` 호출
14. 응답 텍스트 후처리
15. Telegram `sendMessage`
16. assistant 메시지 저장과 Redis 최근 대화 캐시 갱신
17. structured extraction / memory slot async analysis enqueue
18. session lock 해제

### 4.2 처리 단위

- 1개 user = 1개 active session 기준
- session 단위로 컨텍스트와 락 관리

---

## 5. 메모리 구조 설계

최근 원문은 짧게 유지하고, 나머지는 구조화 메모리로 분리한다.

### 5.1 Short Memory

- 내용: 최근 원문 대화 12~16턴
- 저장 위치: Redis 우선, Postgres 영속 보관
- 조회 시점: 매 응답 생성 직전
- 갱신 시점: 사용자/봇 메시지 저장 직후

권장 구조:

- Redis List 또는 JSON 배열
- 턴 단위 저장 예: `[{role:"user", text:"..."}, ...]`

### 5.2 Structured Memory

- 내용: profile, traits, memory events, topic slots, conversation state
- 예:
  - 사용자 취미/직업/호칭 선호
  - 최근 시험/약속/감정 이벤트
  - 현재 중요한 화제와 대화 진행 단계
- 저장 위치: Postgres 중심, 일부 recent/cache는 Redis 사용
- 조회 시점: 프롬프트 조립 직전
- 갱신 시점: 사용자 메시지 처리 후 구조화 추출 + memory slot async analysis

### 5.3 Long Memory

- 내용: 관계 설정, 호칭, 스타일 선호, 대화 규칙 같은 안정적 선호
- 예:
  - 사용자 호칭: 자기야
  - 봇 호칭: 누나
  - 선호 말투: 장난스럽고 살짝 도발적
  - 관계 단계: 썸 후반
- 저장 위치: Postgres + Redis 캐시
- 조회 시점: 프롬프트 조립 직전
- 갱신 시점:
  - `/start` 또는 초기 설정 시
  - 대화 중 명시적 변경 감지 시
  - 관리 기능 추가 후 수동 수정 가능

### 5.4 조회 우선순위

1. short memory
2. active topic slots / conversation state
3. profile / traits / memory events
4. current user input

### 5.5 메모리 갱신 기준

권장 기준:

- 매 사용자 메시지 후 구조화 추출을 비동기 enqueue
- 필요 시 작은 모델 sync overlay를 짧은 timeout으로만 시도
- low confidence 슬롯은 저장 중심으로 두고, prompt 주입은 재등장 후 승격

---

## 6. DB 스키마 설계

### 6.1 테이블 개요

- 현재 구현은 `tg_users`, `tg_chat_sessions`, `tg_chat_messages`, `tg_user_profiles`, `tg_user_traits`, `tg_memory_events`, `tg_topic_slots`, `tg_conversation_state_slots` 계열을 사용한다.
- 아래 SQL은 초기 MVP 시점의 개념 예시다. 실제 스키마는 `internal/store/postgres/store.go`, `user_profile.go`, `user_trait.go`, `proactive.go`, `memory_slots.go`를 기준으로 본다.

### 6.2 SQL DDL

```sql
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    telegram_user_id BIGINT NOT NULL UNIQUE,
    telegram_chat_id BIGINT NOT NULL UNIQUE,
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    language_code VARCHAR(32),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_active ON users (is_active);

CREATE TABLE IF NOT EXISTS chat_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_status VARCHAR(32) NOT NULL DEFAULT 'active',
    mode VARCHAR(32) NOT NULL DEFAULT 'spicy',
    recent_turn_limit SMALLINT NOT NULL DEFAULT 14,
    last_message_at TIMESTAMPTZ,
    last_summary_at TIMESTAMPTZ,
    last_message_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, session_status)
);

CREATE INDEX IF NOT EXISTS idx_chat_sessions_user_id ON chat_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_last_message_at ON chat_sessions (last_message_at DESC);

CREATE TABLE IF NOT EXISTS chat_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    telegram_message_id BIGINT,
    telegram_update_id BIGINT,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    content_len INT NOT NULL DEFAULT 0,
    mode VARCHAR(32) NOT NULL DEFAULT 'spicy',
    prompt_version VARCHAR(64),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_session_id_id
    ON chat_messages (session_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_chat_messages_session_created
    ON chat_messages (session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_chat_messages_role
    ON chat_messages (role);

CREATE TABLE IF NOT EXISTS memory_profile (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL UNIQUE REFERENCES chat_sessions(id) ON DELETE CASCADE,
    user_nickname VARCHAR(100),
    bot_nickname VARCHAR(100),
    relationship_stage VARCHAR(64) NOT NULL DEFAULT 'flirting',
    tone_style VARCHAR(128) NOT NULL DEFAULT 'playful_spicy',
    preferred_mode VARCHAR(32) NOT NULL DEFAULT 'spicy',
    profile_summary TEXT NOT NULL DEFAULT '',
    style_notes TEXT NOT NULL DEFAULT '',
    constraints_notes TEXT NOT NULL DEFAULT '',
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_profile_preferred_mode
    ON memory_profile (preferred_mode);

CREATE TABLE IF NOT EXISTS memory_events (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    source_message_id BIGINT REFERENCES chat_messages(id) ON DELETE SET NULL,
    event_type VARCHAR(64) NOT NULL,
    event_value VARCHAR(255),
    summary TEXT NOT NULL,
    emotional_tone VARCHAR(64),
    importance SMALLINT NOT NULL DEFAULT 1,
    happened_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_memory_events_session_created
    ON memory_events (session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_events_session_importance
    ON memory_events (session_id, importance DESC, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_events_type
    ON memory_events (event_type);
```

### 6.3 필드 용도 요약

- `chat_sessions.mode`: 현재 대화 모드
- `chat_messages.metadata`: 응답 길이, 후처리 결과, retry 여부 등
- `memory_profile.profile_summary`: 장기 프로필 요약
- `memory_profile.style_notes`: 말투/호칭/캐릭터 메모
- `memory_events`: 최근 감정 상태, 다툼, 애칭 변경, 기념일 언급 등

---

## 7. Redis 키 설계

### 7.1 키 네이밍 규칙

```text
chat:recent:{sessionId}
chat:lock:{sessionId}
chat:cooldown:{sessionId}
chat:pending:{sessionId}
memory:profile:{sessionId}
memory:events:{sessionId}
analysis:pending:{sessionId}
telegram:offset:{botName}
```

### 7.2 키별 용도와 TTL

#### `chat:recent:{sessionId}`
- 최근 12~16턴 캐시
- 타입: List 또는 String(JSON)
- TTL: `7d`

#### `chat:lock:{sessionId}`
- 세션 동시 처리 방지
- 타입: String
- TTL: `20s`

#### `chat:cooldown:{sessionId}`
- 지나치게 빠른 연속 입력 완충
- 타입: String
- TTL: `2s`

#### `chat:pending:{sessionId}`
- 처리 중 들어온 추가 메시지 임시 저장
- 타입: List
- TTL: `5m`

#### `memory:profile:{sessionId}`
- 장기 메모리 캐시
- 타입: String(JSON)
- TTL: `24h`

#### `memory:events:{sessionId}`
- 최근 이벤트 캐시
- 타입: String(JSON)
- TTL: `6h`

#### `analysis:pending:{sessionId}`
- 구조화 추출 / 메모리 슬롯 분석 중복 방지 예시 키
- 타입: String
- TTL: `10m`

#### `telegram:offset:{botName}`
- 마지막 polling offset 저장
- 타입: String
- TTL: 없음

---

## 8. 프롬프트 구성 전략

프롬프트는 길게 만들지 않는다.

구성 순서:

1. system prompt
2. memory summary
3. recent conversation
4. current user input

### 8.1 System Prompt

목적:

- 봇의 정체성 정의
- 응답 길이와 분위기 정의
- 연애 대화형 말투 정의
- 현재 모드 반영

예시 형식:

```text
너는 텔레그램에서 대화하는 1:1 가상연애 챗봇이다.
말투는 자연스러운 한국어 구어체를 사용한다.
설명형 답변보다 대화형 반응을 우선한다.
답변은 짧고 템포 있게, 보통 1~4문장으로 말한다.
현재 대화 모드는 spicy다.
spicy 모드에서는 장난스럽고 플러팅 섞인 말투를 사용하고,
가벼운 도발과 살짝 얄미운 애정 표현을 자연스럽게 섞는다.
과하게 길게 설명하지 말고, 상대와 실시간으로 톡하는 느낌을 유지한다.
```

### 8.2 Memory Summary

목적:

- 장기 관계 정보 반영
- 최근 감정 상태 유지

예시 형식:

```text
[Memory Profile]
- 사용자 호칭: 자기
- 봇 호칭: 누나
- 관계 단계: 썸 후반
- 선호 말투: 장난스럽고 살짝 집착 섞인 플러팅

[Recent Emotional Events]
- 오늘 사용자가 피곤하다고 말함
- 방금 살짝 삐진 말투를 보임
- 최근 애칭 사용에 긍정적 반응
```

### 8.3 Recent Conversation

목적:

- 최신 문맥 유지
- 현재 감정선 유지

예시 형식:

```text
[Recent Conversation]
user: 오늘 좀 힘들었다
assistant: 왜 또 혼자 끙끙 앓았어, 나한텐 말하지
user: 너는 맨날 그렇게 다 아는 척이야
assistant: 아는 척 아니고 네 표정 뻔히 보여서 그래
```

### 8.4 Current User Input

목적:

- 현재 턴의 직접 반응 대상

예시 형식:

```text
[Current User Input]
지금 뭐 해
```

### 8.5 프롬프트 길이 관리

- recent turns 최대 14턴 권장
- memory summary는 8줄 이내 권장
- 장기 메모리는 핵심 키-값 위주 유지
- prompt builder에서 문자 수 또는 토큰 수 상한 적용

---

## 9. 연애 대화 모드 설계

### 9.1 모드 종류

#### `soft`
- 다정함 중심
- 장난 강도 낮음
- 위로, 애정 표현, 안정감 강조

#### `spicy` (기본)
- 가벼운 도발, 플러팅, 장난, 밀당
- 톡 대화 같은 빠른 템포
- 살짝 얄미운 표현 허용

#### `rough`
- 더 직설적이고 거친 톤
- 짧고 강한 문장 비중 증가
- 감정 몰입이 강한 캐릭터용

### 9.2 프롬프트 반영 방식

`chat_sessions.mode` 또는 `memory_profile.preferred_mode` 값을 읽어 system prompt에 삽입한다.

예시:

```text
[Mode Guide: spicy]
- 말투는 장난스럽고 은근히 도발적이다.
- 답변은 짧고 템포 있게 말한다.
- 가벼운 질투, 플러팅, 연애식 놀림을 자연스럽게 섞는다.
- 너무 설명형으로 길어지지 않는다.
```

### 9.3 MVP 권장값

- 기본값: `spicy`
- 변경 명령은 차기 확장으로 미뤄도 됨

---

## 10. Ollama 연동 설계

### 10.1 호출 방식

HTTP POST로 Ollama `chat endpoint` 호출

권장 엔드포인트:

- `POST /api/chat`

### 10.2 요청 구조

```json
{
  "model": "gemma4:4b",
  "stream": false,
  "keep_alive": "10m",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "options": {
    "temperature": 0.9,
    "top_p": 0.9,
    "num_ctx": 4096
  }
}
```

### 10.3 권장 설정

- endpoint: `/api/chat`
- `stream=false`
  - MVP에서 구현 단순성 우선
- timeout: `35s`
- retry: 네트워크 오류/일시 실패에 한해 `1회`
- keep_alive: `10m`
- num_ctx: `4096` 또는 모델에 맞춰 `.env` 관리

### 10.4 Context 길이 관리

컨텍스트는 모델의 전체 문맥 길이가 아니라, 애플리케이션 쪽에서 먼저 줄인다.

권장 순서:

1. long memory 축약
2. medium memory 3~5개 이벤트만 사용
3. recent conversation 12~16턴 제한
4. 현재 입력 추가

### 10.5 지연 시 처리

- Ollama 호출 직후 Telegram `sendChatAction(typing)` 전송
- 4초 간격으로 typing 갱신 가능
- timeout 시 짧은 fallback 메시지 반환

예:

```text
잠깐만, 지금 답 정리 중이야. 다시 한번 말해봐도 돼?
```

### 10.6 모델 교체 방식

`.env`에서 관리:

```env
OLLAMA_MODEL=gemma4:4b
```

다른 모델 사용 시 코드 수정 없이 환경변수만 변경한다.

---

## 11. 후처리 설계

LLM 원문을 그대로 보내지 않고 가볍게 정리한다.

### 11.1 후처리 규칙

#### 너무 긴 답변 자르기
- 최대 문자 수 제한
- 권장: `220~320자`

#### 반복 표현 감소
- 같은 문장/유사 문장 반복 시 하나 제거
- 예: 같은 감탄사 2회 이상 반복 제거

#### 말투 일관성 유지
- 현재 mode와 profile style과 맞지 않는 문장 제거
- 설명체, 보고체 문장 정리

#### 답변 길이 제한
- 기본 1~4문장
- 길면 2개의 짧은 문단까지 허용

#### 불필요한 설명체 제거
- "정리하자면", "설명하자면", "AI로서", "다음과 같다" 같은 문구 제거

### 11.2 구현 방식

`internal/chat/postprocess.go`에서 처리

권장 함수:

- `TrimLength(text string, maxChars int) string`
- `ReduceRepetition(text string) string`
- `NormalizeTone(text string, mode string) string`
- `CleanExplanatoryPhrases(text string) string`

---

## 12. 동시성 및 세션 충돌 방지

### 12.1 목표

동일 사용자가 짧은 시간에 여러 메시지를 보낼 때 응답 순서가 꼬이지 않게 한다.

### 12.2 권장 방식

Redis `SETNX` 기반 세션 락 사용

락 예:

```text
key: chat:lock:{sessionId}
ttl: 20s
value: request-id
```

### 12.3 처리 전략

1. 메시지 수신 시 락 획득 시도
2. 락 획득 성공 시 즉시 처리
3. 실패 시 `chat:pending:{sessionId}`에 메시지 적재
4. 현재 처리 완료 후 pending queue 확인
5. 최신 메시지부터 이어서 처리

### 12.4 MVP 단순화안

초기에는 다음 구조로도 충분하다.

- 단일 프로세스
- session 단위 local mutex map
- Redis lock 병행

즉:

- 로컬에서는 빠르게 충돌 방지
- 이후 다중 인스턴스 확장 시 Redis lock이 기준 역할 수행

---

## 13. 설정 파일 구조

### 13.1 환경변수 목록

```env
APP_NAME=heartlink-bot
APP_ENV=local
APP_PORT=8080

TELEGRAM_BOT_TOKEN=
TELEGRAM_ALLOWED_UPDATES=message
TELEGRAM_POLL_TIMEOUT_SEC=30
TELEGRAM_POLL_LIMIT=50

OLLAMA_BASE_URL=https://gitlab.swempire.co.kr/ollama
OLLAMA_MODEL=gemma4-heretic:q4km
OLLAMA_TIMEOUT_SEC=35
OLLAMA_KEEP_ALIVE=10m
OLLAMA_NUM_CTX=4096
OLLAMA_TEMPERATURE=0.9
OLLAMA_TOP_P=0.9

POSTGRES_DSN=postgres://user:password@host:5432/dbname?sslmode=disable
REDIS_URL=redis://user:password@host:6379/0

DEFAULT_CHAT_MODE=spicy
RECENT_TURN_LIMIT=14
SUMMARY_TRIGGER_MESSAGES=10
SESSION_LOCK_TTL_SEC=20
CHAT_COOLDOWN_SEC=2
RESPONSE_MAX_CHARS=280
```

### 13.2 Go Config Struct 예시

```go
type Config struct {
    App struct {
        Name string
        Env  string
        Port int
    }
    Telegram struct {
        BotToken       string
        AllowedUpdates []string
        PollTimeoutSec int
        PollLimit      int
    }
    Ollama struct {
        BaseURL       string
        Model         string
        TimeoutSec    int
        KeepAlive     string
        NumCtx        int
        Temperature   float64
        TopP          float64
    }
    Postgres struct {
        DSN string
    }
    Redis struct {
        URL string
    }
    Chat struct {
        DefaultMode            string
        RecentTurnLimit        int
        SummaryTriggerMessages int
        SessionLockTTLSec      int
        CooldownSec            int
        ResponseMaxChars       int
    }
}
```

---

## 14. 최소 MVP 범위 정의

### 14.1 지금 반드시 구현

- Telegram long polling
- private 1:1 text message 처리
- Go API 서버
- Ollama `/api/chat` 연동
- `.env` 기반 모델명 변경
- Redis 기반 최근 대화 12~16턴 유지
- Postgres 대화 로그 저장
- memory profile + memory events 저장
- 기본 모드 `spicy`
- 간단한 후처리
- 세션 락

### 14.2 차기 확장

- webhook 전환
- 관리자 기능
- 다중 캐릭터
- 대화 모드 변경 명령
- 이벤트 추출 고도화
- 스트리밍 응답
- 감정 점수화
- 요약 품질 향상
- 사용자별 프롬프트 버전 관리

---

## 15. 구현 우선순위

### 1단계. 실행 골격

목표:

- Go 프로젝트 생성
- `.env` 로딩
- Telegram client / Ollama client / Redis / Postgres 연결

완료 기준:

- 서버가 기동되고 외부 의존성 연결 성공

### 2단계. Telegram 수신/응답

목표:

- long polling 구현
- private text message 수신
- 고정 응답 전송

완료 기준:

- 텔레그램에서 메시지 보내면 봇이 응답

### 3단계. 대화 저장 + 최근 메모리

목표:

- users / sessions / messages 저장
- Redis recent chat 캐시 적용
- recent turn limit 적용

완료 기준:

- 최근 14턴 기준으로 대화 문맥 유지

### 4단계. Ollama 프롬프트 연결

목표:

- prompt builder 구현
- mode/spicy 반영
- Ollama 응답 연동
- 후처리 적용

완료 기준:

- 실제 가상연애 톤의 응답 생성

### 5단계. 메모리/후처리 안정화

목표:

- profile / traits / memory events / topic slots 조회/저장
- structured extraction / memory slot async runner 안정화
- session lock / queue / timeout 규칙 적용

완료 기준:

- 최근 대화 + 구조화 메모리 조합으로 안정 동작

---

## 부록 A. 권장 프롬프트 템플릿

```text
[System]
너는 텔레그램에서 1:1로 대화하는 가상연애 챗봇이다.
한국어 메신저 말투를 사용한다.
설명형 답변보다 실제 톡 대화처럼 반응한다.
답변은 짧고 자연스럽게, 보통 1~4문장으로 한다.

[Mode]
현재 모드는 spicy다.
장난스럽고 플러팅 섞인 말투를 사용한다.
살짝 얄미운 애정 표현과 가벼운 도발을 자연스럽게 섞는다.

[Memory Profile]
{profile_summary}

[Recent Events]
{event_summary}

[Recent Conversation]
{recent_turns}

[Current User Input]
{user_input}
```

---

## 부록 B. 구현 메모

- Telegram 전용으로 설계한다.
- 카카오톡 연동은 고려하지 않는다.
- Ollama 모델명은 반드시 `.env`에서 읽는다.
- 실제 운영 비밀번호/접속정보는 `.env`에만 두고 저장소에는 커밋하지 않는다.
- 최근 대화 전체를 계속 쌓아 넣지 말고, recent turns + memory summary 조합을 유지한다.
