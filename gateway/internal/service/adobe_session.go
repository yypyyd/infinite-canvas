package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/adobe"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
)

// Adobe account session lifecycle (gateway account layer — not supply routing):
//
//	import → health_check → sticky_bind → ready
//	                ↓ fail                ↓ mismatch / 401
//	              dead/disabled ←—— fail_rotate / remint
//
// Sticky egress (residential extract), ARP sid, and IMS remint are explicit
// phases with structured logs so admin health fields stay honest.

const (
	AdobeSessionPhaseImport      = "import"
	AdobeSessionPhaseHealthCheck = "health_check"
	AdobeSessionPhaseStickyBind  = "sticky_bind"
	AdobeSessionPhaseReady       = "ready"
	AdobeSessionPhaseRemint      = "remint"
	AdobeSessionPhaseFailRotate  = "fail_rotate"
	AdobeSessionPhaseDead        = "dead"
)

// AdobeSessionSnapshot is the admin-visible session surface for one Adobe account.
type AdobeSessionSnapshot struct {
	Phase          string `json:"phase"`
	StickyEgressFP string `json:"sticky_egress_fp,omitempty"`
	ARPLen         int    `json:"arp_len"`
	HasAccessToken bool   `json:"has_access_token"`
	LastRemintAt   string `json:"last_remint_at,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	Healthy        bool   `json:"healthy"`
}

const (
	metaStickyEgressFP = "sticky_egress_fp"
	metaSessionPhase   = "adobe_session_phase"
	metaLastRemintAt   = "adobe_last_remint_at"
	metaLastSessionErr = "adobe_session_last_error"
)

func adobeSessionLog(event, accountID string, fields map[string]any) {
	parts := make([]string, 0, len(fields)+3)
	parts = append(parts, "adobe_session", "event="+event, "account="+accountID)
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	log.Print(strings.Join(parts, " "))
}

func adobeSessionSnapshot(account model.TokenAccount) AdobeSessionSnapshot {
	snap := AdobeSessionSnapshot{
		Phase:          AdobeSessionPhaseImport,
		ARPLen:         len(strings.TrimSpace(account.ARPSessionToken)),
		HasAccessToken: strings.TrimSpace(account.Value) != "",
		Healthy:        !account.Dead && account.Status == "active" && strings.TrimSpace(account.Value) != "",
	}
	if account.Meta != nil {
		snap.StickyEgressFP = strings.TrimSpace(fmt.Sprint(account.Meta[metaStickyEgressFP]))
		if phase := strings.TrimSpace(fmt.Sprint(account.Meta[metaSessionPhase])); phase != "" {
			snap.Phase = phase
		}
		snap.LastRemintAt = strings.TrimSpace(fmt.Sprint(account.Meta[metaLastRemintAt]))
		snap.LastError = strings.TrimSpace(fmt.Sprint(account.Meta[metaLastSessionErr]))
	}
	if account.Dead || account.Status == "disabled" {
		snap.Phase = AdobeSessionPhaseDead
		snap.Healthy = false
	} else if snap.Phase == AdobeSessionPhaseImport && snap.HasAccessToken && snap.ARPLen > 0 {
		snap.Phase = AdobeSessionPhaseReady
	}
	return snap
}

func adobeStickyEgressFingerprint(ctx context.Context, settings *repo.SiteSettingRepository) string {
	if settings == nil {
		return ""
	}
	snap, err := loadProviderProxies(ctx, settings)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(snap.ExtractByProvider["adobe"]) + "|" + strings.TrimSpace(snap.ByProvider["adobe"])
	if raw == "|" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}

func markAdobeSessionMeta(ctx context.Context, tokens *repo.TokenRepository, accountID, phase, lastErr string, extra map[string]any) {
	if tokens == nil || strings.TrimSpace(accountID) == "" {
		return
	}
	meta := map[string]any{metaSessionPhase: phase, metaLastSessionErr: strings.TrimSpace(lastErr)}
	if phase == AdobeSessionPhaseRemint || phase == AdobeSessionPhaseStickyBind || phase == AdobeSessionPhaseReady {
		meta[metaLastRemintAt] = time.Now().UTC().Format(time.RFC3339)
	}
	for k, v := range extra {
		meta[k] = v
	}
	if err := tokens.UpdateMergingMeta(ctx, "adobe", accountID, meta, nil); err != nil {
		adobeSessionLog("phase_meta_failed", accountID, map[string]any{"phase": phase, "err": err})
	}
}

// recordAdobeSessionImport marks a freshly created Adobe row as import/health_check.
func (s *TokenService) recordAdobeSessionImport(ctx context.Context, accountID string) {
	fp := adobeStickyEgressFingerprint(ctx, s.settings)
	extra := map[string]any{}
	if fp != "" {
		extra[metaStickyEgressFP] = fp
	}
	markAdobeSessionMeta(ctx, s.tokens, accountID, AdobeSessionPhaseHealthCheck, "", extra)
	adobeSessionLog("import", accountID, map[string]any{"phase": AdobeSessionPhaseHealthCheck, "egress_fp": fp})
}

// recordAdobeSessionHealthCheckResult finalizes import → ready or dead.
func (s *TokenService) recordAdobeSessionHealthCheckResult(ctx context.Context, accountID string, ok bool, errMsg string, arpLen int) {
	phase := AdobeSessionPhaseReady
	if !ok {
		phase = AdobeSessionPhaseDead
	}
	fp := adobeStickyEgressFingerprint(ctx, s.settings)
	extra := map[string]any{}
	if fp != "" {
		extra[metaStickyEgressFP] = fp
	}
	markAdobeSessionMeta(ctx, s.tokens, accountID, phase, errMsg, extra)
	adobeSessionLog("health_check", accountID, map[string]any{
		"phase": phase, "ok": ok, "arp_len": arpLen, "egress_fp": fp, "err": errMsg,
	})
}

func (s *V1Service) adobeStickyEgressFingerprint(ctx context.Context) string {
	return adobeStickyEgressFingerprint(ctx, s.settings)
}

func (s *V1Service) markAdobeSessionPhase(ctx context.Context, accountID, phase, lastErr string) {
	markAdobeSessionMeta(ctx, s.tokens, accountID, phase, lastErr, nil)
}

// ensureAdobeIMSMatchesEgress remints the IMS access token on the current
// sticky residential exit when the account was last bound to a different
// egress fingerprint (or never bound). No-op when already matched.
func (s *V1Service) ensureAdobeIMSMatchesEgress(ctx context.Context, token model.TokenAccount) model.TokenAccount {
	fp := s.adobeStickyEgressFingerprint(ctx)
	if fp == "" {
		return token
	}
	cur := ""
	if token.Meta != nil {
		cur = strings.TrimSpace(fmt.Sprint(token.Meta[metaStickyEgressFP]))
	}
	if cur == fp {
		return token
	}
	s.markAdobeSessionPhase(ctx, token.ID, AdobeSessionPhaseStickyBind, "")
	adobeSessionLog("sticky_bind_start", token.ID, map[string]any{
		"egress_fp": fp, "prior_fp": cur, "arp_len": len(strings.TrimSpace(token.ARPSessionToken)),
	})
	refreshed, ok := s.refreshAdobeToken(ctx, token.ID)
	if !ok {
		s.markAdobeSessionPhase(ctx, token.ID, AdobeSessionPhaseFailRotate, "ims_remint_failed")
		adobeSessionLog("remint_failed", token.ID, map[string]any{"egress_fp": fp, "prior_fp": cur})
		return token
	}
	markAdobeSessionMeta(ctx, s.tokens, token.ID, AdobeSessionPhaseReady, "", map[string]any{
		metaStickyEgressFP: fp,
	})
	if t, err := s.tokens.Get(ctx, "adobe", token.ID); err == nil && t != nil {
		adobeSessionLog("remint_ok", t.ID, map[string]any{
			"sticky_key": t.ID, "arp_len": len(strings.TrimSpace(t.ARPSessionToken)), "egress_fp": fp,
		})
		return *t
	}
	adobeSessionLog("remint_ok", refreshed.ID, map[string]any{
		"sticky_key": refreshed.ID, "arp_len": len(strings.TrimSpace(refreshed.ARPSessionToken)), "egress_fp": fp,
	})
	return refreshed
}

// noteAdobeSessionRotate records sticky egress rotation after hard failures.
func noteAdobeSessionRotate(accountID, reason string, client *adobe.Client) {
	if client != nil {
		client.RotateProxySession(accountID)
	}
	adobeSessionLog("fail_rotate", accountID, map[string]any{"reason": reason})
}
