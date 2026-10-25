package service

import (
	"context"
	"errors"
)

// ErrGenerationAbandoned marks a task whose caller (幻图) stopped waiting and
// settled it. Recovery must not deliver a result for it afterwards.
var ErrGenerationAbandoned = errors.New("generation abandoned by caller")

// imageRerouteSafe reports whether a failed image request never reached an
// upstream provider in a way that could still produce (or bill) an image, so
// the caller may safely retry it on another channel.
func imageRerouteSafe(err error) bool {
	if err == nil || imageJobStillPending(err) {
		return false
	}
	return errors.Is(err, ErrNoProviderAccount) || errors.Is(err, ErrProviderAuth) ||
		errors.Is(err, ErrProviderQuota) || errors.Is(err, ErrConcurrencyFull) ||
		errors.Is(err, ErrConcurrencyBackendUnavailable) || errors.Is(err, ErrAdobeUpstreamBusy) ||
		errors.Is(err, ErrAdobeTrialExhausted) || errors.Is(err, ErrAdobeGenerationSession) || errors.Is(err, ErrProviderUnsupported)
}

// AbandonImageRequest is called when the caller gives up on an image task. A
// pending task is cancelled and closed as failed; a task that already finished
// is returned unchanged so the caller can still deliver it.
func (s *V1Service) AbandonImageRequest(ctx context.Context, principal *APIPrincipal, requestID string) (map[string]any, error) {
	if principal == nil || principal.Credential == nil {
		return nil, ErrImageTaskNotFound
	}
	event, err := s.events.GetImageByRequestID(ctx, principal.Credential.ID, requestID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, ErrImageTaskNotFound
	}
	if event.Status == "pending" {
		s.inflight.Cancel(event.ID)
		if _, err := s.events.FailPending(ctx, event.ID, ErrGenerationAbandoned.Error()); err != nil {
			return nil, err
		}
	}
	s.imageJobs.Delete(imageJobKey(principal.Credential.ID, requestID))
	return s.imageTaskFromEvent(ctx, principal, requestID)
}

// AbandonVideoJob is the video counterpart of AbandonImageRequest.
func (s *V1Service) AbandonVideoJob(ctx context.Context, principal *APIPrincipal, id string) (map[string]any, error) {
	event, err := s.videoEventForUser(ctx, principal, id)
	if err != nil {
		return nil, err
	}
	if event.Status == "pending" {
		s.inflight.Cancel(event.ID)
		if _, err := s.events.FailPending(ctx, event.ID, ErrGenerationAbandoned.Error()); err != nil {
			return nil, err
		}
	}
	return s.VideoJob(ctx, principal, id)
}

// eventStillPending re-reads an event before a recovery stores its result, so
// a task the caller already abandoned (and refunded) is not completed later.
func (s *V1Service) eventStillPending(ctx context.Context, eventID string) bool {
	event, err := s.events.GetByID(ctx, eventID)
	return err == nil && event != nil && event.Status == "pending"
}

func (s *V1Service) eventAbandoned(ctx context.Context, eventID string) bool {
	event, err := s.events.GetByID(ctx, eventID)
	return err != nil || event == nil || event.Error == ErrGenerationAbandoned.Error()
}
