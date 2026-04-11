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
