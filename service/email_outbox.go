package service

import (
	"time"

	"github.com/basketikun/infinite-canvas/repository"
)

func StartOrganizationEmailOutboxWorker() {
	logWorkerInfo("email_outbox", "worker_started")
	go func() {
		for {
			processed := false
			for index := 0; index < 20; index++ {
				claimedAt := time.Now().UTC()
				item, claimed, err := repository.ClaimOrganizationEmailOutbox(claimedAt.Format(timestampLayout), claimedAt.Add(2*time.Minute).Format(timestampLayout), newID("email-lease"))
				if err != nil {
					logWorkerError("email_outbox", "item_claim_failed", err)
					break
				}
				if !claimed {
					break
				}
				processed = true
				logAttrs := []any{"organization_id", item.OrganizationID, "outbox_id", item.ID, "invitation_id", item.InvitationID, "attempts", item.Attempts}
				logWorkerInfo("email_outbox", "delivery_started", logAttrs...)
				deliveryErr := SendOrganizationInvitationEmail(item.Receiver, item.OrganizationName, item.Role)
				errorMessage := ""
				if deliveryErr != nil {
					errorMessage = batchProductionErrorMessage(deliveryErr.Error())
				}
				delay := time.Minute * time.Duration(1<<min(item.Attempts-1, 6))
				if err := repository.FinishOrganizationEmailOutbox(item, deliveryErr == nil, errorMessage, time.Now().UTC().Add(delay).Format(timestampLayout), now()); err != nil {
					logWorkerError("email_outbox", "delivery_finalize_failed", err, logAttrs...)
				} else if deliveryErr != nil {
					logWorkerError("email_outbox", "delivery_failed", deliveryErr, logAttrs...)
				} else {
					logWorkerInfo("email_outbox", "delivery_completed", logAttrs...)
				}
			}
			if processed {
				continue
			}
			time.Sleep(10 * time.Second)
		}
	}()
}
