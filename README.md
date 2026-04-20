# telegram_romance_AI_bot

텔레그램에서 1:1로 대화하는 가상연애 챗봇 프로젝트다.  
현재 구현은 `Go long polling bot + Ollama/OpenAI-compatible LLM + health check HTTP server` 구조로 시작한다.

## 용도

- 텔레그램 개인 채팅에서 가상연애 톤으로 대화
- Ollama에 올린 로컬/서버 LLM을 호출해서 응답 생성
- 이후 Redis 최근 대화 메모리, Postgres 저장, 장기 메모리 요약을 붙이기 위한 기본 골격

## 현재 실제로 작동하는 것

- Telegram Bot API `getUpdates` 기반 long polling
- `/start`, `/ping` 명령 처리
- `/reset`, `/리셋` 명령으로 대화 기록 초기화
- `/proactive_on`, `/선톡켜`, `/proactive_off`, `/선톡꺼` 명령 처리
- 일반 텍스트 메시지를 Ollama `/api/chat` 또는 OpenAI-compatible `/v1/chat/completions`으로 전달
- Postgres에 사용자/세션/메시지 저장
- Redis에 최근 대화 14턴 캐시
- 선톡 opt-in, 이벤트 힌트 저장, 조건 기반 선톡 worker
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

OLLAMA_ENDPOINT_01_BASE_URL=https://gitlab.swempire.co.kr/ollama
OLLAMA_ENDPOINT_01_MODEL=gemma4-heretic:q4km
OLLAMA_ENDPOINT_02_BASE_URL=http://127.0.0.1:11434
OLLAMA_ENDPOINT_02_MODEL=gemma4-26b-heretic-q4km:latest
OLLAMA_BASE_URL=https://gitlab.swempire.co.kr/ollama
OLLAMA_HEALTHCHECK_INTERVAL_SEC=1800
OLLAMA_MODEL=gemma4-heretic:q4km

POSTGRES_DSN=postgres://...
REDIS_URL=redis://...
```

## Ollama 준비

기본 설정은 원격 reverse proxy를 사용한다.

```env
OLLAMA_ENDPOINT_01_BASE_URL=https://gitlab.swempire.co.kr/ollama
OLLAMA_ENDPOINT_01_MODEL=gemma4-heretic:q4km
OLLAMA_ENDPOINT_02_BASE_URL=http://127.0.0.1:11434
OLLAMA_ENDPOINT_02_MODEL=gemma4-26b-heretic-q4km:latest
OLLAMA_BASE_URL=https://gitlab.swempire.co.kr/ollama
OLLAMA_HEALTHCHECK_INTERVAL_SEC=1800
OLLAMA_MODEL=gemma4-heretic:q4km
```

프록시 상태는 아래처럼 확인할 수 있다.

```bash
curl https://gitlab.swempire.co.kr/ollama/api/tags
```

단일 서버만 쓸 때는 `OLLAMA_BASE_URL`, `OLLAMA_MODEL`만 써도 된다.

서버마다 모델명이 다를 수 있으면 `OLLAMA_ENDPOINT_01_BASE_URL`, `OLLAMA_ENDPOINT_01_MODEL`처럼 순번별로 짝을 맞춰 넣으면 된다.
봇은 번호가 작은 endpoint부터 healthy한 서버를 우선 사용하고, 연결 실패나 5xx가 나면 다음 서버로 즉시 넘어간다.
또한 `OLLAMA_HEALTHCHECK_INTERVAL_SEC` 주기로 unhealthy 서버의 `/api/tags`를 다시 확인해서 살아나면 원래 우선순위대로 복귀한다.

`http://.../v1` 형태의 base URL을 넣으면 OpenAI-compatible endpoint로 자동 인식해서 `/v1/chat/completions`와 `/v1/models`를 사용한다. 그래서 메인 모델 endpoint도 `Ollama`뿐 아니라 `mlx_lm.server`로 둘 수 있다.

