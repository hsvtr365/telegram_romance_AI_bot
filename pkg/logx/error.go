package logx

import "strings"

// KoreanError returns a short Korean summary for common runtime and network errors.
func KoreanError(err error) string {
	if err == nil {
		return ""
	}

	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "network is unreachable"):
		return "네트워크 경로를 찾지 못해 서버에 연결할 수 없습니다."
	case strings.Contains(lower, "connection reset by peer"):
		return "상대 서버가 연결을 중간에 끊었습니다."
	case strings.Contains(lower, "operation was canceled"):
		return "연결 시도 중 작업이 취소되었습니다."
	case strings.Contains(lower, "context canceled"):
		return "요청 처리 중 작업이 취소되었습니다."
	case strings.Contains(lower, "address already in use"):
		return "이미 사용 중인 포트라서 서버를 시작할 수 없습니다."
	case strings.Contains(lower, "bad gateway"):
		return "중간 프록시 서버가 뒤쪽 서비스에서 정상 응답을 받지 못했습니다."
	case strings.Contains(lower, "connection refused"):
		return "대상 서버가 연결을 거부했습니다."
	case strings.Contains(lower, "no such host"):
		return "서버 주소를 찾지 못했습니다."
	case strings.Contains(lower, "timeout"):
		return "서버 응답 대기 시간이 초과되었습니다."
	default:
		return msg
	}
}
