package chat

import "testing"

func TestShouldTriggerChatRewardAudioExplicitCompletionReward(t *testing.T) {
	input := "웅 지금 끝났어요 보상줘요"
	reply := "--- 진짜 바로 끝내고 올 줄은 몰랐네.\n기특해서 어쩌지? 자, 여기 약속한 보상이에요."

	if !shouldTriggerChatRewardAudio(input, reply) {
		t.Fatalf("expected explicit completion reward request to trigger audio")
	}
}

func TestShouldTriggerChatRewardAudioCompletionWithRewardReply(t *testing.T) {
	input := "다 했어요"
	reply := "잘했다. 약속한 보상 줘야겠네."

	if !shouldTriggerChatRewardAudio(input, reply) {
		t.Fatalf("expected completion with reward-bearing reply to trigger audio")
	}
}

func TestShouldTriggerChatRewardAudioExplicitRewardOnly(t *testing.T) {
	input := "보상줘요"
	reply := "잠시만요, 다시 제대로 보내줄게요."

	if !shouldTriggerChatRewardAudio(input, reply) {
		t.Fatalf("expected explicit reward request to trigger audio")
	}
}

func TestShouldTriggerChatRewardAudioRewardComplaint(t *testing.T) {
	input := "보상안줘요..?"
	reply := "서버가 수지 씨 속도를 못 따라가네요."

	if !shouldTriggerChatRewardAudio(input, reply) {
		t.Fatalf("expected reward complaint to trigger audio retry")
	}
}

func TestShouldTriggerChatRewardAudioDoesNotTriggerCasualRewardTalk(t *testing.T) {
	input := "보상은 나중에 뭐가 좋아요?"
	reply := "나중에 정하자."

	if shouldTriggerChatRewardAudio(input, reply) {
		t.Fatalf("expected casual reward discussion without completion to stay silent")
	}
}

func TestChatRewardTranscriptCleansRoleLikeSeparators(t *testing.T) {
	reply := "--- 진짜 바로 끝내고 올 줄은 몰랐네.\n기특해서 어쩌지? 자, 여기 약속한 보상이에요.\n마음에 들어요?"

	got := chatRewardTranscript("웅 지금 끝났어요 보상줘요", reply)
	want := "진짜 바로 끝내고 올 줄은 몰랐네. 기특해서 어쩌지? 자, 여기 약속한 보상이에요. 마음에 들어요?"
	if got != want {
		t.Fatalf("unexpected transcript\nwant: %q\n got: %q", want, got)
	}
}
