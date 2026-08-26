package audit

import (
	"testing"
	"time"
)

func TestEventChainDetectsTampering(t *testing.T) {
	first, err := Build("mission-1", "created", "actor-1", "request-1", 0, 1, map[string]string{"value": "a"}, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	first.StatusAfter = "draft"
	first = Seal(first, 1, "")
	second, err := Build("mission-1", "assessed", "actor-2", "request-2", 1, 2, map[string]string{"value": "b"}, time.Unix(200, 0))
	if err != nil {
		t.Fatal(err)
	}
	second.StatusAfter = "risk_assessed"
	second = Seal(second, 2, first.EventHash)
	events := []Event{first, second}
	if err := Verify(events); err != nil {
		t.Fatalf("有效事件链未通过: %v", err)
	}
	events[1].StatusAfter = "archived"
	if err := Verify(events); err == nil {
		t.Fatal("篡改状态后仍通过事件链校验")
	}
	events[1] = second
	events[1].Data = []byte(`{"value":"tampered"}`)
	if err := Verify(events); err == nil {
		t.Fatal("篡改事件载荷后仍通过事件链校验")
	}
}

func TestEventChainRequiresContinuousSequence(t *testing.T) {
	event, err := Build("mission-1", "created", "actor-1", "request-1", 0, 1, struct{}{}, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	event = Seal(event, 2, "")
	if err := Verify([]Event{event}); err == nil {
		t.Fatal("非连续序号未被拒绝")
	}
}
