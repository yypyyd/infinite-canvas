package service

import "testing"

func TestAgentIntentWantsImage(t *testing.T) {
	if !agentIntentWantsImage("\u751f\u6210\u597d\u770b\u7684\u5973\u5b69") {
		t.Fatal("expected image intent for portrait prompt")
	}
	if agentIntentWantsImage("\u628a\u8fd9\u4e24\u4e2a\u8282\u70b9\u8fde\u8d77\u6765") {
		t.Fatal("did not expect image intent for connect prompt")
	}
}

func TestAgentClaimedMediaWithoutTools(t *testing.T) {
	if !agentClaimedMediaWithoutTools("\u5df2\u7ecf\u4e3a\u4f60\u751f\u6210\u4e86\u4e00\u5f20\u6e29\u67d4\u751c\u7f8e\u98ce") {
		t.Fatal("expected fake media success detection")
	}
	if agentClaimedMediaWithoutTools("preparing to call image tools") {
		t.Fatal("did not expect false positive")
	}
}
