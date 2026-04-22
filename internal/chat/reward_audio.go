package chat

import (
	"context"
	"regexp"
	"strings"
	"time"

	channelx "github.com/hsvtr365/telegram_romance_AI_bot/internal/channel"
)

var rewardCleanupPattern = regexp.MustCompile(`(?m)^[-–—\s]+`)

func (s *Service) maybeSendChatRewardAudio(ctx context.Context, message channelx.InboundMessage, input string, reply string) {
	if s == nil {
		return
	}
	if !shouldTriggerChatRewardAudio(input, reply) {
		return
	}
	if s.rewardAudio == nil || s.bot == nil {
		if s.logger != nil {
			s.logger.Warn("chat reward tts triggered but audio generator is unavailable", "target", message.Target().Key())
		}
		return
	}

	key := chatRewardKey(message)
	s.rewardAudioMu.Lock()
	if _, ok := s.rewardAudioSent[key]; ok {
		s.rewardAudioMu.Unlock()
		return
	}
	s.rewardAudioSent[key] = struct{}{}
	s.rewardAudioMu.Unlock()

	transcript := chatRewardTranscript(input, reply)
	if transcript == "" {
		return
	}

	target := message.Target()
	if s.logger != nil {
		s.logger.Info("chat reward tts triggered", "target", target.Key())
	}
	go func() {
		audioCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 75*time.Second)
		defer cancel()

		audio, err := s.rewardAudio.GenerateAudio(audioCtx, transcript)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to generate chat reward tts", "target", target.Key(), "error", err)
			}
			return
		}
		if len(audio.Data) == 0 {
			return
		}
		if audio.FileName == "" {
			audio.FileName = "reward.wav"
		}
		if s.logger != nil {
			s.logger.Info("chat reward tts generated", "target", target.Key(), "mime_type", audio.MIMEType, "file_name", audio.FileName, "bytes", len(audio.Data))
		}
		if err := s.bot.SendAudio(audioCtx, target, audio); err != nil && s.logger != nil {
			s.logger.Warn("failed to send chat reward tts", "target", target.Key(), "error", err)
		} else if s.logger != nil {
			s.logger.Info("chat reward tts sent", "target", target.Key(), "file_name", audio.FileName)
		}
	}()
}

func shouldTriggerChatRewardAudio(input string, reply string) bool {
	normalizedInput := compactKorean(input)
	normalizedReply := compactKorean(reply)
	if normalizedInput == "" {
		return false
	}

	hasRewardAsk := containsRewardAudioAny(normalizedInput, "보상", "상줘", "칭찬", "칭찬해")
	hasExplicitRewardAsk := containsRewardAudioAny(normalizedInput,
		"보상줘", "보상주세요", "보상안줘", "보상안줘요", "보상내놔", "보내줘", "다시보내",
	)
	hasCompletion := containsRewardAudioAny(normalizedInput,
		"끝났", "끝냄", "끝냈", "완료", "다했", "다함", "해냈", "했어요", "했습니다", "마쳤", "처리했", "끝",
	)
	if hasExplicitRewardAsk {
		return true
	}
	if hasRewardAsk && hasCompletion {
		return true
	}

	replyPromisesReward := containsRewardAudioAny(normalizedReply, "보상", "기특", "잘했", "잘했다", "수고했", "약속한")
	return hasCompletion && replyPromisesReward
}

func chatRewardTranscript(input string, reply string) string {
	cleaned := cleanRewardReply(reply)
	if cleaned != "" && containsRewardAudioAny(compactKorean(cleaned), "보상", "기특", "잘했", "잘했다", "수고했", "약속한") {
		return limitRewardTranscript(cleaned)
	}

	return "잘했어요. 말만 한 게 아니라 끝내고 온 거잖아요. 이건 보상받을 만합니다."
}

func cleanRewardReply(reply string) string {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return ""
	}
	lines := strings.Split(reply, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(rewardCleanupPattern.ReplaceAllString(line, ""))
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, " "))
}

func limitRewardTranscript(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= 180 {
		return string(runes)
	}
	return string(runes[:180])
}

func chatRewardKey(message channelx.InboundMessage) string {
	parts := []string{
		strings.TrimSpace(message.BotID),
		strings.TrimSpace(message.Channel),
		strings.TrimSpace(message.ExternalChatID),
		strings.TrimSpace(message.ExternalMessageID),
		strings.TrimSpace(message.ExternalUpdateID),
	}
	key := strings.Join(parts, ":")
	if strings.Trim(key, ":") == "" {
		return message.LockKey() + ":" + compactKorean(message.Text)
	}
	return key
}

func compactKorean(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
}

func containsRewardAudioAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
