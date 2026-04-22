package gemini

import (
	"encoding/base64"
	"encoding/binary"
	"testing"
)

func TestConvertLinearPCMToWAV(t *testing.T) {
	raw := []byte{0x01, 0x02, 0x03, 0x04}
	got, mimeType, err := convertLinearPCMToWAV(raw, "audio/L16;rate=24000")
	if err != nil {
		t.Fatalf("convertLinearPCMToWAV returned error: %v", err)
	}
	if mimeType != "audio/wav" {
		t.Fatalf("unexpected mime type: %s", mimeType)
	}
	if string(got[0:4]) != "RIFF" || string(got[8:12]) != "WAVE" {
		t.Fatalf("missing wav markers: %q %q", got[0:4], got[8:12])
	}
	if rate := binary.LittleEndian.Uint32(got[24:28]); rate != 24000 {
		t.Fatalf("unexpected sample rate: %d", rate)
	}
	if bits := binary.LittleEndian.Uint16(got[34:36]); bits != 16 {
		t.Fatalf("unexpected bits per sample: %d", bits)
	}
	if dataSize := binary.LittleEndian.Uint32(got[40:44]); dataSize != uint32(len(raw)) {
		t.Fatalf("unexpected data size: %d", dataSize)
	}
	if string(got[44:]) != string(raw) {
		t.Fatalf("raw data was not appended")
	}
}

func TestShouldWrapRawPCMWrapsMislabelledWAV(t *testing.T) {
	raw := []byte{0x00, 0x00, 0x01, 0x00}
	if !shouldWrapRawPCM(raw, "audio/wav") {
		t.Fatal("expected raw data mislabeled as wav to be wrapped")
	}
}

func TestShouldWrapRawPCMLeavesRealWAVAlone(t *testing.T) {
	wav := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	if shouldWrapRawPCM(wav, "audio/wav") {
		t.Fatal("expected real wav data to stay unchanged")
	}
}

func TestConvertMislabelledWAVRawPCMUsesGeminiDefaults(t *testing.T) {
	raw := []byte{0x00, 0x00, 0x01, 0x00}
	got, mimeType, err := convertLinearPCMToWAV(raw, "audio/wav")
	if err != nil {
		t.Fatalf("convertLinearPCMToWAV returned error: %v", err)
	}
	if mimeType != "audio/wav" {
		t.Fatalf("unexpected mime type: %s", mimeType)
	}
	if string(got[0:4]) != "RIFF" || string(got[8:12]) != "WAVE" {
		t.Fatalf("missing wav markers")
	}
	if rate := binary.LittleEndian.Uint32(got[24:28]); rate != 24000 {
		t.Fatalf("unexpected default sample rate: %d", rate)
	}
	if bits := binary.LittleEndian.Uint16(got[34:36]); bits != 16 {
		t.Fatalf("unexpected default bits per sample: %d", bits)
	}
}

func TestFirstInlineData(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("audio"))
	inline, ok := firstInlineData(generateContentResponse{
		Candidates: []struct {
			Content content `json:"content,omitempty"`
		}{{
			Content: content{Parts: []part{{InlineData: &inlineData{MIMEType: "audio/wav", Data: payload}}}},
		}},
	})
	if !ok {
		t.Fatal("expected inline data")
	}
	if inline.MIMEType != "audio/wav" || inline.Data != payload {
		t.Fatalf("unexpected inline data: %+v", inline)
	}
}
