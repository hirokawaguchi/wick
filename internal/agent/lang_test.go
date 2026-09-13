package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/store"
)

func TestCannedSpeechEN(t *testing.T) {
	if got := reactLine("kai", "hello", i18n.EN); !strings.Contains(got, "understood") {
		t.Fatalf("react en = %q", got)
	}
	if got := fillerLine(1, i18n.EN); got != "...it's a bit quiet." {
		t.Fatalf("filler en = %q", got)
	}
	if got := workerReply("scout", "", i18n.EN); !strings.Contains(got, "scout will take this") {
		t.Fatalf("worker en = %q", got)
	}

	b := buildBrain(Spec{Behavior: "chatter", Room: 1}, "kai", "Kai", i18n.EN)
	if s, _ := b.Next(Observation{}); s != "chat 1\n" {
		t.Fatalf("enter = %q", s)
	}
	s, _ := b.Next(Observation{})
	if !strings.Contains(s, "This is Kai") {
		t.Fatalf("chatter hi en = %q", s)
	}

	feed := func() []NoteInfo {
		return []NoteInfo{{Num: 2, Title: "Chat", RespCount: store.MaxResponses}}
	}
	rb := newResponderBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist", feed, i18n.EN)
	rb.lastPost = time.Now().Add(-2 * time.Minute)
	got, _ := rb.Next(Observation{Now: time.Now()})
	if !strings.HasPrefix(got, "open junk.test\nwContinued: ") {
		t.Fatalf("continuation en = %q", got)
	}
	if !strings.Contains(got, "Continued from") {
		t.Fatalf("continuation body en = %q", got)
	}
}

func TestJanitorEnglishMark(t *testing.T) {
	if !hasConclusionMark("Conclusion: enough for now.") {
		t.Fatal("english conclusion mark not detected")
	}
	if !hasConclusionMark("summary: done") {
		t.Fatal("english summary mark not detected")
	}
	if hasConclusionMark("just chatting") {
		t.Fatal("false positive on ordinary text")
	}
}

func TestPosterSeedsEN(t *testing.T) {
	seeds := posterSeeds(i18n.EN)
	if seeds[0] != "A small recent find" {
		t.Fatalf("seed0 = %q", seeds[0])
	}
	topics := posterTopics("Kai", i18n.EN)
	if !strings.Contains(topics[0].title, "Kai's note") {
		t.Fatalf("topic title = %q", topics[0].title)
	}
}
