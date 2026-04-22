package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	channelx "github.com/hsvtr365/telegram_romance_AI_bot/internal/channel"
)

type TTSConfig struct {
	APIKey       string
	Model        string
	VoiceName    string
	AudioProfile string
	BaseURL      string
	Timeout      time.Duration
	Temperature  float64
	DebugDir     string
}

type TTSClient struct {
	cfg        TTSConfig
	httpClient *http.Client
}

func NewTTSClient(cfg TTSConfig) *TTSClient {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.VoiceName = strings.TrimSpace(cfg.VoiceName)
	cfg.AudioProfile = strings.TrimSpace(cfg.AudioProfile)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.DebugDir = strings.TrimSpace(cfg.DebugDir)
	if cfg.Model == "" {
		cfg.Model = "gemini-3.1-flash-tts-preview"
	}
	if cfg.VoiceName == "" {
		cfg.VoiceName = "Charon"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 1
	}
	if cfg.AudioProfile == "" {
		cfg.AudioProfile = defaultAudioProfile
	}
	return &TTSClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *TTSClient) GenerateAudio(ctx context.Context, transcript string) (channelx.AudioAttachment, error) {
	transcript = strings.TrimSpace(transcript)
	if c == nil || c.cfg.APIKey == "" || transcript == "" {
		return channelx.AudioAttachment{}, nil
	}
	promptText := c.prompt(transcript)
	c.dumpPromptForDebug(transcript, promptText)

	payload := generateContentRequest{
		Contents: []content{{
			Role: "user",
			Parts: []part{{
				Text: promptText,
			}},
		}},
		GenerationConfig: generationConfig{
			Temperature:        c.cfg.Temperature,
			ResponseModalities: []string{"audio"},
			SpeechConfig: speechConfig{
				VoiceConfig: voiceConfig{
					PrebuiltVoiceConfig: prebuiltVoiceConfig{
						VoiceName: c.cfg.VoiceName,
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return channelx.AudioAttachment{}, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.cfg.BaseURL, c.cfg.Model, c.cfg.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return channelx.AudioAttachment{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return channelx.AudioAttachment{}, ctxErr
		}
		return channelx.AudioAttachment{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return channelx.AudioAttachment{}, err
	}
	if resp.StatusCode >= 400 {
		return channelx.AudioAttachment{}, fmt.Errorf("gemini tts returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out generateContentResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return channelx.AudioAttachment{}, fmt.Errorf("decode gemini tts response: %w", err)
	}

	inline, ok := firstInlineData(out)
	if !ok {
		return channelx.AudioAttachment{}, fmt.Errorf("gemini tts response did not include audio")
	}

	audioData, err := base64.StdEncoding.DecodeString(inline.Data)
	if err != nil {
		return channelx.AudioAttachment{}, fmt.Errorf("decode gemini tts audio: %w", err)
	}

	mimeType := strings.TrimSpace(inline.MIMEType)
	fileName := "reward"
	if shouldWrapRawPCM(audioData, mimeType) {
		audioData, mimeType, err = convertLinearPCMToWAV(audioData, mimeType)
		if err != nil {
			return channelx.AudioAttachment{}, err
		}
		fileName += ".wav"
	} else {
		fileName += extensionForAudio(mimeType)
	}

	attachment := channelx.AudioAttachment{
		Data:     audioData,
		MIMEType: mimeType,
		FileName: fileName,
	}
	if err := c.writeDebugAudio(attachment); err != nil {
		return channelx.AudioAttachment{}, err
	}
	return attachment, nil
}

func (c *TTSClient) prompt(transcript string) string {
	profile := c.cfg.AudioProfile
	if strings.HasPrefix(transcript, "[WARM]") {
		transcript = strings.TrimSpace(strings.TrimPrefix(transcript, "[WARM]"))
		//profile = "30대 초반의 직장인 남성. 평소에는 이성적이고 차갑지만, 지금은 상대방에게 무장해제된 듯한 부드럽고 다정한 목소리. 나른하면서도 깊은 울림이 있는 저음으로, 상대가 대견해서 어쩔 줄 모르겠다는 분위기를 풍겨줘. 문장 사이의 호흡을 여유 있게 두고, 말 끝을 아주 부드럽게 맺으며 다정하게 속삭이는 느낌으로 읽어줘."
		profile = "말하는 속도는 빠르고 발음은 또렷해야 합니다. 목소리 톤은 저음이고 날카롭고 매혹적인 느낌이 있으며, 또렷하지 않은 단어의 끝부분에는 살짝 거친 소리가 섞여 있습니다. 지적이고 카리스마 넘치면서도 약간 도발적인, 마치 당신에게 무엇을 해야 할지 지시할 수 있는 듯한 느낌이어야 합니다. 침착하고 절제된 어조로, 신중하게 발음하며, 절대 감정적이거나 연극적이지 않아야 합니다.  문장 사이의 공백은 최소화 해주세요."
	}
	return "## Audio Profile:\n" + profile + "\n\n## Transcript:\n" + transcript
}

func (c *TTSClient) dumpPromptForDebug(transcript string, promptText string) {
	if c == nil {
		return
	}
	f, err := os.OpenFile("debug_tts_prompt.log", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.WriteString(fmt.Sprintf("========== [ TTS Prompt Dump: %s ] ==========\n", time.Now().Format("2006-01-02 15:04:05")))
	_, _ = f.WriteString(fmt.Sprintf("model: %s\n", c.cfg.Model))
	_, _ = f.WriteString(fmt.Sprintf("voice_name: %s\n", c.cfg.VoiceName))
	_, _ = f.WriteString(fmt.Sprintf("temperature: %.2f\n", c.cfg.Temperature))
	_, _ = f.WriteString(fmt.Sprintf("debug_dir: %s\n\n", c.cfg.DebugDir))
	_, _ = f.WriteString("[Transcript]\n")
	_, _ = f.WriteString(transcript)
	_, _ = f.WriteString("\n\n[Prompt]\n")
	_, _ = f.WriteString(promptText)
	_, _ = f.WriteString("\n\n========================================================\n\n")
}

func (c *TTSClient) writeDebugAudio(audio channelx.AudioAttachment) error {
	if c == nil || c.cfg.DebugDir == "" || len(audio.Data) == 0 {
		return nil
	}
	if err := os.MkdirAll(c.cfg.DebugDir, 0750); err != nil {
		return fmt.Errorf("create tts debug dir: %w", err)
	}
	fileName := strings.TrimSpace(audio.FileName)
	if fileName == "" {
		fileName = "reward.wav"
	}
	stamped := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405.000000000"), filepath.Base(fileName))
	path := filepath.Join(c.cfg.DebugDir, stamped)
	if err := os.WriteFile(path, audio.Data, 0600); err != nil {
		return fmt.Errorf("write tts debug audio: %w", err)
	}
	return nil
}

type generateContentRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type generationConfig struct {
	Temperature        float64      `json:"temperature,omitempty"`
	ResponseModalities []string     `json:"responseModalities,omitempty"`
	SpeechConfig       speechConfig `json:"speechConfig,omitempty"`
}

type speechConfig struct {
	VoiceConfig voiceConfig `json:"voiceConfig,omitempty"`
}

type voiceConfig struct {
	PrebuiltVoiceConfig prebuiltVoiceConfig `json:"prebuiltVoiceConfig,omitempty"`
}

type prebuiltVoiceConfig struct {
	VoiceName string `json:"voiceName,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts,omitempty"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type inlineData struct {
	MIMEType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
}

type generateContentResponse struct {
	Candidates []struct {
		Content content `json:"content,omitempty"`
	} `json:"candidates,omitempty"`
}

func firstInlineData(resp generateContentResponse) (inlineData, bool) {
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" {
				return *part.InlineData, true
			}
		}
	}
	return inlineData{}, false
}

type wavOptions struct {
	Channels      int
	SampleRate    int
	BitsPerSample int
}

func convertLinearPCMToWAV(raw []byte, mimeType string) ([]byte, string, error) {
	options := parseAudioMIME(mimeType)
	if options.SampleRate <= 0 && looksLikeWAVMIME(mimeType) {
		options.SampleRate = 24000
	}
	if options.BitsPerSample <= 0 && looksLikeWAVMIME(mimeType) {
		options.BitsPerSample = 16
	}
	if options.SampleRate <= 0 || options.BitsPerSample <= 0 {
		return nil, "", fmt.Errorf("unsupported raw audio mime type: %s", mimeType)
	}

	header := make([]byte, 44)
	byteRate := options.SampleRate * options.Channels * options.BitsPerSample / 8
	blockAlign := options.Channels * options.BitsPerSample / 8

	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(raw)))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(options.Channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(options.SampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], uint16(options.BitsPerSample))
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(raw)))

	return append(header, raw...), "audio/wav", nil
}

func shouldWrapRawPCM(data []byte, mimeType string) bool {
	if hasWAVHeader(data) {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(normalized, "audio/l") ||
		strings.Contains(normalized, ";rate=") ||
		looksLikeWAVMIME(normalized)
}

func hasWAVHeader(data []byte) bool {
	return len(data) >= 12 &&
		string(data[0:4]) == "RIFF" &&
		string(data[8:12]) == "WAVE"
}

func looksLikeWAVMIME(mimeType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	return normalized == "audio/wav" || normalized == "audio/wave" || normalized == "audio/x-wav"
}

func parseAudioMIME(mimeType string) wavOptions {
	options := wavOptions{Channels: 1}
	parts := strings.Split(mimeType, ";")
	if len(parts) > 0 {
		fileType := strings.TrimSpace(parts[0])
		if slash := strings.Index(fileType, "/"); slash >= 0 {
			format := fileType[slash+1:]
			format = strings.ToLower(format)
			if strings.HasPrefix(format, "l") {
				if bits, err := strconv.Atoi(strings.TrimPrefix(format, "l")); err == nil {
					options.BitsPerSample = bits
				}
			}
		}
	}
	for _, param := range parts[1:] {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "rate":
			if rate, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				options.SampleRate = rate
			}
		}
	}
	return options
}

func extensionForAudio(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return ".wav"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/ogg":
		return ".ogg"
	default:
		return ".wav"
	}
}

const defaultAudioProfile = "말하는 속도는 빠르고 발음은 또렷해야 합니다. 목소리 톤은 저음이고 날카롭고 매혹적인 느낌이 있으며, 또렷하지 않은 단어의 끝부분에는 살짝 거친 소리가 섞여 있습니다. 지적이고 카리스마 넘치면서도 약간 도발적인, 마치 당신에게 무엇을 해야 할지 지시할 수 있는 듯한 느낌이어야 합니다. 침착하고 절제된 어조로, 신중하게 발음하며, 절대 감정적이거나 연극적이지 않아야 합니다.  문장 사이의 공백은 최소화 해주세요."
