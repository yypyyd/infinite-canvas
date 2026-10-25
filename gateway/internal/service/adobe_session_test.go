package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"gorm.io/datatypes"
)

func TestAdobeSessionSnapshotPhases(t *testing.T) {
	ready := model.TokenAccount{
		ID: "a1", Status: "active", Value: "jwt", ARPSessionToken: "sid1234567890",
		Meta: datatypes.JSONMap{metaSessionPhase: AdobeSessionPhaseReady, metaStickyEgressFP: "abc"},
	}
	snap := adobeSessionSnapshot(ready)
	if snap.Phase != AdobeSessionPhaseReady || snap.ARPLen == 0 || !snap.Healthy || snap.StickyEgressFP != "abc" {
		t.Fatalf("ready snap %#v", snap)
	}

	dead := model.TokenAccount{ID: "a2", Status: "disabled", Dead: true, Meta: datatypes.JSONMap{metaSessionPhase: AdobeSessionPhaseReady}}
	snap = adobeSessionSnapshot(dead)
	if snap.Phase != AdobeSessionPhaseDead || snap.Healthy {
		t.Fatalf("dead snap %#v", snap)
	}
}