## MLX 메인 모델 붙이기

`supergemma4-26b-mlx`를 메인 endpoint 풀의 2순위로 넣고, 전체 순서를 `원격 서버 -> 로컬 MLX -> 로컬 Ollama`로 두려면:

1. MLX 서버를 띄운다.

```bash
cd /Users/suji/models/supergemma4-26b-mlx
./run_server.sh
```

2. `.env`에 아래 값을 넣는다.

```env
OLLAMA_ENDPOINT_01_BASE_URL=https://gitlab.swempire.co.kr/ollama
OLLAMA_ENDPOINT_01_MODEL=gemma4-heretic:q4km
OLLAMA_ENDPOINT_02_BASE_URL=http://127.0.0.1:18123/v1
OLLAMA_ENDPOINT_02_MODEL=/Users/suji/models/supergemma4-26b-mlx
OLLAMA_ENDPOINT_03_BASE_URL=http://127.0.0.1:11434
OLLAMA_ENDPOINT_03_MODEL=gemma4-26b-heretic-q4km:latest
OLLAMA_BASE_URL=https://gitlab.swempire.co.kr/ollama
OLLAMA_MODEL=gemma4-heretic:q4km
```

이렇게 하면 메인 대화는 먼저 원격 Ollama 서버를 시도하고, 실패하면 로컬 MLX, 그것도 실패하면 로컬 Ollama로 넘어간다. 즉 메인 모델 풀에 `Ollama`와 `MLX`를 섞어서 순서대로 failover 시킬 수 있다.

## Ollama run 같은 대화형 사용

`mlx-lm`은 `ollama run`과 완전히 같은 CLI는 아니지만 비슷하게 쓸 수 있다.

간단한 1회성 대화:

```bash
cd /Users/suji/models/supergemma4-26b-mlx
./run_generate.sh "오늘 기분 좋은 톤으로 인사해줘."
```

반복 대화형 REPL:

```bash
source /Users/suji/models/supergemma4-26b-mlx/.venv/bin/activate
mlx_lm.chat --model /Users/suji/models/supergemma4-26b-mlx
```

OpenAI-compatible 서버를 띄워두고 외부 클라이언트에서 채팅:

```bash
cd /Users/suji/models/supergemma4-26b-mlx
./run_server.sh
./test_chat.sh "안녕?"
```

REPL인 `mlx_lm.chat`은 `ollama run`과 가장 비슷한 사용감이고, 서버 방식은 P2 같은 외부 앱에 붙이기 좋다.

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

## 선톡 사용법

선톡은 기본적으로 꺼져 있다.  
먼저 텔레그램에서 아래 명령 중 하나를 보내면 된다.

```text
/선톡켜
/proactive_on
```

끄고 싶으면 아래 명령을 쓰면 된다.

```text
/선톡꺼
/proactive_off
```

사용 흐름은 아주 단순하다.

1. 먼저 `/선톡켜`를 보낸다.
2. 평소처럼 대화한다.
3. 시험, 약속, 회식, 면접 같은 일정 이야기를 하면 봇이 그 내용을 힌트로 저장할 수 있다.
4. 이후 대화가 끊겼거나, 일정 타이밍이 오거나, 조건이 맞으면 봇이 먼저 톡할 수 있다.

중요한 점:

- 선톡은 켠다고 바로 오는 방식이 아니다.
- 사용자의 최근 활동, 이벤트 힌트, 시간대, 이전 반응 같은 조건이 맞을 때만 보낸다.
- 너무 자주 보내지 않도록 사용자별 간격과 누적 반응을 함께 본다.

가볍게 테스트하려면 이런 식으로 써보면 된다.

```text
내일 시험 있어
오늘 저녁에 약속 있어
좀 이따 면접 봐
```

이런 메시지가 들어가면 봇이 일정 힌트를 저장하고, 나중에 적절한 타이밍에 먼저 말을 걸 수 있다.

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
