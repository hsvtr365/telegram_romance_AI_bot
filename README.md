# telegram_romance_AI_bot

텔레그램에서 1:1로 대화하는 가상연애 챗봇 프로젝트다.  
현재 구현은 `Go long polling bot + Ollama + health check HTTP server` 구조로 시작한다.

## 용도

- 텔레그램 개인 채팅에서 가상연애 톤으로 대화
- Ollama에 올린 로컬/서버 LLM을 호출해서 응답 생성
- 이후 Redis 최근 대화 메모리, Postgres 저장, 장기 메모리 요약을 붙이기 위한 기본 골격

## 현재 실제로 작동하는 것

- Telegram Bot API `getUpdates` 기반 long polling
- `/start`, `/ping` 명령 처리
- `/reset`, `/리셋` 명령으로 대화 기록 초기화
- 일반 텍스트 메시지를 Ollama `/api/chat`으로 전달
- Postgres에 사용자/세션/메시지 저장
- Redis에 최근 대화 14턴 캐시
- `spicy` 기본 모드 프롬프트 적용
- 응답 문장 분리 전송
- `APP_PORT`에서 health endpoint 제공

## 현재 아직 안 붙은 것

- 세션 락
- 요약 메모리 worker
- webhook 모드

즉, 지금은 “기동 가능한 최소 MVP 골격”이다.

## 동작 구조

1. 텔레그램 사용자가 봇에게 메시지를 보낸다.
2. Go 프로세스가 Telegram `getUpdates`로 메시지를 가져온다.
3. `/start`, `/ping`은 즉시 응답한다.
4. `/reset`, `/리셋`은 현재 사용자의 챗봇 전용 세션/메시지/최근 대화 캐시를 삭제한다.
5. Postgres에서 사용자를 upsert하고 active session을 조회/생성한다.
6. Redis 또는 Postgres에서 최근 대화 14턴을 조회한다.
7. 일반 메시지는 가상연애용 system prompt와 함께 Ollama로 보낸다.
8. 사용자/봇 메시지를 Postgres에 저장하고 recent cache를 갱신한다.
9. 동시에 `APP_PORT`에서 `/healthz` 응답을 제공한다.

## 프로젝트 구조

```text
cmd/bot-api              실행 진입점
internal/app             앱 조립
internal/config          .env 로딩
internal/telegram        Telegram long polling 클라이언트
internal/chat            프롬프트/후처리/메시지 처리
internal/ollama          Ollama API 클라이언트
internal/store           Postgres/Redis 저장 계층
internal/httpserver      health check HTTP 서버
pkg/logx                 로거
docs/                    설계 문서
```

## 요구 사항

- Go 1.26+
- Telegram Bot Token
- Ollama 실행 중
- PostgreSQL 접근 가능
- Redis 접근 가능
- 사용할 모델이 Ollama에 준비되어 있어야 함

## 환경 변수

프로젝트 루트의 `.env`를 사용한다.

현재 핵심 값:

```env
APP_PORT=8083

TELEGRAM_BOT_TOKEN=...

OLLAMA_BASE_URL=http://localhost:11434
OLLAMA_MODEL=gemma4-e2b-uncensored-q8kp:latest

POSTGRES_DSN=postgres://...
REDIS_URL=redis://...
```

## Ollama 준비

Ollama가 아직 떠 있지 않다면 먼저 올린다.

```bash
ollama serve
```

모델이 없다면 pull 후 확인한다.

```bash
ollama pull gemma4-e2b-uncensored-q8kp:latest
ollama list
```

다른 모델로 바꾸고 싶으면 `.env`의 `OLLAMA_MODEL`만 바꾸면 된다.

## 로컬에서 실행하는 법

프로젝트 루트에서:

```bash
go run ./cmd/bot-api
```

또는 바이너리로:

```bash
mkdir -p bin
go build -o ./bin/bot-api ./cmd/bot-api
./bin/bot-api
```

실행되면 아래가 동시에 동작한다.

- Telegram long polling 봇
- HTTP health endpoint

## API 서버 확인 방법

현재 HTTP 서버는 health 확인용으로만 열린다.

- 주소: `http://localhost:8083`
- health: `http://localhost:8083/healthz`

확인:

```bash
curl http://localhost:8083/healthz
```

정상 응답 예시:

```json
{"status":"ok"}
```

## 저장되는 데이터

현재 봇은 아래 전용 테이블을 사용한다.

- `tg_users`
- `tg_chat_sessions`
- `tg_chat_messages`

저장 흐름은 다음과 같다.

1. 사용자가 메시지를 보내면 `tg_users` upsert
2. 해당 사용자의 active session을 `tg_chat_sessions`에서 조회/생성
3. 최근 대화는 Redis `chat:recent:{sessionId}`에서 조회
4. 사용자 메시지는 `tg_chat_messages`에 저장
5. Ollama 응답 전송 후 assistant 메시지도 `tg_chat_messages`에 저장
6. user/assistant turn 모두 Redis recent cache에 반영

## 리셋 명령

아래 명령 중 하나를 보내면 현재 사용자의 챗봇 전용 기록을 삭제한다.

```text
/reset
/리셋
```

삭제 범위:

- `tg_chat_messages`
- `tg_chat_sessions`
- 해당 사용자의 Redis recent cache

`tg_users`도 함께 지워졌다가 다음 메시지에서 다시 생성된다.  
즉, 다음 대화는 처음 접속한 것처럼 새 세션으로 시작된다.

## 백그라운드에서 올리는 법

### 1. 바이너리 빌드

```bash
mkdir -p bin logs run
go build -o ./bin/bot-api ./cmd/bot-api
```

### 2. nohup으로 백그라운드 실행

```bash
nohup ./bin/bot-api > ./logs/bot-api.log 2>&1 & echo $! > ./run/bot-api.pid
```

### 3. PID 확인

```bash
cat ./run/bot-api.pid
ps -p "$(cat ./run/bot-api.pid)" -o pid,ppid,command
```

### 4. 로그 보기

```bash
tail -f ./logs/bot-api.log
```

### 5. 서버 살아있는지 확인

```bash
curl http://localhost:8083/healthz
```

## 백그라운드 프로세스 내리는 법

```bash
kill "$(cat ./run/bot-api.pid)"
```

강제 종료가 필요하면:

```bash
kill -9 "$(cat ./run/bot-api.pid)"
```

## 운영 메모

- 이 프로젝트는 현재 webhook이 아니라 long polling 방식이다.
- 따라서 외부에서 `8083` 포트를 텔레그램 webhook용으로 열 필요는 없다.
- `APP_PORT`는 지금 health 확인용이다.
- 실제 대화 품질은 대부분 `OLLAMA_MODEL`과 프롬프트에 의해 결정된다.

## 빠른 점검 순서

1. `.env`에 Telegram 토큰이 들어있는지 확인
2. `ollama serve`가 떠 있는지 확인
3. `ollama list`로 모델이 있는지 확인
4. `go run ./cmd/bot-api` 또는 `./bin/bot-api` 실행
5. `curl http://localhost:8083/healthz` 확인
6. 텔레그램에서 `/start` 보내서 응답 확인

## 다음 작업 추천

- 세션 락 추가
- memory profile / event 요약 추가
