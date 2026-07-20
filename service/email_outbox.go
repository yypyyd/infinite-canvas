package service

import (
	"log"
	"time"

	"github.com/basketikun/infinite-canvas/repository"
)

func StartOrganizationEmailOutboxWorker() {
	go func() {
		for {
			processed := false
			for index := 0; index < 20; index++ {
				claimedAt := time.Now().UTC()
				item, claimed, err := repository.ClaimOrganizationEmailOutbox(claimedAt.Format(timestampLayout), claimedAt.Add(2*time.Minute).Format(timestampLayout), newID("email-lease"))
				if err != nil { log.Printf("claim organization email outbox failed: %v", err); break }
				if !claimed { break }
				processed = true
				deliveryErr := SendOrganizationInvitationEmail(item.Receiver, item.OrganizationName, item.Role)
				errorMessage := ""
				if deliveryErr != nil { errorMessage = batchProductionErrorMessage(deliveryErr.Error()) }
				delay := time.Minute * time.Duration(1<<min(item.Attempts-1, 6))
				if err := repository.FinishOrganizationEmailOutbox(item, deliveryErr == nil, errorMessage, time.Now().UTC().Add(delay).Format(timestampLayout), now()); err != nil { log.Printf("finish organization email outbox failed: %v", err) }
			}
			if processed { continue }
			time.Sleep(10 * time.Second)
		}
	}()
}
